// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Reporter writes progress to the console and to the report file.
type Reporter struct {
	mu      sync.Mutex
	console io.Writer
	file    *os.File
	Quiet   bool // suppress console output (cron-friendly); report file still written
}

// NewReporter creates a reporter. path may be "" (console only).
func NewReporter(path string) (*Reporter, error) {
	r := &Reporter{console: os.Stdout}
	if path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
		if err != nil {
			return nil, err
		}
		r.file = f
	}
	return r, nil
}

// Close closes the report file.
func (r *Reporter) Close() {
	if r.file != nil {
		r.file.Close()
	}
}

// Printf writes a line to the console and the report.
func (r *Reporter) Printf(format string, a ...any) { r.write(true, fmt.Sprintf(format, a...)) }

// Filef writes a line to the report file only (e.g. raw command output).
func (r *Reporter) Filef(format string, a ...any) { r.write(false, fmt.Sprintf(format, a...)) }

// Event writes a timestamped event line.
func (r *Reporter) Event(format string, a ...any) {
	r.Printf("%s  %s", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, a...))
}

func (r *Reporter) write(console bool, s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	if console && !r.Quiet {
		io.WriteString(r.console, s)
	}
	if r.file != nil {
		io.WriteString(r.file, s)
	}
}

// Indent prefixes every non-empty line of text.
func Indent(text, prefix string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			b.WriteString(prefix + l + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
