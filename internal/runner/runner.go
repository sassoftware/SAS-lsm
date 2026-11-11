// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package runner executes commands on tier hosts, either locally or through
// the system OpenSSH client. Using the system client (rather than an embedded
// SSH library) means ~/.ssh/config, known_hosts, ssh-agent, Kerberos/GSSAPI
// and connection multiplexing all work exactly as they do for the admin.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// Result of one command.
type Result struct {
	Host     string
	Command  string
	Output   string
	ExitCode int
	Err      error // transport/timeout errors (not non-zero exit codes)
	Duration time.Duration
}

// Runner runs commands.
type Runner struct {
	SSHOptions []string
	Multiplex  bool
	LocalShort string // short hostname of this machine
	LocalUser  string
	// ForceSSH disables the "run locally when host is this machine" shortcut.
	ForceSSH bool
}

// New builds a Runner for this machine.
func New(sshOptions []string, multiplex bool) *Runner {
	r := &Runner{SSHOptions: sshOptions, Multiplex: multiplex}
	if h, err := os.Hostname(); err == nil {
		r.LocalShort = Short(h)
	}
	if u, err := user.Current(); err == nil {
		r.LocalUser = u.Username
	}
	return r
}

// Short returns the hostname up to the first dot, lower-cased.
func Short(h string) string {
	h = strings.ToLower(h)
	if i := strings.IndexByte(h, '.'); i > 0 {
		return h[:i]
	}
	return h
}

// IsLocal reports whether user@host can be run without SSH.
func (r *Runner) IsLocal(usr, host string) bool {
	if r.ForceSSH || usr != r.LocalUser {
		return false
	}
	s := Short(host)
	return s == r.LocalShort || s == "localhost" || host == "127.0.0.1"
}

// SSHArgs returns the ssh arguments (without the remote command) for user@host.
func (r *Runner) SSHArgs(usr, host string) []string {
	args := append([]string{}, r.SSHOptions...)
	if r.Multiplex {
		home, _ := os.UserHomeDir()
		args = append(args,
			"-o", "ControlMaster=auto",
			"-o", "ControlPath="+filepath.Join(home, ".ssh", "sas_lsm-%C"),
			"-o", "ControlPersist=60")
	}
	return append(args, "-l", usr, host)
}

// Run executes cmd for user on host and waits up to timeout.
func (r *Runner) Run(ctx context.Context, usr, host, cmd string, timeout time.Duration) Result {
	start := time.Now()
	res := Result{Host: host, Command: cmd}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	local := r.IsLocal(usr, host)
	var c *exec.Cmd
	if local {
		c = exec.CommandContext(ctx, "/bin/sh", "-c", cmd)
	} else {
		c = exec.CommandContext(ctx, "ssh", append(r.SSHArgs(usr, host), cmd)...)
	}
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	// Backgrounded children (e.g. "nohup ... &") can hold the output pipe open.
	// WaitDelay stops us from hanging on them after the command itself exits.
	c.WaitDelay = 5 * time.Second
	err := c.Run()
	res.Output = buf.String()
	res.Duration = time.Since(start)
	var ee *exec.ExitError
	switch {
	case err == nil:
	case ctx.Err() == context.DeadlineExceeded:
		res.Err = fmt.Errorf("timed out after %s", timeout)
		res.ExitCode = -1
	case errors.Is(err, exec.ErrWaitDelay):
		// command finished; a background child kept the pipe open - not an error
	case errors.As(err, &ee):
		res.ExitCode = ee.ExitCode()
		// ssh uses 255 for its own failures (connection, auth, host key).
		if !local && res.ExitCode == 255 {
			res.Err = fmt.Errorf("ssh to %s@%s failed (exit 255): %s", usr, host, firstLine(res.Output))
		}
	default:
		res.Err = err
		res.ExitCode = -1
	}
	return res
}

// Interactive runs a command attached to the user's terminal (for ssh-copy-id).
func Interactive(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// ShellQuote quotes s for a POSIX shell.
func ShellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./=:@,+%") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "no output"
	}
	return s
}
