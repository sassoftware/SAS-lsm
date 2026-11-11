// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sassoftware/sas-lsm/internal/config"
	"github.com/sassoftware/sas-lsm/internal/runner"
)

const maxLogFilesPerHost = 50

// CleanPreviousLogs removes log output from an earlier run.
func (e *Engine) CleanPreviousLogs() {
	os.RemoveAll(filepath.Join(e.StatusDir, "logfiles"))
	os.Remove(filepath.Join(e.StatusDir, "logfiles.ANALYSIS"))
}

// collectLogs copies recent logs from failed tiers, writes an error summary
// and creates a support bundle. It only runs when log collection is enabled.
func (e *Engine) collectLogs() {
	if !(e.Opt.CollectLogs || e.Cfg.Settings.CollectLogs) || len(e.failed) == 0 {
		return
	}
	e.Rep.Printf("")
	e.Rep.Event("collecting logs from failed tier(s)")
	root := filepath.Join(e.StatusDir, "logfiles")
	done := map[*config.Tier]bool{}
	for _, t := range e.failed {
		if done[t] || t.LogDir == "" {
			continue
		}
		done[t] = true
		for _, h := range t.Hosts {
			e.copyHostLogs(t, h, root)
		}
	}
	e.analyzeLogs(root)
	if b, err := e.bundle(); err == nil {
		e.Rep.Printf("    support bundle: %s", b)
	} else {
		e.Rep.Printf("    warning: could not create support bundle: %v", err)
	}
}

// copyHostLogs copies the tail of recently changed logs through the existing
// SSH connection (no scp, no remote temp files).
func (e *Engine) copyHostLogs(t *config.Tier, h, root string) {
	s := e.Cfg.Settings
	find := fmt.Sprintf("find %s -type f -name '*.log' -mmin -%d 2>/dev/null | grep -v -e kahadb -e tmlog | head -n %d",
		runner.ShellQuote(t.LogDir), s.LogMinutes, maxLogFilesPerHost)
	res := e.R.Run(e.ctx, t.User, h, find, t.CommandTimeout)
	if res.Err != nil {
		e.Rep.Printf("    warning: cannot list logs on %s: %v", h, res.Err)
		return
	}
	n := 0
	for _, f := range strings.Split(res.Output, "\n") {
		f = strings.TrimSpace(f)
		if !strings.HasPrefix(f, "/") {
			continue
		}
		out := e.R.Run(e.ctx, t.User, h, fmt.Sprintf("tail -n %d %s", s.LogMaxLines, runner.ShellQuote(f)), t.CommandTimeout)
		if out.Err != nil {
			continue
		}
		dst := filepath.Join(root, h, sanitize(t.Name), f)
		if os.MkdirAll(filepath.Dir(dst), 0o750) == nil && os.WriteFile(dst, []byte(out.Output), 0o640) == nil {
			n++
		}
	}
	e.Rep.Printf("    %s: copied the last %d lines of %d log file(s) changed in the last %d minutes", h, s.LogMaxLines, n, s.LogMinutes)
}

var (
	errLine   = regexp.MustCompile(`ERROR`)
	errIgnore = regexp.MustCompile(`wrapper|INFO|Exception|Access|NOTE|duplicate key value violates unique constraint`)
	bracketed = regexp.MustCompile(`\[[^\]\[]*\]`)
)

// analyzeLogs ranks the most frequent ERROR messages (same filtering as 4.x).
func (e *Engine) analyzeLogs(root string) {
	counts := map[string]int{}
	perFile := map[string][]string{}
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		rel, _ := filepath.Rel(root, p)
		for ln := 1; sc.Scan(); ln++ {
			l := sc.Text()
			if !errLine.MatchString(l) || errIgnore.MatchString(l) {
				continue
			}
			msg := strings.TrimSpace(bracketed.ReplaceAllString(l[strings.Index(l, "ERROR"):], ""))
			if len(msg) > 100 {
				msg = msg[:100]
			}
			counts[msg]++
			if len(perFile[rel]) < 50 {
				perFile[rel] = append(perFile[rel], fmt.Sprintf("%d: %s", ln, l))
			}
		}
		return nil
	})
	type kv struct {
		k string
		v int
	}
	var top []kv
	for k, v := range counts {
		top = append(top, kv{k, v})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].v > top[j].v || (top[i].v == top[j].v && top[i].k < top[j].k) })
	if len(top) > 20 {
		top = top[:20]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "SAS_lsm LOG ANALYSIS  %s\n\nProblem: %s\n", time.Now().Format(time.RFC1123), e.problem)
	fmt.Fprintf(&b, "Tips when engaging SAS Technical Support: https://support.sas.com/kb/57/691.html\n\n")
	fmt.Fprintf(&b, "Most frequent ERROR messages:\n")
	for _, x := range top {
		fmt.Fprintf(&b, "%6d  %s\n", x.v, x.k)
	}
	files := make([]string, 0, len(perFile))
	for f := range perFile {
		files = append(files, f)
	}
	sort.Strings(files)
	fmt.Fprintf(&b, "\nERROR lines by file (first 50 per file):\n")
	for _, f := range files {
		fmt.Fprintf(&b, "%s\n", f)
		for _, l := range perFile[f] {
			fmt.Fprintf(&b, "    %s\n", l)
		}
	}
	os.WriteFile(filepath.Join(e.StatusDir, "logfiles.ANALYSIS"), []byte(b.String()), 0o640)
	if len(top) > 0 {
		e.Rep.Printf("")
		e.Rep.Printf("    Most frequent ERROR messages in collected logs:")
		for i, x := range top {
			if i == 10 {
				break
			}
			e.Rep.Printf("    %5d  %s", x.v, x.k)
		}
	}
	e.Rep.Printf("    analysis: %s", filepath.Join(e.StatusDir, "logfiles.ANALYSIS"))
}

// bundle creates a .tar.gz of the status directory (excluding archives and
// older bundles) for upload to SAS Technical Support.
func (e *Engine) bundle() (string, error) {
	name := filepath.Join(e.StatusDir, "support-bundle-"+time.Now().Format("20060102-150405")+".tar.gz")
	f, err := os.Create(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	err = filepath.Walk(e.StatusDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(e.StatusDir, p)
		base := filepath.Base(p)
		if rel == "." || strings.HasPrefix(base, "support-bundle-") {
			return nil
		}
		if fi.IsDir() && (base == "archive" || base == ".lock") {
			return filepath.SkipDir
		}
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return nil
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			if src, err := os.Open(p); err == nil {
				io.Copy(tw, src)
				src.Close()
			}
		}
		return nil
	})
	if cerr := tw.Close(); err == nil {
		err = cerr
	}
	if cerr := gz.Close(); err == nil {
		err = cerr
	}
	return name, err
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == ' ' {
			return '_'
		}
		return r
	}, s)
}
