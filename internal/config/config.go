// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package config loads and validates SAS_lsm YAML configuration files.
package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sassoftware/sas-lsm/internal/yamlite"
)

// Built-in tier types.
const (
	TypeSASServers      = "sas-servers"
	TypeMetadataCluster = "metadata-cluster"
	TypeLSFGrid         = "lsf-grid"
	TypeCustom          = "custom"
)

// Events are the userexit hook points.
var Events = []string{
	"pre_start", "post_start_success", "post_start_failure",
	"pre_stop", "post_stop_success", "post_stop_failure",
	"pre_status", "post_status",
}

// Config is a fully resolved configuration.
type Config struct {
	Path     string
	Name     string // used for the status directory name
	Settings Settings
	Tiers    []*Tier
}

// Settings are global (not per-tier) options.
type Settings struct {
	StatusRoot     string
	SSHOptions     []string
	SSHMultiplex   bool
	AdminEmail     []string
	MailCommand    string
	ArchiveReports bool
	CollectLogs    bool
	LogMinutes     int
	LogMaxLines    int
}

// UserExit is one script to run around a tier action.
type UserExit struct {
	Run      string
	Required bool
	On       string // "first" (default) or "all" hosts of the tier
}

// LSF holds lsf-grid tier settings.
type LSF struct {
	Master             string
	Nodes              []string
	EnvDir             string // LSF_ENVDIR (contains profile.lsf)
	ServerDir          string // LSF_SERVERDIR (daemon binaries)
	PMHost             string
	PMEnvDir           string // JS_ENVDIR (contains profile.js)
	PMServerDir        string // JS_SERVERDIR
	EGOControlsDaemons bool
	Sudo               bool
}

// Tier is one ordered unit of control.
type Tier struct {
	Index          int // 1-based position
	Line           int
	Name           string
	Type           string
	Hosts          []string
	User           string
	ConfigDir      string
	Parallel       bool
	Retries        int
	RetryInterval  time.Duration
	TierDelay      time.Duration
	CommandTimeout time.Duration
	OKExitCodes    []int

	Start, Stop, Restart, Status string
	StopCluster                  string // metadata-cluster only

	UpPattern, DownPattern, PausedPattern string
	Up, Down, Paused                      *regexp.Regexp

	LogDir      string
	UserExits   map[string][]UserExit
	UserExitDir string
	LSF         *LSF
}

// Label returns "N:name" for messages.
func (t *Tier) Label() string { return fmt.Sprintf("%d:%s", t.Index, t.Name) }

// Lookup finds a tier by 1-based number or by name (case-insensitive).
func (c *Config) Lookup(ref string) (*Tier, error) {
	if n, err := strconv.Atoi(ref); err == nil {
		if n < 1 || n > len(c.Tiers) {
			return nil, fmt.Errorf("tier %d does not exist (valid: 1-%d)", n, len(c.Tiers))
		}
		return c.Tiers[n-1], nil
	}
	var names []string
	for _, t := range c.Tiers {
		if strings.EqualFold(t.Name, ref) {
			return t, nil
		}
		names = append(names, t.Name)
	}
	return nil, fmt.Errorf("no tier named %q (tiers: %s)", ref, strings.Join(names, ", "))
}

// ValidationError collects every problem found, so users can fix them in one pass.
type ValidationError struct {
	Path     string
	Problems []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("configuration %s is not valid:\n  - %s", e.Path, strings.Join(e.Problems, "\n  - "))
}

// Load reads, parses and validates a configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, string(data))
}

// Parse parses configuration text. path is used for messages and naming.
func Parse(path, src string) (*Config, error) {
	root, err := yamlite.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	rm, ok := root.(*yamlite.Map)
	if !ok {
		return nil, fmt.Errorf("%s: top level must be a set of 'key: value' entries", path)
	}
	v := &validator{}
	cfg := &Config{Path: path}
	base := filepath.Base(path)
	cfg.Name = strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")

	v.allowKeys(rm, "", "version", "settings", "defaults", "tiers")
	if ver, ok := rm.Get("version"); ok {
		if s, _ := ver.(string); s != "1" {
			v.add(rm.Line("version"), "version: only version 1 is supported")
		}
	} else {
		v.add(0, "version: missing (add 'version: 1' at the top of the file)")
	}

	sm := v.mapAt(rm, "settings")
	cfg.Settings = Settings{
		StatusRoot:     v.str(sm, "status_root", "/var/tmp/sas_lsm"),
		SSHOptions:     v.strList(sm, "ssh_options", []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=accept-new"}),
		SSHMultiplex:   v.boolean(sm, "ssh_multiplex", true),
		AdminEmail:     v.strList(sm, "admin_email", nil),
		MailCommand:    v.str(sm, "mail_command", "mailx"),
		ArchiveReports: v.boolean(sm, "archive_reports", true),
		CollectLogs:    v.boolean(sm, "collect_logs", false),
		LogMinutes:     v.integer(sm, "log_minutes", 60, 1),
		LogMaxLines:    v.integer(sm, "log_max_lines", 5000, 10),
	}
	v.allowKeys(sm, "settings.", "status_root", "ssh_options", "ssh_multiplex", "admin_email", "mail_command",
		"archive_reports", "collect_logs", "log_minutes", "log_max_lines")

	dm := v.mapAt(rm, "defaults")
	v.allowKeys(dm, "defaults.", tierInheritable...)
	def := tierDefaults{
		user:      v.str(dm, "user", currentUser()),
		configdir: v.str(dm, "configdir", ""),
		parallel:  v.boolean(dm, "parallel", false),
		retries:   v.integer(dm, "retries", 10, 0),
		interval:  v.seconds(dm, "retry_interval", 6),
		delay:     v.seconds(dm, "tier_delay", 15),
		timeout:   v.seconds(dm, "command_timeout", 900),
		okCodes:   v.intList(dm, "ok_exit_codes", []int{0, 1, 2, 4}),
	}

	tv, ok := rm.Get("tiers")
	if !ok {
		v.add(0, "tiers: missing (at least one tier is required)")
	}
	list, _ := tv.([]any)
	if ok && len(list) == 0 {
		v.add(rm.Line("tiers"), "tiers: must be a list with at least one tier ('- name: ...')")
	}
	seen := map[string]int{}
	for i, item := range list {
		tm, ok := item.(*yamlite.Map)
		if !ok {
			v.add(rm.Line("tiers"), "tiers[%d]: each tier must be a set of 'key: value' entries", i+1)
			continue
		}
		t := v.tier(tm, i+1, def)
		if t == nil {
			continue
		}
		key := strings.ToLower(t.Name)
		if prev, dup := seen[key]; dup {
			v.add(t.Line, "tier %q: name already used by tier %d", t.Name, prev)
		}
		seen[key] = t.Index
		cfg.Tiers = append(cfg.Tiers, t)
	}
	clusters := 0
	for _, t := range cfg.Tiers {
		if t.Type == TypeMetadataCluster {
			clusters++
		}
	}
	if clusters > 1 {
		v.add(0, "only one metadata-cluster tier is allowed per configuration")
	}
	if len(v.problems) > 0 {
		return nil, &ValidationError{Path: path, Problems: v.problems}
	}
	return cfg, nil
}

var tierInheritable = []string{"user", "configdir", "parallel", "retries", "retry_interval", "tier_delay", "command_timeout", "ok_exit_codes"}

type tierDefaults struct {
	user, configdir          string
	parallel                 bool
	retries                  int
	interval, delay, timeout time.Duration
	okCodes                  []int
}

func (v *validator) tier(m *yamlite.Map, idx int, d tierDefaults) *Tier {
	t := &Tier{Index: idx, Line: firstLine(m)}
	t.Name = v.str(m, "name", "")
	where := fmt.Sprintf("tier %d", idx)
	if t.Name == "" {
		v.add(t.Line, "%s: 'name' is required", where)
		t.Name = fmt.Sprintf("tier%d", idx)
	} else {
		where = fmt.Sprintf("tier %d (%s)", idx, t.Name)
	}
	t.Type = v.str(m, "type", TypeSASServers)
	t.User = v.str(m, "user", d.user)
	t.ConfigDir = strings.TrimRight(v.str(m, "configdir", d.configdir), "/")
	t.Parallel = v.boolean(m, "parallel", d.parallel)
	t.Retries = v.integer(m, "retries", d.retries, 0)
	t.RetryInterval = v.seconds(m, "retry_interval", int(d.interval/time.Second))
	t.TierDelay = v.seconds(m, "tier_delay", int(d.delay/time.Second))
	t.CommandTimeout = v.seconds(m, "command_timeout", int(d.timeout/time.Second))
	t.OKExitCodes = v.intList(m, "ok_exit_codes", d.okCodes)
	t.Hosts = v.strList(m, "hosts", nil)
	t.LogDir = v.str(m, "logdir", "")
	t.UserExitDir = v.str(m, "userexit_dir", "")
	if t.User == "" {
		v.add(t.Line, "%s: no user set and the current user could not be determined", where)
	}
	if t.User == "root" {
		v.add(t.Line, "%s: user must be the SAS installer account, not root", where)
	}

	common := []string{"name", "type", "hosts", "logdir", "userexits", "userexit_dir", "start", "stop", "restart", "status",
		"up_pattern", "down_pattern"}
	common = append(common, tierInheritable...)

	sub := func(s string) string {
		s = strings.ReplaceAll(s, "{configdir}", t.ConfigDir)
		return strings.ReplaceAll(s, "{user}", t.User)
	}
	override := func(key, def string) string { return sub(v.str(m, key, def)) }
	needConfigDir := func() {
		if t.ConfigDir == "" {
			v.add(t.Line, "%s: 'configdir' is required for type %s (set it here or under defaults:)", where, t.Type)
		}
	}

	switch t.Type {
	case TypeSASServers:
		v.allowKeys(m, where+": ", common...)
		needConfigDir()
		t.Start = override("start", "{configdir}/sas.servers start")
		t.Stop = override("stop", "{configdir}/sas.servers stop")
		t.Restart = override("restart", "")
		t.Status = override("status", "{configdir}/sas.servers status")
		t.UpPattern = v.str(m, "up_pattern", `is UP`)
		t.DownPattern = v.str(m, "down_pattern", `is NOT up`)
		if t.LogDir == "" {
			t.LogDir = t.ConfigDir
		}
	case TypeMetadataCluster:
		v.allowKeys(m, where+": ", common...)
		needConfigDir()
		ms := "{configdir}/SASMeta/MetadataServer/MetadataServer.sh"
		t.Start = override("start", "nohup "+ms+" start >/dev/null 2>&1 &")
		t.Stop = override("stop", ms+" stop")
		t.StopCluster = sub(ms + " stopCluster")
		t.Status = override("status", ms+" status 2>&1")
		t.UpPattern = v.str(m, "up_pattern", `is RUNNING on`)
		t.DownPattern = v.str(m, "down_pattern", `Server appears to be stopped`)
		t.PausedPattern = `is PAUSED on`
		if len(t.Hosts) < 3 {
			v.add(t.Line, "%s: a metadata cluster needs at least 3 hosts (found %d); use type sas-servers for a single metadata server", where, len(t.Hosts))
		}
		if t.LogDir == "" {
			t.LogDir = t.ConfigDir
		}
	case TypeLSFGrid:
		v.allowKeys(m, where+": ", append(common, "lsf")...)
		lm := v.mapAt(m, "lsf")
		if lm == nil {
			v.add(t.Line, "%s: type lsf-grid requires an 'lsf:' section", where)
			lm = &yamlite.Map{Values: map[string]any{}, Lines: map[string]int{}}
		}
		v.allowKeys(lm, where+" lsf: ", "master", "nodes", "envdir", "serverdir", "pm_host", "pm_envdir", "pm_serverdir",
			"ego_controls_daemons", "sudo")
		l := &LSF{
			Master:             v.str(lm, "master", ""),
			Nodes:              v.strList(lm, "nodes", nil),
			EnvDir:             v.str(lm, "envdir", ""),
			ServerDir:          v.str(lm, "serverdir", ""),
			PMHost:             v.str(lm, "pm_host", ""),
			PMEnvDir:           v.str(lm, "pm_envdir", ""),
			PMServerDir:        v.str(lm, "pm_serverdir", ""),
			EGOControlsDaemons: v.boolean(lm, "ego_controls_daemons", false),
			Sudo:               v.boolean(lm, "sudo", true),
		}
		for _, kv := range [][2]string{{"master", l.Master}, {"envdir", l.EnvDir}, {"serverdir", l.ServerDir}} {
			if kv[1] == "" {
				v.add(t.Line, "%s: lsf.%s is required", where, kv[0])
			}
		}
		if l.PMHost != "" && (l.PMEnvDir == "" || l.PMServerDir == "") {
			v.add(t.Line, "%s: lsf.pm_envdir and lsf.pm_serverdir are required when lsf.pm_host is set", where)
		}
		if len(t.Hosts) > 0 {
			v.add(m.Line("hosts"), "%s: do not set 'hosts' for lsf-grid; use lsf.master, lsf.nodes and lsf.pm_host", where)
		}
		t.Hosts = uniq(append(append([]string{l.Master}, l.Nodes...), l.PMHost))
		t.LSF = l
	case TypeCustom:
		v.allowKeys(m, where+": ", common...)
		for _, k := range []string{"start", "stop", "status", "up_pattern", "down_pattern"} {
			if _, ok := m.Get(k); !ok {
				v.add(t.Line, "%s: type custom requires '%s'", where, k)
			}
		}
		t.Start = override("start", "")
		t.Stop = override("stop", "")
		t.Restart = override("restart", "")
		t.Status = override("status", "")
		t.UpPattern = v.str(m, "up_pattern", "")
		t.DownPattern = v.str(m, "down_pattern", "")
	default:
		v.add(m.Line("type"), "%s: unknown type %q (valid: %s, %s, %s, %s)", where, t.Type,
			TypeSASServers, TypeMetadataCluster, TypeLSFGrid, TypeCustom)
		return nil
	}
	if t.Type != TypeLSFGrid && len(t.Hosts) == 0 {
		v.add(t.Line, "%s: 'hosts' must list at least one host", where)
	}
	t.LogDir = sub(t.LogDir)
	t.UserExitDir = sub(t.UserExitDir)
	t.Up = v.regex(t.Line, where, "up_pattern", t.UpPattern)
	t.Down = v.regex(t.Line, where, "down_pattern", t.DownPattern)
	t.Paused = v.regex(t.Line, where, "paused", t.PausedPattern)
	t.UserExits = v.userExits(m, where, sub)
	return t
}

func (v *validator) userExits(m *yamlite.Map, where string, sub func(string) string) map[string][]UserExit {
	out := map[string][]UserExit{}
	um := v.mapAt(m, "userexits")
	if um == nil {
		return out
	}
	v.allowKeys(um, where+" userexits: ", Events...)
	for _, ev := range um.Keys {
		items, ok := um.Values[ev].([]any)
		if !ok {
			if s, isStr := um.Values[ev].(string); isStr && s != "" {
				items = []any{s}
			} else {
				v.add(um.Line(ev), "%s: userexits.%s must be a list", where, ev)
				continue
			}
		}
		for _, it := range items {
			switch x := it.(type) {
			case string:
				out[ev] = append(out[ev], UserExit{Run: sub(x), On: "first"})
			case *yamlite.Map:
				v.allowKeys(x, where+" userexits."+ev+": ", "run", "required", "on")
				ue := UserExit{Run: sub(v.str(x, "run", "")), Required: v.boolean(x, "required", false), On: v.str(x, "on", "first")}
				if ue.Run == "" {
					v.add(firstLine(x), "%s: userexits.%s entry needs 'run:'", where, ev)
				}
				if ue.On != "first" && ue.On != "all" {
					v.add(x.Line("on"), "%s: userexits.%s 'on' must be first or all", where, ev)
				}
				out[ev] = append(out[ev], ue)
			default:
				v.add(um.Line(ev), "%s: userexits.%s has an invalid entry", where, ev)
			}
		}
	}
	return out
}

// ---- typed accessors that record problems instead of failing fast ----

type validator struct{ problems []string }

func (v *validator) add(line int, f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	if line > 0 {
		msg = fmt.Sprintf("line %d: %s", line, msg)
	}
	v.problems = append(v.problems, msg)
}

func (v *validator) allowKeys(m *yamlite.Map, prefix string, allowed ...string) {
	if m == nil {
		return
	}
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	for _, k := range m.Keys {
		if !ok[k] {
			sorted := append([]string(nil), allowed...)
			sort.Strings(sorted)
			v.add(m.Line(k), "%sunknown setting %q (allowed: %s)", prefix, k, strings.Join(sorted, ", "))
		}
	}
}

func (v *validator) mapAt(m *yamlite.Map, key string) *yamlite.Map {
	val, ok := m.Get(key)
	if !ok {
		return nil
	}
	if s, isStr := val.(string); isStr && s == "" {
		return nil
	}
	mm, ok := val.(*yamlite.Map)
	if !ok {
		v.add(m.Line(key), "%s: must be a set of indented 'key: value' entries", key)
		return nil
	}
	return mm
}

func (v *validator) str(m *yamlite.Map, key, def string) string {
	val, ok := m.Get(key)
	if !ok {
		return def
	}
	s, ok := val.(string)
	if !ok {
		v.add(m.Line(key), "%s: must be a single value, not a list or section", key)
		return def
	}
	return s
}

func (v *validator) strList(m *yamlite.Map, key string, def []string) []string {
	val, ok := m.Get(key)
	if !ok {
		return def
	}
	switch x := val.(type) {
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, it := range x {
			s, ok := it.(string)
			if !ok || s == "" {
				v.add(m.Line(key), "%s: every list item must be a non-empty value", key)
				continue
			}
			out = append(out, s)
		}
		return out
	}
	v.add(m.Line(key), "%s: must be a value or a list", key)
	return def
}

func (v *validator) integer(m *yamlite.Map, key string, def, min int) int {
	val, ok := m.Get(key)
	if !ok {
		return def
	}
	s, _ := val.(string)
	n, err := strconv.Atoi(s)
	if err != nil || n < min {
		v.add(m.Line(key), "%s: must be a whole number >= %d (found %q)", key, min, s)
		return def
	}
	return n
}

func (v *validator) seconds(m *yamlite.Map, key string, def int) time.Duration {
	return time.Duration(v.integer(m, key, def, 0)) * time.Second
}

func (v *validator) intList(m *yamlite.Map, key string, def []int) []int {
	if _, ok := m.Get(key); !ok {
		return def
	}
	var out []int
	for _, s := range v.strList(m, key, nil) {
		n, err := strconv.Atoi(s)
		if err != nil {
			v.add(m.Line(key), "%s: %q is not a whole number", key, s)
			continue
		}
		out = append(out, n)
	}
	return out
}

func (v *validator) boolean(m *yamlite.Map, key string, def bool) bool {
	val, ok := m.Get(key)
	if !ok {
		return def
	}
	s, _ := val.(string)
	switch strings.ToLower(s) {
	case "true", "yes", "on":
		return true
	case "false", "no", "off":
		return false
	}
	v.add(m.Line(key), "%s: must be true or false (found %q)", key, s)
	return def
}

func (v *validator) regex(line int, where, key, pat string) *regexp.Regexp {
	if pat == "" {
		return nil
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		v.add(line, "%s: %s is not a valid regular expression: %v", where, key, err)
		return nil
	}
	return re
}

func firstLine(m *yamlite.Map) int {
	if m == nil || len(m.Keys) == 0 {
		return 0
	}
	return m.Lines[m.Keys[0]]
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func currentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}
