// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package tools implements the setup helpers: doctor, ssh-setup, convert and discover.
package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sassoftware/sas-lsm/internal/config"
	"github.com/sassoftware/sas-lsm/internal/runner"
)

type check struct {
	desc string
	cmd  string // must exit 0 to pass
	hint string
}

// Doctor checks that every tier host is reachable and prepared. It returns
// the number of failed checks.
func Doctor(ctx context.Context, w io.Writer, cfg *config.Config) int {
	r := runner.New(cfg.Settings.SSHOptions, cfg.Settings.SSHMultiplex)
	failed := 0
	pass := func(ok bool, what, detail, hint string) {
		mark := "ok  "
		if !ok {
			mark = "FAIL"
			failed++
		}
		fmt.Fprintf(w, "  [%s] %s", mark, what)
		if detail != "" {
			fmt.Fprintf(w, " - %s", detail)
		}
		fmt.Fprintln(w)
		if !ok && hint != "" {
			fmt.Fprintf(w, "         fix: %s\n", hint)
		}
	}

	fmt.Fprintf(w, "Controller (%s, user %s)\n", r.LocalShort, r.LocalUser)
	pass(r.LocalUser != "root", "not running as root", "", "run SAS_lsm as the SAS installer account")
	dir := filepath.Join(cfg.Settings.StatusRoot, cfg.Name)
	err := os.MkdirAll(dir, 0o750)
	if err == nil {
		f := filepath.Join(dir, ".doctor")
		err = os.WriteFile(f, []byte("ok"), 0o640)
		os.Remove(f)
	}
	pass(err == nil, "status directory writable", dir, "set settings.status_root to a directory this user can write")
	needSSH := false
	for _, t := range cfg.Tiers {
		for _, h := range t.Hosts {
			needSSH = needSSH || !r.IsLocal(t.User, h)
		}
	}
	if needSSH {
		_, err = exec.LookPath("ssh")
		pass(err == nil, "OpenSSH client available", "", "install the OpenSSH client package")
	}
	if len(cfg.Settings.AdminEmail) > 0 {
		_, err = exec.LookPath(cfg.Settings.MailCommand)
		pass(err == nil, "mail command available", cfg.Settings.MailCommand, "install mailx or set settings.mail_command")
	}
	fmt.Fprintln(w)

	type target struct{ user, host string }
	checks := map[target][]check{}
	var order []target
	add := func(u, h string, c ...check) {
		k := target{u, h}
		if _, ok := checks[k]; !ok {
			order = append(order, k)
		}
		checks[k] = append(checks[k], c...)
	}
	for _, t := range cfg.Tiers {
		for _, h := range t.Hosts {
			add(t.User, h)
			switch t.Type {
			case config.TypeSASServers:
				add(t.User, h, check{"sas.servers is executable (" + t.Label() + ")", "test -x " + runner.ShellQuote(t.ConfigDir+"/sas.servers"),
					"check this tier's hosts and configdir (" + t.ConfigDir + ")"})
			case config.TypeMetadataCluster:
				p := t.ConfigDir + "/SASMeta/MetadataServer/MetadataServer.sh"
				add(t.User, h, check{"MetadataServer.sh is executable (" + t.Label() + ")", "test -x " + runner.ShellQuote(p), "check configdir; expected " + p})
			case config.TypeLSFGrid:
				add(t.User, h, check{"profile.lsf readable (" + t.Label() + ")", "test -r " + runner.ShellQuote(t.LSF.EnvDir+"/profile.lsf"), "check lsf.envdir"})
				if t.LSF.Sudo {
					add(t.User, h, check{"passwordless sudo (" + t.Label() + ")", "sudo -n true",
						"LSF daemons need root; allow this account to run the LSF commands with sudo NOPASSWD (see the setup guide, lsf-grid)"})
				}
			}
			if t.UserExitDir != "" && h == t.Hosts[0] {
				add(t.User, h, check{"userexit_dir exists (" + t.Label() + ")", "test -d " + runner.ShellQuote(t.UserExitDir), "create the directory or remove userexit_dir"})
			}
			for ev, list := range t.UserExits {
				for _, ue := range list {
					if ue.On != "all" && h != t.Hosts[0] {
						continue
					}
					if f := strings.Fields(ue.Run); len(f) > 0 && strings.HasPrefix(f[0], "/") {
						add(t.User, h, check{fmt.Sprintf("userexit %s is executable (%s)", f[0], ev), "test -x " + runner.ShellQuote(f[0]), "chmod +x the script, or fix the path"})
					}
				}
			}
		}
	}

	for _, tg := range order {
		local := r.IsLocal(tg.user, tg.host)
		label := tg.user + "@" + tg.host
		if local {
			label += " (this machine - no SSH needed)"
		}
		fmt.Fprintln(w, label)
		res := r.Run(ctx, tg.user, tg.host, "id -un", 30*time.Second)
		if res.Err != nil {
			pass(false, "connect without a password", sshReason(res.Output),
				"run 'sas_lsm ssh-setup -c "+cfg.Path+"' to install an SSH key for "+tg.user+"@"+tg.host)
			fmt.Fprintln(w)
			continue
		}
		if !local {
			pass(true, "connect without a password", "", "")
		}
		got := strings.TrimSpace(res.Output)
		pass(got == tg.user, "logged in as the expected user", got, "check ~/.ssh/config for a User override")
		for _, c := range checks[tg] {
			res := r.Run(ctx, tg.user, tg.host, c.cmd, 30*time.Second)
			pass(res.Err == nil && res.ExitCode == 0, c.desc, "", c.hint)
		}
		fmt.Fprintln(w)
	}
	if failed == 0 {
		fmt.Fprintln(w, "All checks passed.")
	} else {
		fmt.Fprintf(w, "%d check(s) failed.\n", failed)
	}
	return failed
}
