// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sassoftware/sas-lsm/internal/config"
	"github.com/sassoftware/sas-lsm/internal/runner"
)

// exec runs an action command on a tier host, logs it, and checks the exit code.
func (e *Engine) exec(t *config.Tier, host, cmd string) runner.Result {
	return e.run(t, host, cmd, true)
}

// check runs a status command. It is logged to the report file only, to keep
// the console readable (results are summarised by printState).
func (e *Engine) check(t *config.Tier, host, cmd string) runner.Result {
	return e.run(t, host, cmd, false)
}

func (e *Engine) run(t *config.Tier, host, cmd string, show bool) runner.Result {
	cmd = strings.ReplaceAll(cmd, "{host}", host)
	where := t.User + "@" + host
	if e.R.IsLocal(t.User, host) {
		where += " (local)"
	}
	if show {
		e.Rep.Printf("    >> [%s] %s", where, cmd)
	} else {
		e.Rep.Filef("    >> [%s] %s", where, cmd)
	}
	res := e.R.Run(e.ctx, t.User, host, cmd, t.CommandTimeout)
	if out := strings.TrimSpace(res.Output); out != "" {
		e.Rep.Filef("%s", Indent(out, "       | "))
	}
	if res.Err == nil && !slices.Contains(t.OKExitCodes, res.ExitCode) {
		res.Err = fmt.Errorf("exit code %d (accepted: %v)", res.ExitCode, t.OKExitCodes)
	}
	if res.Err != nil {
		e.Rep.Printf("    !! [%s] %v", host, res.Err)
	}
	return res
}

// eachHost runs fn for every host, in parallel when requested.
func (e *Engine) eachHost(hosts []string, parallel bool, fn func(host string) error) error {
	var mu sync.Mutex
	var errs []string
	record := func(h string, err error) {
		if err != nil {
			mu.Lock()
			errs = append(errs, fmt.Sprintf("%s: %v", h, err))
			mu.Unlock()
		}
	}
	if !parallel || len(hosts) < 2 {
		for _, h := range hosts {
			record(h, fn(h))
		}
	} else {
		var wg sync.WaitGroup
		for _, h := range hosts {
			wg.Add(1)
			go func(h string) {
				defer wg.Done()
				record(h, fn(h))
			}(h)
		}
		wg.Wait()
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

// ---------- generic actions ----------

func (e *Engine) startTier(t *config.Tier) error {
	switch t.Type {
	case config.TypeLSFGrid:
		return e.lsfStart(t)
	case config.TypeMetadataCluster:
		// cluster nodes must start together to form a quorum
		return e.eachHost(t.Hosts, true, func(h string) error { return e.exec(t, h, t.Start).Err })
	}
	return e.eachHost(t.Hosts, t.Parallel, func(h string) error { return e.exec(t, h, t.Start).Err })
}

func (e *Engine) stopTier(t *config.Tier) error {
	switch t.Type {
	case config.TypeLSFGrid:
		return e.lsfStop(t)
	case config.TypeMetadataCluster:
		return e.clusterStop(t)
	}
	return e.eachHost(t.Hosts, t.Parallel, func(h string) error { return e.exec(t, h, t.Stop).Err })
}

// probe runs one status check of the tier (no waiting).
func (e *Engine) probe(t *config.Tier) TierState {
	switch t.Type {
	case config.TypeLSFGrid:
		return e.lsfProbe(t)
	case config.TypeMetadataCluster:
		return e.clusterProbe(t)
	}
	hs := e.probeHosts(t, t.Hosts, t.Status)
	return TierState{State: combine(hs), Hosts: hs}
}

func (e *Engine) probeHosts(t *config.Tier, hosts []string, cmd string) []HostState {
	out := make([]HostState, len(hosts))
	idx := map[string]int{}
	for i, h := range hosts {
		idx[h] = i
	}
	e.eachHost(hosts, t.Parallel || t.Type == config.TypeMetadataCluster, func(h string) error {
		res := e.check(t, h, cmd)
		hs := HostState{Host: h}
		hs.State, hs.Lines = classify(res.Output, t.Up, t.Down)
		if t.Paused != nil && hs.State != Up && t.Paused.MatchString(res.Output) {
			hs.State, hs.Extra = Down, "paused"
		}
		if res.Err != nil && hs.State == Unknown {
			hs.Err = res.Err
		}
		out[idx[h]] = hs
		return nil
	})
	return out
}

// waitFor polls until the tier reaches want or retries are exhausted.
func (e *Engine) waitFor(t *config.Tier, want State) TierState {
	var st TierState
	for try := 0; ; try++ {
		st = e.probe(t)
		if st.State == want || try >= t.Retries || e.ctx.Err() != nil {
			return st
		}
		e.Rep.Printf("    .. tier %s is %s, waiting for %s (check %d of %d, next in %s)",
			t.Label(), st.State, want, try+1, t.Retries, t.RetryInterval)
		if !e.sleep(t.RetryInterval) {
			return st
		}
	}
}

func (e *Engine) printState(st TierState) {
	for _, h := range st.Hosts {
		name := h.Host
		if h.Extra != "" {
			name += " (" + h.Extra + ")"
		}
		e.Rep.Printf("    %-8s %s", h.State, name)
		for _, l := range h.Lines {
			e.Rep.Printf("               %s", l)
		}
		if h.Err != nil {
			e.Rep.Printf("               error: %v", h.Err)
		}
	}
	if st.Note != "" {
		e.Rep.Printf("    note: %s", st.Note)
	}
}

// ---------- metadata cluster ----------

// clusterProbe applies the SAS metadata cluster quorum rule: the cluster is
// UP when a majority of nodes run (or exactly half, including the first node).
func (e *Engine) clusterProbe(t *config.Tier) TierState {
	hs := e.probeHosts(t, t.Hosts, t.Status)
	up, down := 0, 0
	for _, h := range hs {
		switch h.State {
		case Up:
			up++
		case Down:
			down++
		default:
			return TierState{State: Unknown, Hosts: hs, Note: "one or more cluster nodes returned an unrecognized status"}
		}
	}
	n := len(hs)
	switch {
	case up == n:
		return TierState{State: Up, Hosts: hs}
	case down == n:
		return TierState{State: Down, Hosts: hs}
	case up > down || (up == down && hs[0].State == Up):
		return TierState{State: Up, Hosts: hs, Note: fmt.Sprintf("quorum achieved (%d of %d nodes running); cluster is degraded", up, n)}
	}
	return TierState{State: Partial, Hosts: hs, Note: fmt.Sprintf("no quorum (%d of %d nodes running)", up, n)}
}

// clusterStop issues stopCluster on one running node, then stops any node
// that is still running individually.
func (e *Engine) clusterStop(t *config.Tier) error {
	st := e.clusterProbe(t)
	var running []string
	for _, h := range st.Hosts {
		if h.State == Up {
			running = append(running, h.Host)
		}
	}
	if len(running) == 0 {
		e.Rep.Printf("    no cluster node is running; nothing to stop")
		return nil
	}
	e.exec(t, running[0], t.StopCluster)
	st = e.waitFor(t, Down)
	if st.State == Down {
		return nil
	}
	var left []string
	for _, h := range st.Hosts {
		if h.State != Down {
			left = append(left, h.Host)
		}
	}
	e.Rep.Printf("    stopCluster did not stop every node; stopping %s individually", strings.Join(left, ", "))
	return e.eachHost(left, true, func(h string) error { return e.exec(t, h, t.Stop).Err })
}

// ---------- Platform LSF grid ----------

func (e *Engine) lsfCmd(t *config.Tier, envFile, cmd string) string {
	inner := ". " + envFile + " && " + cmd
	if t.LSF.Sudo {
		return "sudo -n /bin/sh -c " + runner.ShellQuote(inner)
	}
	return inner
}

func (e *Engine) lsfHostStart(t *config.Tier, h string) error {
	env := t.LSF.EnvDir + "/profile.lsf"
	if err := e.exec(t, h, e.lsfCmd(t, env, "lsadmin limstartup")).Err; err != nil {
		return err
	}
	if t.LSF.EGOControlsDaemons {
		return nil // EGO starts res and sbatchd
	}
	if err := e.exec(t, h, e.lsfCmd(t, env, "lsadmin resstartup")).Err; err != nil {
		return err
	}
	return e.exec(t, h, e.lsfCmd(t, env, "badmin hstartup")).Err
}

func (e *Engine) lsfHostStop(t *config.Tier, h string) error {
	env := t.LSF.EnvDir + "/profile.lsf"
	var errs []string
	for _, c := range []string{"badmin hshutdown", "lsadmin resshutdown", "lsadmin limshutdown"} {
		if err := e.exec(t, h, e.lsfCmd(t, env, c)).Err; err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (e *Engine) lsfStart(t *config.Tier) error {
	l := t.LSF
	e.Rep.Printf("    starting LSF on master %s", l.Master)
	if err := e.lsfHostStart(t, l.Master); err != nil {
		return err
	}
	if len(l.Nodes) > 0 {
		e.Rep.Printf("    starting LSF on %d node(s)", len(l.Nodes))
		if err := e.eachHost(l.Nodes, t.Parallel, func(h string) error { return e.lsfHostStart(t, h) }); err != nil {
			return err
		}
	}
	if l.PMHost != "" {
		e.Rep.Printf("    starting Process Manager on %s", l.PMHost)
		return e.exec(t, l.PMHost, e.lsfCmd(t, l.PMEnvDir+"/profile.js", "jadmin start")).Err
	}
	return nil
}

func (e *Engine) lsfStop(t *config.Tier) error {
	l := t.LSF
	var errs []string
	if l.PMHost != "" {
		e.Rep.Printf("    stopping Process Manager on %s", l.PMHost)
		if err := e.exec(t, l.PMHost, e.lsfCmd(t, l.PMEnvDir+"/profile.js", "jadmin stop")).Err; err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(l.Nodes) > 0 {
		e.Rep.Printf("    stopping LSF on %d node(s)", len(l.Nodes))
		if err := e.eachHost(l.Nodes, t.Parallel, func(h string) error { return e.lsfHostStop(t, h) }); err != nil {
			errs = append(errs, err.Error())
		}
	}
	e.Rep.Printf("    stopping LSF on master %s", l.Master)
	if err := e.lsfHostStop(t, l.Master); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// psCheck builds a portable (Linux/AIX/Solaris) process check that prints
// "<daemon> UP" or "<daemon> DOWN" for each daemon.
func psCheck(dir string, daemons ...string) string {
	var parts []string
	for _, d := range daemons {
		pat := runner.ShellQuote(dir + "/" + d + "( |$)")
		parts = append(parts, fmt.Sprintf("if ps -eo args | grep -v grep | grep -E %s >/dev/null 2>&1; then echo '%s UP'; else echo '%s DOWN'; fi", pat, d, d))
	}
	return strings.Join(parts, "; ")
}

func (e *Engine) lsfProbe(t *config.Tier) TierState {
	l := t.LSF
	type probe struct{ host, cmd string }
	base := []string{"lim", "res", "sbatchd"}
	probes := []probe{{l.Master, psCheck(l.ServerDir, append(append([]string{}, base...), "mbatchd", "mbschd")...)}}
	for _, n := range l.Nodes {
		if n != l.Master {
			probes = append(probes, probe{n, psCheck(l.ServerDir, base...)})
		}
	}
	if l.PMHost != "" {
		probes = append(probes, probe{l.PMHost, psCheck(l.PMServerDir, "jfd")})
	}
	hs := make([]HostState, len(probes))
	var wg sync.WaitGroup
	run := func(i int, p probe) {
		res := e.check(t, p.host, p.cmd)
		var lines []string
		up, down := 0, 0
		for _, ln := range strings.Split(strings.TrimSpace(res.Output), "\n") {
			switch {
			case strings.HasSuffix(ln, " UP"):
				up++
				lines = append(lines, ln)
			case strings.HasSuffix(ln, " DOWN"):
				down++
				lines = append(lines, ln)
			}
		}
		st := Unknown
		switch {
		case up > 0 && down == 0:
			st = Up
		case down > 0 && up == 0:
			st = Down
		case up > 0 && down > 0:
			st = Partial
		}
		hs[i] = HostState{Host: p.host, State: st, Lines: lines}
		if st == Unknown {
			hs[i].Err = res.Err
		}
	}
	for i, p := range probes {
		if t.Parallel {
			wg.Add(1)
			go func(i int, p probe) { defer wg.Done(); run(i, p) }(i, p)
		} else {
			run(i, p)
		}
	}
	wg.Wait()
	return TierState{State: combine(hs), Hosts: hs}
}

func (e *Engine) sleep(d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-e.ctx.Done():
		return false
	}
}
