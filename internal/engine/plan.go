// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"io"
	"strings"

	"github.com/sassoftware/sas-lsm/internal/config"
)

// Plan prints what start or stop would do, without running anything.
func Plan(w io.Writer, cfg *config.Config, action string, edge *config.Tier) {
	tiers := cfg.Tiers
	var order []*config.Tier
	if action == "start" {
		order = tiers[edge.Index-1:]
		fmt.Fprintf(w, "DRY RUN: start from tier %s\n\n", edge.Label())
		for _, t := range tiers {
			switch {
			case t.Index < edge.Index:
				fmt.Fprintf(w, "  check  tier %-24s must be UP\n", t.Label())
			case t.Index > edge.Index:
				fmt.Fprintf(w, "  check  tier %-24s must be DOWN\n", t.Label())
			}
		}
		fmt.Fprintln(w)
	} else {
		for i := len(tiers) - 1; i >= edge.Index-1; i-- {
			order = append(order, tiers[i])
		}
		fmt.Fprintf(w, "DRY RUN: stop down to tier %s\n\n", edge.Label())
	}
	target := "UP"
	if action == "stop" {
		target = "DOWN"
	}
	for _, t := range order {
		mode := "one host at a time"
		if t.Parallel {
			mode = "hosts in parallel"
		}
		fmt.Fprintf(w, "tier %s  [type %s, user %s, %s]\n", t.Label(), t.Type, t.User, mode)
		for _, ue := range t.UserExits["pre_"+action] {
			req := ""
			if ue.Required {
				req = "  (required)"
			}
			fmt.Fprintf(w, "  userexit pre_%-8s %s%s\n", action, ue.Run, req)
		}
		if t.UserExitDir != "" {
			fmt.Fprintf(w, "  userexit directory    %s (files are resolved at run time)\n", t.UserExitDir)
		}
		switch t.Type {
		case config.TypeLSFGrid:
			l := t.LSF
			fmt.Fprintf(w, "  %s LSF daemons: master %s, nodes [%s]", action, l.Master, strings.Join(l.Nodes, ", "))
			if l.PMHost != "" {
				fmt.Fprintf(w, ", Process Manager on %s", l.PMHost)
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "  wait for %s: LSF daemon process checks (up to %d checks, %s apart)\n\n", target, t.Retries, t.RetryInterval)
			continue
		case config.TypeMetadataCluster:
			if action == "start" {
				fmt.Fprintf(w, "  on every node %s: %s\n", strings.Join(t.Hosts, ", "), t.Start)
			} else {
				fmt.Fprintf(w, "  on the first running node: %s\n", t.StopCluster)
				fmt.Fprintf(w, "  then, on any node still running: %s\n", t.Stop)
			}
		default:
			cmd := t.Start
			if action == "stop" {
				cmd = t.Stop
			}
			for _, h := range t.Hosts {
				fmt.Fprintf(w, "  %-20s %s\n", h, strings.ReplaceAll(cmd, "{host}", h))
			}
		}
		fmt.Fprintf(w, "  wait for %s: %s  (up=/%s/ down=/%s/, up to %d checks, %s apart)\n\n",
			target, t.Status, t.UpPattern, t.DownPattern, t.Retries, t.RetryInterval)
	}
}
