// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package engine implements ordered start, stop and status of SAS tiers.
package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sassoftware/sas-lsm/internal/config"
	"github.com/sassoftware/sas-lsm/internal/runner"
)

// Exit codes.
const (
	ExitOK         = 0
	ExitFailed     = 1 // an action or status check failed
	ExitDependency = 2 // start refused: dependency check failed
	ExitConfig     = 3 // invalid configuration or usage
	ExitUnknown    = 4 // state could not be determined
	ExitLocked     = 5 // another SAS_lsm run is active for this configuration
)

// Options control a run.
type Options struct {
	NoUserExits bool
	CollectLogs bool
}

// Engine runs actions against one configuration.
type Engine struct {
	Cfg       *config.Config
	R         *runner.Runner
	Rep       *Reporter
	Opt       Options
	StatusDir string
	ctx       context.Context
	failed    []*config.Tier
	problem   string // first problem description, used in the log analysis
}

// New prepares an engine (creates the status directory).
func New(ctx context.Context, cfg *config.Config, opt Options) (*Engine, error) {
	dir := filepath.Join(cfg.Settings.StatusRoot, cfg.Name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("cannot create status directory %s: %w (check settings.status_root)", dir, err)
	}
	return &Engine{
		Cfg:       cfg,
		R:         runner.New(cfg.Settings.SSHOptions, cfg.Settings.SSHMultiplex),
		Opt:       opt,
		StatusDir: dir,
		ctx:       ctx,
	}, nil
}

// Lock prevents two runs against the same configuration at once. mkdir is
// atomic on every platform (flock is not available on AIX/Solaris by default).
func (e *Engine) Lock() (func(), error) {
	lock := filepath.Join(e.StatusDir, ".lock")
	for attempt := 0; attempt < 2; attempt++ {
		if err := os.Mkdir(lock, 0o750); err == nil {
			os.WriteFile(filepath.Join(lock, "pid"), []byte(strconv.Itoa(os.Getpid())), 0o640)
			return func() { os.RemoveAll(lock) }, nil
		}
		b, _ := os.ReadFile(filepath.Join(lock, "pid"))
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 && processAlive(pid) {
			return nil, fmt.Errorf("another SAS_lsm run (pid %d) is active for this configuration (lock: %s)", pid, lock)
		}
		os.RemoveAll(lock) // stale lock from a run that was killed
	}
	return nil, fmt.Errorf("could not acquire lock %s", lock)
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func (e *Engine) lastAction() string {
	b, err := os.ReadFile(filepath.Join(e.StatusDir, ".lastaction"))
	if err != nil {
		return "stop"
	}
	action, _, _ := strings.Cut(strings.TrimSpace(string(b)), ":")
	return action
}

func (e *Engine) setLastAction(action string, rc int) int {
	os.WriteFile(filepath.Join(e.StatusDir, ".lastaction"), []byte(fmt.Sprintf("%s:%d\n", action, rc)), 0o640)
	return rc
}

func (e *Engine) fail(t *config.Tier, why string) {
	e.failed = append(e.failed, t)
	if e.problem == "" {
		e.problem = fmt.Sprintf("tier %s: %s", t.Label(), why)
	}
}

// ---------------------------------------------------------------- start

// Start starts tiers from tier `from` to the last tier. Tiers before `from`
// must already be UP and tiers after it must be DOWN.
func (e *Engine) Start(from *config.Tier) int {
	tiers := e.Cfg.Tiers
	e.Rep.Event("START from tier %s to tier %s", from.Label(), tiers[len(tiers)-1].Label())
	e.Rep.Printf("")

	e.Rep.Event("checking dependencies")
	ok := true
	for _, t := range tiers {
		if t.Index == from.Index {
			continue
		}
		want := Down
		if t.Index < from.Index {
			want = Up
		}
		st := e.probe(t)
		mark := "ok"
		if st.State != want {
			mark, ok = "FAILED", false
		}
		e.Rep.Printf("  tier %-24s expected %-4s found %-7s %s", t.Label(), want, st.State, mark)
		if st.State != want {
			e.printState(st)
		}
	}
	if !ok {
		e.Rep.Printf("")
		e.Rep.Printf("<<FAILURE>> dependency check failed; nothing was started.")
		e.Rep.Printf("            Tiers before %s must be UP and tiers after it must be DOWN.", from.Label())
		e.Rep.Printf("            Use 'status' to review, or start from an earlier tier.")
		return e.setLastAction("start", ExitDependency)
	}
	e.Rep.Printf("")

	for _, t := range tiers[from.Index-1:] {
		e.Rep.Event("tier %s: start (%s on %s)", t.Label(), t.Type, strings.Join(t.Hosts, ", "))
		if err := e.userExits(t, "pre_start"); err != nil {
			e.fail(t, err.Error())
			e.Rep.Printf("<<FAILURE>> required pre_start userexit failed: %v", err)
			return e.setLastAction("start", ExitFailed)
		}
		if st := e.probe(t); t.Index == from.Index && st.State != Down {
			e.Rep.Printf("    tier is %s; restarting it", st.State)
			if t.Restart != "" {
				e.eachHost(t.Hosts, t.Parallel, func(h string) error { return e.exec(t, h, t.Restart).Err })
			} else {
				e.stopTier(t)
				if st := e.waitFor(t, Down); st.State != Down {
					e.printState(st)
					e.Rep.Printf("    warning: tier did not reach DOWN before restart; starting anyway")
				}
				e.startTier(t)
			}
		} else {
			e.startTier(t)
		}
		st := e.waitFor(t, Up)
		e.printState(st)
		if st.State != Up {
			e.Rep.Event("tier %s: start FAILED (state %s)", t.Label(), st.State)
			e.userExits(t, "post_start_failure")
			e.fail(t, "did not reach UP after start (state "+st.State.String()+")")
			e.collectLogs()
			e.Rep.Printf("")
			e.Rep.Printf("<<FAILURE>> tier %s did not start. Later tiers were not started.", t.Label())
			return e.setLastAction("start", ExitFailed)
		}
		e.Rep.Event("tier %s: start succeeded", t.Label())
		if err := e.userExits(t, "post_start_success"); err != nil {
			e.fail(t, err.Error())
			e.Rep.Printf("<<FAILURE>> required post_start_success userexit failed: %v", err)
			return e.setLastAction("start", ExitFailed)
		}
		if t.Index < len(tiers) && t.TierDelay > 0 {
			e.Rep.Printf("    waiting %s before the next tier (tier_delay)", t.TierDelay)
			if !e.sleep(t.TierDelay) {
				return e.setLastAction("start", ExitFailed)
			}
		}
		e.Rep.Printf("")
	}
	e.Rep.Printf("<<SUCCESS>> deployment tier services are operational")
	return e.setLastAction("start", ExitOK)
}

// ---------------------------------------------------------------- stop

// Stop stops tiers from the last tier down to tier `to`.
func (e *Engine) Stop(to *config.Tier) int {
	tiers := e.Cfg.Tiers
	e.Rep.Event("STOP from tier %s down to tier %s", tiers[len(tiers)-1].Label(), to.Label())
	e.Rep.Printf("")
	for i := len(tiers) - 1; i >= to.Index-1; i-- {
		t := tiers[i]
		e.Rep.Event("tier %s: stop (%s on %s)", t.Label(), t.Type, strings.Join(t.Hosts, ", "))
		if err := e.userExits(t, "pre_stop"); err != nil {
			e.fail(t, err.Error())
			e.Rep.Printf("<<FAILURE>> required pre_stop userexit failed: %v", err)
			return e.setLastAction("stop", ExitFailed)
		}
		e.stopTier(t)
		st := e.waitFor(t, Down)
		e.printState(st)
		if st.State != Down {
			e.Rep.Event("tier %s: stop FAILED (state %s)", t.Label(), st.State)
			e.userExits(t, "post_stop_failure")
			e.fail(t, "did not reach DOWN after stop (state "+st.State.String()+")")
			e.collectLogs()
			e.Rep.Printf("")
			e.Rep.Printf("<<FAILURE>> tier %s did not stop. Earlier tiers were left running.", t.Label())
			return e.setLastAction("stop", ExitFailed)
		}
		e.Rep.Event("tier %s: stop succeeded", t.Label())
		if err := e.userExits(t, "post_stop_success"); err != nil {
			e.fail(t, err.Error())
			e.Rep.Printf("<<FAILURE>> required post_stop_success userexit failed: %v", err)
			return e.setLastAction("stop", ExitFailed)
		}
		e.Rep.Printf("")
	}
	e.Rep.Printf("<<SUCCESS>> deployment tier services are stopped from tier %s to tier %s",
		tiers[len(tiers)-1].Label(), to.Label())
	return e.setLastAction("stop", ExitOK)
}

// ---------------------------------------------------------------- status

// Status reports every tier. The exit code reflects whether the deployment
// matches the last start/stop action (all UP after a start).
func (e *Engine) Status() int {
	last := e.lastAction()
	e.Rep.Event("STATUS (last action: %s)", last)
	e.Rep.Printf("")
	up, down, unknown := 0, 0, 0
	for _, t := range e.Cfg.Tiers {
		e.userExits(t, "pre_status")
		var st TierState
		for try := 0; ; try++ {
			st = e.probe(t)
			if st.State != Unknown || try >= t.Retries || e.ctx.Err() != nil {
				break
			}
			e.Rep.Printf("    .. status of tier %s is unknown, retrying in %s", t.Label(), t.RetryInterval)
			e.sleep(t.RetryInterval)
		}
		e.Rep.Printf("  tier %-24s %s", t.Label(), st.State)
		e.printState(st)
		e.userExits(t, "post_status")
		switch st.State {
		case Up:
			up++
		case Down:
			down++
		case Unknown:
			unknown++
			e.fail(t, "status could not be determined")
		default:
			e.fail(t, "tier is only partly running")
		}
		e.Rep.Printf("")
	}
	n := len(e.Cfg.Tiers)
	rc := ExitOK
	switch {
	case unknown > 0:
		e.Rep.Printf("<<STATUS>> deployment tier services are in an UNKNOWN state")
		rc = ExitUnknown
	case up == n:
		e.Rep.Printf("<<STATUS>> deployment tier services are operational")
	case down == n:
		e.Rep.Printf("<<STATUS>> deployment tier services are NOT operational")
		if last == "start" {
			rc = ExitFailed
		}
	default:
		e.Rep.Printf("<<STATUS>> deployment tier services are NOT fully operational")
		if last == "start" {
			rc = ExitFailed
		}
	}
	if rc != ExitOK && last == "start" {
		e.collectLogs()
	}
	return rc
}

// ---------------------------------------------------------------- reporting

// Finish archives the report and e-mails it when configured.
func (e *Engine) Finish(action, reportPath string, rc int) {
	if e.Cfg.Settings.ArchiveReports && action != "status" {
		dir := filepath.Join(e.StatusDir, "archive")
		if os.MkdirAll(dir, 0o750) == nil {
			if b, err := os.ReadFile(reportPath); err == nil {
				os.WriteFile(filepath.Join(dir, time.Now().Format("2006-01-02-15.04.05")+".report."+action), b, 0o640)
			}
		}
	}
	if len(e.Cfg.Settings.AdminEmail) == 0 || (action == "status" && rc == ExitOK) {
		return
	}
	result := "SUCCESS"
	if rc != ExitOK {
		result = "FAILURE"
	}
	host, _ := os.Hostname()
	subject := fmt.Sprintf("SAS_lsm %s %s %s on %s", result, action, e.Cfg.Name, host)
	f, err := os.Open(reportPath)
	if err != nil {
		return
	}
	defer f.Close()
	c := exec.Command(e.Cfg.Settings.MailCommand, append([]string{"-s", subject}, e.Cfg.Settings.AdminEmail...)...)
	c.Stdin = f
	if out, err := c.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not send e-mail with %s: %v %s\n", e.Cfg.Settings.MailCommand, err, strings.TrimSpace(string(out)))
	}
}
