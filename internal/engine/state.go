// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"regexp"
	"strings"
)

// State of a host or tier.
type State int

const (
	Unknown State = iota
	Up
	Down
	Partial
)

func (s State) String() string {
	switch s {
	case Up:
		return "UP"
	case Down:
		return "DOWN"
	case Partial:
		return "PARTIAL"
	}
	return "UNKNOWN"
}

// HostState is the observed state on one host.
type HostState struct {
	Host  string
	State State
	Lines []string // relevant status lines, shown in the report
	Err   error
	Extra string // e.g. "paused" for metadata cluster nodes
}

// TierState is the combined state of a tier.
type TierState struct {
	State State
	Hosts []HostState
	Note  string
}

// classify decides a host state from status output and up/down patterns.
//
//	up matched, down not  -> Up
//	down matched, up not  -> Down
//	both matched          -> Partial (some services up, some down)
//	neither               -> Unknown
func classify(output string, up, down *regexp.Regexp) (State, []string) {
	var lines []string
	upHit, downHit := false, false
	for _, l := range strings.Split(output, "\n") {
		u := up != nil && up.MatchString(l)
		d := down != nil && down.MatchString(l)
		if u || d {
			lines = append(lines, strings.TrimSpace(l))
		}
		upHit = upHit || u
		downHit = downHit || d
	}
	switch {
	case upHit && !downHit:
		return Up, lines
	case downHit && !upHit:
		return Down, lines
	case upHit && downHit:
		return Partial, lines
	}
	return Unknown, lines
}

// combine merges host states into a tier state.
func combine(hosts []HostState) State {
	if len(hosts) == 0 {
		return Unknown
	}
	ups, downs := 0, 0
	for _, h := range hosts {
		switch h.State {
		case Unknown:
			return Unknown
		case Up:
			ups++
		case Down:
			downs++
		}
	}
	switch {
	case ups == len(hosts):
		return Up
	case downs == len(hosts):
		return Down
	}
	return Partial
}
