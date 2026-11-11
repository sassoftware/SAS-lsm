// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/sassoftware/sas-lsm/internal/config"
	"github.com/sassoftware/sas-lsm/internal/runner"
)

// PlannedExit is a userexit resolved for one event.
type PlannedExit struct {
	config.UserExit
	Source string // "config" or the legacy directory
}

// ResolveUserExits returns the scripts that will run for a tier event, in
// order: scripts listed in the configuration first, then scripts found in
// userexit_dir (on the tier's first host) using the SAS_lsm 4.x naming convention.
func (e *Engine) ResolveUserExits(t *config.Tier, event string) ([]PlannedExit, error) {
	var out []PlannedExit
	for _, ue := range t.UserExits[event] {
		out = append(out, PlannedExit{UserExit: ue, Source: "config"})
	}
	if t.UserExitDir == "" || len(t.Hosts) == 0 {
		return out, nil
	}
	cmd := "find " + runner.ShellQuote(t.UserExitDir) + " -type f 2>/dev/null"
	res := e.R.Run(e.ctx, t.User, t.Hosts[0], cmd, t.CommandTimeout)
	if res.Err != nil {
		return out, fmt.Errorf("listing %s on %s: %v", t.UserExitDir, t.Hosts[0], res.Err)
	}
	var files []string
	for _, f := range strings.Split(res.Output, "\n") {
		f = strings.TrimSpace(f)
		if f != "" && legacyMatch(path.Base(f), event) {
			files = append(files, f)
		}
	}
	sort.Strings(files) // portable replacement for 'sort -z' (not available on Solaris)
	for _, f := range files {
		req := strings.Contains(strings.ToLower(path.Base(f)), "_required_")
		out = append(out, PlannedExit{UserExit: config.UserExit{Run: f, Required: req, On: "first"}, Source: t.UserExitDir})
	}
	return out, nil
}

// legacyMatch implements the SAS_lsm 4.x file naming convention
// NN_<pre|post>_<start|stop|status>[_<success|failure>]_name, case-insensitively.
// A post script without success/failure runs after either outcome.
func legacyMatch(name, event string) bool {
	n := "_" + strings.ToLower(name)
	parts := strings.Split(event, "_") // pre_start | post_start_success | post_status
	base := "_" + parts[0] + "_" + parts[1] + "_"
	if len(parts) == 2 {
		return strings.Contains(n, base)
	}
	if strings.Contains(n, base+parts[2]+"_") {
		return true
	}
	return strings.Contains(n, base) && !strings.Contains(n, base+"success_") && !strings.Contains(n, base+"failure_")
}

// userExits runs the scripts for an event. A failing script marked required
// returns an error; other failures are reported as warnings.
func (e *Engine) userExits(t *config.Tier, event string) error {
	if e.Opt.NoUserExits {
		return nil
	}
	list, err := e.ResolveUserExits(t, event)
	if err != nil {
		e.Rep.Printf("    warning: userexits for %s: %v", event, err)
	}
	if len(list) == 0 {
		return nil
	}
	e.Rep.Printf("    userexits: %s (%d script(s))", event, len(list))
	for _, ue := range list {
		hosts := t.Hosts[:1]
		if ue.On == "all" {
			hosts = t.Hosts
		}
		for _, h := range hosts {
			res := e.R.Run(e.ctx, t.User, h, strings.ReplaceAll(ue.Run, "{host}", h), t.CommandTimeout)
			e.Rep.Printf("    >> [%s@%s] %s  -> exit %d", t.User, h, ue.Run, res.ExitCode)
			if out := strings.TrimSpace(res.Output); out != "" {
				e.Rep.Filef("%s", Indent(out, "       | "))
			}
			if res.Err == nil && res.ExitCode == 0 {
				continue
			}
			why := res.Err
			if why == nil {
				why = fmt.Errorf("exit code %d", res.ExitCode)
			}
			if ue.Required {
				return fmt.Errorf("%s on %s: %v", ue.Run, h, why)
			}
			e.Rep.Printf("    warning: userexit %s failed on %s (%v); not required, continuing", ue.Run, h, why)
		}
	}
	return nil
}
