// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"bufio"
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

// Key types ssh-setup can create or use.
const (
	KeyAuto    = "auto"
	KeyEd25519 = "ed25519"
	KeyECDSA   = "ecdsa"
	KeyRSA     = "rsa"
)

// SSHTarget is one account on one host.
type SSHTarget struct{ User, Host string }

func (t SSHTarget) String() string { return t.User + "@" + t.Host }

// SSHSetupOptions control ssh-setup.
type SSHSetupOptions struct {
	Targets    []SSHTarget
	SSHOptions []string
	KeyType    string // auto, ed25519, ecdsa, rsa
	AssumeYes  bool
	Verbose    bool   // also show ssh's own message for every host
	NextStep   string // hint printed when every host is ready
}

// TargetsFromConfig returns every unique user@host in a configuration.
func TargetsFromConfig(cfg *config.Config) []SSHTarget {
	seen := map[SSHTarget]bool{}
	var out []SSHTarget
	for _, t := range cfg.Tiers {
		for _, h := range t.Hosts {
			tg := SSHTarget{t.User, h}
			if !seen[tg] {
				seen[tg] = true
				out = append(out, tg)
			}
		}
	}
	return out
}

// TargetsFromList parses "host" or "user@host" entries. Entries without a
// user get defaultUser.
func TargetsFromList(list []string, defaultUser string) []SSHTarget {
	seen := map[SSHTarget]bool{}
	var out []SSHTarget
	for _, e := range list {
		tg := SSHTarget{defaultUser, e}
		if u, h, ok := strings.Cut(e, "@"); ok && u != "" && h != "" {
			tg = SSHTarget{u, h}
		}
		if !seen[tg] {
			seen[tg] = true
			out = append(out, tg)
		}
	}
	return out
}

// ---------------------------------------------------------------- FIPS

// fipsProbe lets tests replace the files that are read.
var fipsProbe = func(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// FIPSEnabled reports whether this machine runs in FIPS mode, and why.
// It checks the kernel flag (all FIPS Linux) and the RHEL 8+/Fedora system
// crypto policy, which can be set to FIPS without the kernel flag.
func FIPSEnabled() (bool, string) {
	if fipsProbe("/proc/sys/crypto/fips_enabled") == "1" {
		return true, "kernel FIPS mode (/proc/sys/crypto/fips_enabled = 1)"
	}
	if p := fipsProbe("/etc/crypto-policies/state/current"); strings.HasPrefix(strings.ToUpper(p), "FIPS") {
		return true, "system crypto policy " + p
	}
	return false, ""
}

// keyFile is the default OpenSSH file name for a key type.
func keyFile(t string) string { return "id_" + t }

// keyPreference returns key types in the order they should be used.
// Ed25519 is not a FIPS-approved algorithm, so it is excluded in FIPS mode.
func keyPreference(requested string, fips bool) ([]string, error) {
	switch requested {
	case "", KeyAuto:
		if fips {
			return []string{KeyECDSA, KeyRSA}, nil
		}
		return []string{KeyEd25519, KeyECDSA, KeyRSA}, nil
	case KeyEd25519:
		if fips {
			return nil, fmt.Errorf("--key-type ed25519 cannot be used: this system is in FIPS mode, which does not allow Ed25519 keys (use ecdsa or rsa)")
		}
		return []string{KeyEd25519}, nil
	case KeyECDSA, KeyRSA:
		return []string{requested}, nil
	}
	return nil, fmt.Errorf("unknown --key-type %q (valid: auto, ecdsa, rsa, ed25519)", requested)
}

// keygenArgs returns ssh-keygen arguments. ECDSA P-384 and RSA 4096 are
// accepted by OpenSSH in FIPS mode on RHEL, SLES and Ubuntu Pro FIPS.
func keygenArgs(t, path, comment string) []string {
	args := []string{"-q", "-N", "", "-f", path, "-C", comment, "-t", t}
	switch t {
	case KeyECDSA:
		args = append(args, "-b", "384")
	case KeyRSA:
		args = append(args, "-b", "4096")
	}
	return args
}

// ---------------------------------------------------------------- host check

// hostState is the result of testing one host with the key.
type hostState int

const (
	stReady       hostState = iota // key already works
	stNeedsKey                     // reachable, key not accepted yet: install it
	stUnreachable                  // DNS, network or sshd problem: a password won't help
	stHostKey                      // host key changed: must be resolved by the admin first
	stOther                        // anything else
)

// label is the short tag shown in the host list.
func (s hostState) label() string {
	switch s {
	case stReady:
		return "[ready]"
	case stNeedsKey:
		return "[needs key]"
	case stHostKey:
		return "[host key]"
	}
	return "[problem]"
}

// classifySSH turns the result of "ssh host true" into a state and a plain
// description. The expected first-run case (key not installed yet) is
// described as expected rather than as an error.
func classifySSH(ok bool, out string) (hostState, string) {
	if ok {
		return stReady, "key already installed"
	}
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(out, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("Permission denied", "Too many authentication failures", "Authentication failed", "No more authentication methods"):
		return stNeedsKey, "key not installed yet (expected)"
	case has("Host key verification failed", "REMOTE HOST IDENTIFICATION HAS CHANGED"):
		return stHostKey, "host key has changed since this host was first recorded"
	case has("Could not resolve hostname", "Name or service not known", "nodename nor servname"):
		return stUnreachable, "host name could not be resolved"
	case has("Connection refused"):
		return stUnreachable, "SSH connection refused (is sshd running on this host?)"
	case has("No route to host", "Network is unreachable", "Connection timed out", "timed out"):
		return stUnreachable, "host could not be reached"
	}
	return stOther, sshReason(out)
}

// fixHint is the advice printed for hosts that a password cannot fix.
func fixHint(s hostState, tg SSHTarget) string {
	switch s {
	case stHostKey:
		return fmt.Sprintf("confirm with your system administrator that the change is expected, then run: ssh-keygen -R %s", tg.Host)
	case stUnreachable:
		return "check the host name, the network, and that sshd is running on the host"
	}
	return fmt.Sprintf("run 'ssh -v %s true' to see the full SSH conversation", tg)
}

// ---------------------------------------------------------------- main flow

// SSHSetup makes sure this account has a usable SSH key and that the key is
// installed for every target. Each host's password is requested at most
// once, by ssh-copy-id.
func SSHSetup(ctx context.Context, w io.Writer, opt SSHSetupOptions) error {
	fips, why := FIPSEnabled()
	if fips {
		fmt.Fprintf(w, "FIPS mode detected (%s): Ed25519 keys will not be used.\n", why)
	}
	prefs, err := keyPreference(opt.KeyType, fips)
	if err != nil {
		return err
	}

	base := runner.New(opt.SSHOptions, false)
	var targets []SSHTarget
	for _, tg := range opt.Targets {
		if !base.IsLocal(tg.User, tg.Host) {
			targets = append(targets, tg)
		}
	}
	if len(targets) == 0 {
		fmt.Fprintln(w, "Every host is this machine and runs as this user; no SSH setup is needed.")
		return nil
	}

	key, err := ensureKey(w, prefs, fips, base)
	if err != nil {
		return err
	}

	// Test with exactly this key, so an unusable default key (for example
	// Ed25519 in FIPS mode) can't hide the real result.
	r := runner.New(append(append([]string{}, opt.SSHOptions...),
		"-o", "IdentityFile="+key, "-o", "IdentitiesOnly=yes"), false)

	width := 0
	for _, tg := range targets {
		if n := len(tg.String()); n > width {
			width = n
		}
	}
	line := func(s hostState, tg SSHTarget, msg string) {
		fmt.Fprintf(w, "  %-12s %-*s   %s\n", s.label(), width, tg, msg)
	}

	type checked struct {
		tg    SSHTarget
		state hostState
		msg   string
	}
	var ready, needKey, problems []checked
	fmt.Fprintf(w, "\nChecking which hosts already accept this key...\n\n")
	for _, tg := range targets {
		res := r.Run(ctx, tg.User, tg.Host, "true", 20*time.Second)
		st, msg := classifySSH(res.Err == nil && res.ExitCode == 0, res.Output)
		line(st, tg, msg)
		if opt.Verbose && st != stReady {
			if raw := strings.TrimSpace(res.Output); raw != "" {
				fmt.Fprintf(w, "%s\n", indent(raw, strings.Repeat(" ", 17)+"ssh: "))
			}
		}
		c := checked{tg, st, msg}
		switch st {
		case stReady:
			ready = append(ready, c)
		case stNeedsKey:
			needKey = append(needKey, c)
		default:
			problems = append(problems, c)
		}
	}

	if len(problems) > 0 {
		fmt.Fprintf(w, "\n%d host(s) have a problem that a password will not fix. They will be skipped:\n", len(problems))
		for _, p := range problems {
			fmt.Fprintf(w, "  %s: %s\n", p.tg, fixHint(p.state, p.tg))
		}
	}

	if len(needKey) == 0 {
		fmt.Fprintln(w)
		if len(problems) > 0 {
			return fmt.Errorf("%d host(s) could not be set up; fix the problems above and run ssh-setup again", len(problems))
		}
		fmt.Fprintln(w, "All hosts are ready. No passwords are needed.")
		printNext(w, opt.NextStep)
		return nil
	}

	if _, err := exec.LookPath("ssh-copy-id"); err != nil {
		fmt.Fprintf(w, "\nssh-copy-id is not installed on this machine, so the key must be added by hand.\n")
		fmt.Fprintf(w, "On each host marked [needs key], append the contents of %s.pub to\n", key)
		fmt.Fprintf(w, "~/.ssh/authorized_keys for that account (see the setup guide, 'Set Up Passwordless SSH').\n")
		return fmt.Errorf("ssh-copy-id not found")
	}

	plural := "host needs"
	if len(needKey) > 1 {
		plural = "hosts need"
	}
	fmt.Fprintf(w, "\n%d %s the key. For each one, ssh-copy-id will ask for that account's\n", len(needKey), plural)
	fmt.Fprintf(w, "password once, then install the key so SAS_lsm can log in without a password.\n")
	if !opt.AssumeYes {
		fmt.Fprint(w, "Continue? [y/N] ")
		ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(ans)); a != "y" && a != "yes" {
			return fmt.Errorf("cancelled; no keys were installed")
		}
	}

	var installed []SSHTarget
	var failed []checked
	for i, c := range needKey {
		tg := c.tg
		fmt.Fprintf(w, "\n==> [%d/%d] %s: enter the password for %s when prompted\n", i+1, len(needKey), tg, tg.User)
		// -f: install even if ssh-copy-id thinks another default key already works.
		// IdentitiesOnly keeps ssh-copy-id's own login from tripping over unusable keys.
		err := runner.Interactive("ssh-copy-id", "-f", "-i", key+".pub",
			"-o", "StrictHostKeyChecking=accept-new", "-o", "IdentitiesOnly=yes", tg.String())
		if err != nil {
			fmt.Fprintf(w, "    the key was not installed (wrong password, or the account cannot log in)\n")
			failed = append(failed, checked{tg, stNeedsKey, "key was not installed (wrong password, or the account cannot log in)"})
			continue
		}
		res := r.Run(ctx, tg.User, tg.Host, "true", 20*time.Second)
		if res.Err == nil && res.ExitCode == 0 {
			fmt.Fprintf(w, "    verified: %s now accepts the key\n", tg)
			installed = append(installed, tg)
			continue
		}
		msg := "key was copied, but login without a password still fails: " + sshReason(res.Output)
		fmt.Fprintf(w, "    %s\n", msg)
		failed = append(failed, checked{tg, stOther, msg})
	}

	fmt.Fprintf(w, "\nSummary\n")
	fmt.Fprintf(w, "  ready:    %d host(s) (%d already set up, %d installed now)\n", len(ready)+len(installed), len(ready), len(installed))
	if len(failed) > 0 {
		fmt.Fprintf(w, "  failed:   %d host(s)\n", len(failed))
		for _, f := range failed {
			fmt.Fprintf(w, "              %s - %s\n", f.tg, f.msg)
		}
	}
	if len(problems) > 0 {
		fmt.Fprintf(w, "  skipped:  %d host(s) (see the problems listed above)\n", len(problems))
	}
	if len(failed)+len(problems) > 0 {
		fmt.Fprintln(w)
		return fmt.Errorf("%d host(s) are not ready; fix them and run ssh-setup again (hosts that are ready are skipped)", len(failed)+len(problems))
	}
	fmt.Fprintln(w, "\nAll hosts are ready.")
	printNext(w, opt.NextStep)
	return nil
}

// ensureKey returns the path of a usable private key, creating one if needed.
func ensureKey(w io.Writer, prefs []string, fips bool, base *runner.Runner) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sshDir := filepath.Join(home, ".ssh")
	for _, t := range prefs {
		p := filepath.Join(sshDir, keyFile(t))
		if exists(p) && exists(p+".pub") {
			fmt.Fprintf(w, "Using existing SSH key %s\n", p+".pub")
			warnEd25519(w, fips, sshDir)
			return p, nil
		}
	}
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return "", err
	}
	comment := "sas_lsm " + base.LocalUser + "@" + base.LocalShort
	var lastErr error
	for _, t := range prefs {
		p := filepath.Join(sshDir, keyFile(t))
		if exists(p) {
			// private key without .pub: do not overwrite someone's key
			lastErr = fmt.Errorf("%s exists but %s.pub does not; recreate it with 'ssh-keygen -y -f %s > %s.pub'", p, p, p, p)
			continue
		}
		fmt.Fprintf(w, "No usable SSH key found for %s. Creating %s key %s (no passphrase, so scheduled runs work).\n", base.LocalUser, strings.ToUpper(t), p)
		if lastErr = runner.Interactive("ssh-keygen", keygenArgs(t, p, comment)...); lastErr == nil {
			warnEd25519(w, fips, sshDir)
			return p, nil
		}
	}
	return "", fmt.Errorf("could not create an SSH key: %v", lastErr)
}

func warnEd25519(w io.Writer, fips bool, sshDir string) {
	if p := filepath.Join(sshDir, keyFile(KeyEd25519)); fips && exists(p) {
		fmt.Fprintf(w, "Note: %s cannot be used in FIPS mode. ssh may print a warning about it; the warning is harmless.\n", p)
	}
}

func printNext(w io.Writer, next string) {
	if next != "" {
		fmt.Fprintln(w, "Next: "+next)
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + strings.TrimSpace(l)
	}
	return strings.Join(lines, "\n")
}

// sshReason picks the most useful line from ssh's error output. Warnings
// such as "Ed25519 keys are not allowed in FIPS mode" are skipped so the real
// cause (for example "Permission denied") is shown.
func sshReason(out string) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	for _, key := range []string{"Permission denied", "Host key verification failed", "Could not resolve",
		"Connection refused", "Connection timed out", "No route to host", "Too many authentication failures"} {
		for _, l := range lines {
			if strings.Contains(l, key) {
				return l
			}
		}
	}
	for _, l := range lines {
		if !strings.Contains(l, "not allowed in FIPS mode") && !strings.HasPrefix(l, "Warning:") {
			return l
		}
	}
	if len(lines) > 0 {
		return lines[0]
	}
	return "login failed"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
