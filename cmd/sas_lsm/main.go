// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// sas_lsm manages ordered startup, shutdown and status of multi-tier
// SAS 9.4 deployments on UNIX/Linux.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sassoftware/sas-lsm/internal/config"
	"github.com/sassoftware/sas-lsm/internal/engine"
	"github.com/sassoftware/sas-lsm/internal/runner"
	"github.com/sassoftware/sas-lsm/internal/tools"
)

// Version is set at build time with -ldflags "-X main.Version=...".
var Version = "5.0.0-dev"

const usage = `SAS_lsm %s - ordered start/stop/status for SAS 9.4 UNIX/Linux deployments

Usage:
  sas_lsm <command> [options]

Operate:
  start      [--from TIER]   start tiers from TIER (default: first) to the last tier
  stop       [--to TIER]     stop tiers from the last tier down to TIER (default: first)
  status                     show the state of every tier

Set up:
  ssh-setup  --hosts h1,u@h2 [--user U]      install an SSH key on a list of hosts (no config needed)
  ssh-setup  -c FILE                         ...or on every host in a configuration file
             options: --key-type auto|ecdsa|rsa|ed25519 (auto avoids ed25519 in FIPS mode)
                      --yes (do not ask before installing)  --verbose (show ssh's own messages)
  discover   --hosts h1,h2 --configdir DIR [--user U]   draft a config by inspecting hosts
  convert    OLD.cfg         convert a SAS_lsm 4.x config to YAML (prints to stdout)
  validate                   check the configuration file
  doctor                     check SSH, users, paths and permissions on every host
  userexits  [--tier TIER]   list the userexit scripts that would run for each event
  version    [--check]       print the version (--check asks GitHub for the latest)

Common options:
  -c, --config FILE   configuration file (default: $SAS_LSM_CONFIG or ./sas_lsm.yaml)
  --dry-run           (start/stop) print the plan without running anything
  --collect-logs      collect logs and a support bundle from failed tiers
  --no-userexits      do not run userexit scripts
  -q, --quiet         no console output; report file only (for cron)

TIER is a tier number (1, 2, ...) or a tier name.
SAS_lsm 4.x options are accepted: -a N, -o N, -s, -c CFG, -e, -x, -v

Exit codes: 0 ok, 1 failed, 2 start dependency check failed, 3 config/usage error,
            4 state unknown, 5 another run is active
`

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Printf(usage, Version)
		return 0
	}
	args = translateLegacy(args)
	cmd, rest := args[0], args[1:]

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfgPath := fs.String("config", defaultConfig(), "")
	fs.StringVar(cfgPath, "c", defaultConfig(), "")
	from := fs.String("from", "", "")
	to := fs.String("to", "", "")
	tier := fs.String("tier", "", "")
	dry := fs.Bool("dry-run", false, "")
	collect := fs.Bool("collect-logs", false, "")
	noUE := fs.Bool("no-userexits", false, "")
	quiet := fs.Bool("quiet", false, "")
	fs.BoolVar(quiet, "q", false, "")
	hosts := fs.String("hosts", "", "")
	cdir := fs.String("configdir", "", "")
	usr := fs.String("user", "", "")
	yes := fs.Bool("yes", false, "")
	verbose := fs.Bool("verbose", false, "")
	fs.BoolVar(verbose, "v", false, "")
	check := fs.Bool("check", false, "")
	keyType := fs.String("key-type", tools.KeyAuto, "")

	// allow positional arguments (convert OLD.cfg) anywhere among the flags
	var positional []string
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\nRun 'sas_lsm help' for usage.\n", err)
			return engine.ExitConfig
		}
		rest = fs.Args()
		if len(rest) > 0 {
			positional = append(positional, rest[0])
			rest = rest[1:]
		}
	}

	switch cmd {
	case "version":
		fmt.Printf("SAS_lsm %s\n", Version)
		if *check {
			checkLatest()
		}
		return engine.ExitOK
	case "convert":
		if len(positional) != 1 {
			fmt.Fprintln(os.Stderr, "usage: sas_lsm convert OLD.cfg > new.yaml")
			return engine.ExitConfig
		}
		out, notes, err := tools.Convert(positional[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return engine.ExitConfig
		}
		fmt.Print(out)
		if len(notes) > 0 {
			fmt.Fprintln(os.Stderr, "Review notes:")
			for _, n := range notes {
				fmt.Fprintln(os.Stderr, "  -", n)
			}
		}
		return engine.ExitOK
	case "discover":
		if *hosts == "" || *cdir == "" {
			fmt.Fprintln(os.Stderr, "usage: sas_lsm discover --hosts meta1,compute1,mid1 --configdir /opt/sas/config/Lev1 [--user sas] > sas_lsm.yaml")
			return engine.ExitConfig
		}
		u := *usr
		if u == "" {
			u = runner.New(nil, false).LocalUser
		}
		opts := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=accept-new"}
		if err := tools.Discover(ctx, os.Stdout, os.Stderr, splitList(*hosts), u, strings.TrimRight(*cdir, "/"), opts); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return engine.ExitFailed
		}
		return engine.ExitOK
	}

	if os.Geteuid() == 0 {
		fmt.Fprintln(os.Stderr, "error: do not run sas_lsm as root; run it as the SAS installer account")
		return engine.ExitConfig
	}

	// ssh-setup with --hosts works before any configuration file exists.
	if cmd == "ssh-setup" && *hosts != "" {
		u := *usr
		if u == "" {
			u = runner.New(nil, false).LocalUser
		}
		err := tools.SSHSetup(ctx, os.Stdout, tools.SSHSetupOptions{
			Targets:    tools.TargetsFromList(splitList(*hosts), u),
			SSHOptions: []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=accept-new"},
			KeyType:    *keyType,
			AssumeYes:  *yes,
			Verbose:    *verbose,
			NextStep:   "sas_lsm discover --hosts " + *hosts + " --configdir <SAS-config-dir> --user " + u + " > sas_lsm.yaml",
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return engine.ExitFailed
		}
		return engine.ExitOK
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "Specify a configuration with -c FILE, or create one with 'sas_lsm discover' (see the setup guide).")
			if cmd == "ssh-setup" {
				fmt.Fprintln(os.Stderr, "To set up SSH before a configuration exists, use: sas_lsm ssh-setup --hosts host1,host2 --user <SAS installer account>")
			}
		}
		return engine.ExitConfig
	}

	switch cmd {
	case "validate":
		fmt.Printf("%s is valid: %d tier(s)\n", cfg.Path, len(cfg.Tiers))
		for _, t := range cfg.Tiers {
			fmt.Printf("  %-3d %-24s %-17s %s@%s\n", t.Index, t.Name, t.Type, t.User, strings.Join(t.Hosts, ","))
		}
		return engine.ExitOK
	case "doctor":
		if tools.Doctor(ctx, os.Stdout, cfg) > 0 {
			return engine.ExitFailed
		}
		return engine.ExitOK
	case "ssh-setup":
		if err := tools.SSHSetup(ctx, os.Stdout, tools.SSHSetupOptions{
			Targets:    tools.TargetsFromConfig(cfg),
			SSHOptions: cfg.Settings.SSHOptions,
			KeyType:    *keyType,
			AssumeYes:  *yes,
			Verbose:    *verbose,
			NextStep:   "sas_lsm doctor -c " + cfg.Path,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return engine.ExitFailed
		}
		return engine.ExitOK
	case "userexits":
		return listUserExits(ctx, cfg, *tier)
	case "start", "stop", "status":
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\nRun 'sas_lsm help' for usage.\n", cmd)
		return engine.ExitConfig
	}

	edgeRef := "1"
	if cmd == "start" && *from != "" {
		edgeRef = *from
	}
	if cmd == "stop" && *to != "" {
		edgeRef = *to
	}
	edge, err := cfg.Lookup(edgeRef)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return engine.ExitConfig
	}
	if *dry {
		if cmd == "status" {
			fmt.Fprintln(os.Stderr, "--dry-run applies to start and stop")
			return engine.ExitConfig
		}
		engine.Plan(os.Stdout, cfg, cmd, edge)
		return engine.ExitOK
	}

	e, err := engine.New(ctx, cfg, engine.Options{NoUserExits: *noUE, CollectLogs: *collect})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return engine.ExitConfig
	}
	unlock, err := e.Lock()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return engine.ExitLocked
	}
	defer unlock()

	report := filepath.Join(e.StatusDir, "report."+cmd)
	rep, err := engine.NewReporter(report)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return engine.ExitConfig
	}
	rep.Quiet = *quiet
	e.Rep = rep
	host, _ := os.Hostname()
	rep.Filef("COMMAND: sas_lsm %s", strings.Join(args, " "))
	rep.Printf("SAS_lsm %s  config %s  host %s  user %s", Version, cfg.Path, host, e.R.LocalUser)
	if *noUE {
		rep.Printf("userexits are DISABLED for this run (--no-userexits)")
	}
	rep.Printf("")
	e.CleanPreviousLogs()

	var rc int
	switch cmd {
	case "start":
		rc = e.Start(edge)
	case "stop":
		rc = e.Stop(edge)
	case "status":
		rc = e.Status()
	}
	if ctx.Err() != nil {
		rep.Printf("<<INTERRUPTED>> run was cancelled; tiers may be in a partial state")
		rc = engine.ExitFailed
	}
	rep.Printf("")
	rep.Printf("report: %s", report)
	rep.Close()
	e.Finish(cmd, report, rc)
	return rc
}

// translateLegacy maps SAS_lsm 4.x options (-a N -o N -s -c CFG -e -x -v)
// onto the new commands so existing cron entries keep working.
func translateLegacy(args []string) []string {
	if !strings.HasPrefix(args[0], "-") {
		return args
	}
	var cmd string
	var out []string
	for i := 0; i < len(args); i++ {
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch a := args[i]; a {
		case "-a", "--start":
			cmd = "start"
			out = append(out, "--from", next())
		case "-o", "--stop":
			cmd = "stop"
			out = append(out, "--to", next())
		case "-s", "--status":
			cmd = "status"
		case "-v", "--version":
			cmd = "version"
		case "-e", "--log-collect":
			out = append(out, "--collect-logs")
		case "-x", "--no-userexits":
			out = append(out, "--no-userexits")
		default:
			out = append(out, a)
		}
	}
	if cmd == "" {
		return args
	}
	return append([]string{cmd}, out...)
}

func listUserExits(ctx context.Context, cfg *config.Config, ref string) int {
	e, err := engine.New(ctx, cfg, engine.Options{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return engine.ExitConfig
	}
	tiers := cfg.Tiers
	if ref != "" {
		t, err := cfg.Lookup(ref)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return engine.ExitConfig
		}
		tiers = []*config.Tier{t}
	}
	for _, t := range tiers {
		fmt.Printf("tier %s\n", t.Label())
		found := false
		for _, ev := range config.Events {
			list, err := e.ResolveUserExits(t, ev)
			if err != nil {
				fmt.Printf("  %-20s error: %v\n", ev, err)
			}
			for i, ue := range list {
				label := ev
				if i > 0 {
					label = ""
				}
				extra := ""
				if ue.Required {
					extra += " (required)"
				}
				if ue.On == "all" {
					extra += " [all hosts]"
				}
				fmt.Printf("  %-20s %s%s   <- %s\n", label, ue.Run, extra, ue.Source)
				found = true
			}
		}
		if !found {
			fmt.Println("  (none)")
		}
		fmt.Println()
	}
	return engine.ExitOK
}

func checkLatest() {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get("https://api.github.com/repos/sassoftware/sas-lsm/releases/latest")
	if err != nil {
		fmt.Println("could not check for updates:", err)
		return
	}
	defer resp.Body.Close()
	var r struct {
		Tag string `json:"tag_name"`
	}
	if json.NewDecoder(resp.Body).Decode(&r) != nil || r.Tag == "" {
		fmt.Println("could not read the latest release information")
		return
	}
	fmt.Printf("latest release on GitHub: %s\n", r.Tag)
}

func defaultConfig() string {
	if c := os.Getenv("SAS_LSM_CONFIG"); c != "" {
		return c
	}
	return "sas_lsm.yaml"
}

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}
