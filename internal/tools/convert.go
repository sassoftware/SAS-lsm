// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// legacyDump sources a SAS_lsm 4.x configuration file in bash (exactly as
// SAS_lsm 4.x did) and prints every value with unambiguous separators.
const legacyDump = `
set +u
. "$1" >/dev/null 2>&1 || { echo "__ERROR__ could not source $1" ; exit 1; }
p() { printf '%s\037%s\036' "$1" "$2"; }
p MAXTIERS "$MAXTIERS"; p STATUSROOT "$STATUSROOT"; p MAXRETRY "$MAXRETRY"; p SLEEP "$SLEEP"
p SASADMIN "$SASADMIN"; p SSHOPTIONS "$SSHOPTIONS"; p ARCHIVEDLOGS "$ARCHIVEDLOGS"; p LOCALSSH "$LOCALSSH"
i=1
while [ "$i" -le "${MAXTIERS:-0}" ]; do
  for v in TIERNAME TIERINST TIERHOST TIERSTAR TIERSTOP TIERREST TIERSTAT TIERSTAS TIERSTOS TIERLOGD TIERTYPE TIEREXIT; do
    eval "val=\${$v[$i]}"
    p "$v.$i" "$val"
  done
  i=$((i+1))
done
`

type legacyTier struct {
	name, user, host, start, stop, restart, status, down, up, logdir, typ, exit string
}

var (
	reSASServers = regexp.MustCompile(`^(\S+)/sas\.servers\s+start$`)
	reMetaStart  = regexp.MustCompile(`(\S+)/SASMeta/MetadataServer/MetadataServer\.sh\s+start`)
)

// Convert translates a SAS_lsm 4.x configuration file to the YAML format.
// The legacy file is sourced by bash, so it must be trusted (it is the same
// file SAS_lsm 4.x already executes). Returns the YAML and review notes.
func Convert(path string) (string, []string, error) {
	cmd := exec.Command("bash", "-c", legacyDump, "convert", path)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("bash is required to read a SAS_lsm 4.x file: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil || strings.Contains(out.String(), "__ERROR__") {
			return "", nil, fmt.Errorf("could not read %s with bash: %v %s", path, err, strings.TrimSpace(errb.String()))
		}
	case <-time.After(20 * time.Second):
		cmd.Process.Kill()
		return "", nil, fmt.Errorf("reading %s took too long (does it prompt for input?)", path)
	}
	vals := map[string]string{}
	for _, rec := range strings.Split(out.String(), "\x1e") {
		if k, v, ok := strings.Cut(rec, "\x1f"); ok {
			vals[k] = v
		}
	}
	max, err := strconv.Atoi(vals["MAXTIERS"])
	if err != nil || max < 1 {
		return "", nil, fmt.Errorf("%s: MAXTIERS is missing or not a number", path)
	}
	var tiers []legacyTier
	for i := 1; i <= max; i++ {
		g := func(k string) string { return strings.TrimSpace(vals[fmt.Sprintf("%s.%d", k, i)]) }
		tiers = append(tiers, legacyTier{
			name: g("TIERNAME"), user: g("TIERINST"), host: g("TIERHOST"),
			start: unquote(g("TIERSTAR")), stop: unquote(g("TIERSTOP")), restart: unquote(g("TIERREST")),
			status: unquote(g("TIERSTAT")), down: g("TIERSTAS"), up: g("TIERSTOS"),
			logdir: g("TIERLOGD"), typ: strings.ToUpper(g("TIERTYPE")), exit: g("TIEREXIT"),
		})
	}

	var notes []string
	y := &yamlWriter{}
	y.comment("Converted from SAS_lsm 4.x configuration " + path + " on " + time.Now().Format("2006-01-02"))
	y.comment("Review every tier below, then run:  sas_lsm validate -c <this file>  and  sas_lsm doctor -c <this file>")
	y.kv(0, "version", "1")
	y.blank()

	y.key(0, "settings")
	if s := vals["STATUSROOT"]; s != "" {
		y.kv(2, "status_root", s)
		if s == "/tmp" {
			notes = append(notes, "status_root is /tmp; consider a persistent directory such as /var/tmp/sas_lsm (files in /tmp may be cleaned at reboot)")
		}
	}
	if a := strings.Fields(vals["SASADMIN"]); len(a) > 0 {
		y.list(2, "admin_email", a)
	}
	if o := vals["SSHOPTIONS"]; o != "" {
		if strings.Contains(o, "StrictHostKeyChecking=no") {
			notes = append(notes, "ssh_options: StrictHostKeyChecking=no was replaced by accept-new (new hosts are trusted on first connect; changed keys are refused)")
		}
		y.list(2, "ssh_options", filterOpts(strings.Fields(o)))
	}
	if strings.EqualFold(vals["ARCHIVEDLOGS"], "disabled") {
		y.kv(2, "archive_reports", "false")
	}
	if strings.EqualFold(vals["LOCALSSH"], "override") {
		notes = append(notes, "LOCALSSH=override has no direct setting; SAS_lsm 5 always uses SSH when the tier user differs from the user running sas_lsm")
	}
	y.blank()

	y.key(0, "defaults")
	commonUser := mostCommon(tiers, func(t legacyTier) string { return t.user })
	if commonUser != "" {
		y.kv(2, "user", commonUser)
	}
	commonCfg := mostCommon(tiers, func(t legacyTier) string {
		if m := reSASServers.FindStringSubmatch(t.start); m != nil {
			return m[1]
		}
		return ""
	})
	if commonCfg != "" {
		y.kv(2, "configdir", commonCfg)
	}
	if v := vals["MAXRETRY"]; v != "" {
		y.kv(2, "retries", v)
	}
	if v := vals["SLEEP"]; v != "" {
		y.kv(2, "retry_interval", v)
	}
	y.blank()

	y.key(0, "tiers")
	var cluster []legacyTier
	for i, t := range tiers {
		switch t.typ {
		case "MDCN":
			cluster = append(cluster, t)
			continue
		case "MDCV":
			if len(cluster) == 0 {
				return "", nil, fmt.Errorf("tier %d is MDCV but no MDCN tiers precede it", i+1)
			}
			var hosts []string
			for _, c := range cluster {
				hosts = append(hosts, c.host)
			}
			y.item(2, "name", "metadata")
			y.kv(4, "type", "metadata-cluster")
			y.list(4, "hosts", hosts)
			if cluster[0].user != commonUser {
				y.kv(4, "user", cluster[0].user)
			}
			cfgdir := ""
			if m := reMetaStart.FindStringSubmatch(cluster[0].start); m != nil {
				cfgdir = m[1]
			}
			if cfgdir != "" && cfgdir != commonCfg {
				y.kv(4, "configdir", cfgdir)
			}
			notes = append(notes, fmt.Sprintf("tiers %d-%d (metadata cluster nodes and validation) became one tier of type metadata-cluster", i+1-len(cluster), i+1))
			cluster = nil
			y.blank()
			continue
		}
		name := slug(t.name)
		if name == "" {
			name = fmt.Sprintf("tier%d", i+1)
		}
		y.item(2, "name", name)
		if m := reSASServers.FindStringSubmatch(t.start); m != nil && strings.Contains(t.up, "is UP") {
			y.kv(4, "type", "sas-servers")
			y.list(4, "hosts", []string{t.host})
			if t.user != commonUser {
				y.kv(4, "user", t.user)
			}
			if m[1] != commonCfg {
				y.kv(4, "configdir", m[1])
			}
		} else {
			y.kv(4, "type", "custom")
			y.list(4, "hosts", []string{t.host})
			if t.user != commonUser {
				y.kv(4, "user", t.user)
			}
			y.kv(4, "start", t.start)
			y.kv(4, "stop", t.stop)
			if t.restart != "" {
				y.kv(4, "restart", t.restart)
			}
			y.kv(4, "status", t.status)
			y.kv(4, "up_pattern", t.up)
			y.kv(4, "down_pattern", t.down)
			if t.logdir != "" {
				y.kv(4, "logdir", t.logdir)
			}
			notes = append(notes, fmt.Sprintf("tier %d (%s) was converted as type custom; check its commands and patterns", i+1, name))
		}
		if t.exit != "" {
			y.kv(4, "userexit_dir", t.exit)
			notes = append(notes, fmt.Sprintf("tier %d (%s): userexit_dir kept; names now match in any case, and post scripts without _success_/_failure_ run after either result", i+1, name))
		}
		y.blank()
	}
	if len(cluster) > 0 {
		return "", nil, fmt.Errorf("MDCN tiers found without a closing MDCV tier; the legacy configuration is incomplete")
	}
	return y.String(), notes, nil
}

// slug turns "Mid Tier (web)" into "mid-tier-web" so names are easy to type with --from/--to.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

// filterOpts replaces insecure options and adds the ones SAS_lsm relies on.
func filterOpts(in []string) []string {
	var out []string
	for i := 0; i < len(in); i++ {
		if in[i] == "-o" && i+1 < len(in) {
			v := in[i+1]
			i++
			if strings.HasPrefix(v, "StrictHostKeyChecking=no") {
				v = "StrictHostKeyChecking=accept-new"
			}
			out = append(out, "-o", v)
			continue
		}
		out = append(out, in[i])
	}
	has := strings.Join(out, " ")
	if !strings.Contains(has, "BatchMode") {
		out = append(out, "-o", "BatchMode=yes")
	}
	if !strings.Contains(has, "ConnectTimeout") {
		out = append(out, "-o", "ConnectTimeout=10")
	}
	return out
}

func mostCommon(ts []legacyTier, f func(legacyTier) string) string {
	count := map[string]int{}
	best, n := "", 0
	for _, t := range ts {
		if v := f(t); v != "" {
			count[v]++
			if count[v] > n {
				best, n = v, count[v]
			}
		}
	}
	return best
}
