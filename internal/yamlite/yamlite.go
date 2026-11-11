// Copyright © 2026, SAS Institute Inc., Cary, NC, USA.  All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package yamlite parses the small, predictable subset of YAML used by
// SAS_lsm configuration files. It exists so SAS_lsm can be built with the Go
// standard library only (no third-party modules to vet or vendor).
//
// Supported:
//   - block mappings        key: value
//   - block sequences       - item   /   - key: value
//   - flow sequences        [a, b, "c d"]
//   - scalars               plain, 'single quoted', "double quoted" (\" \\ \n \t)
//   - comments              # ... (whole line, or after whitespace)
//
// Not supported (rejected with an error): anchors/aliases (&, *), tags (!!),
// flow mappings ({}), block scalars (| and >), multi-document streams.
//
// Values are returned as string, []any or *Map. Scalars are never typed here;
// the config package converts them (int, bool, ...) with clear errors.
package yamlite

import (
	"fmt"
	"strings"
)

// Map is an insertion-ordered mapping with line information for diagnostics.
type Map struct {
	Keys   []string
	Values map[string]any
	Lines  map[string]int
}

// Get returns the value for key and whether it was present.
func (m *Map) Get(key string) (any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m.Values[key]
	return v, ok
}

// Line returns the 1-based source line of key, or 0.
func (m *Map) Line(key string) int {
	if m == nil {
		return 0
	}
	return m.Lines[key]
}

type line struct {
	num    int // 1-based
	indent int
	text   string // content without indentation and comments
}

// Error is a parse error with a line number.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

func errf(n int, f string, a ...any) error { return &Error{Line: n, Msg: fmt.Sprintf(f, a...)} }

// Parse parses a document and returns its root value.
func Parse(src string) (any, error) {
	lines, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return &Map{Values: map[string]any{}, Lines: map[string]int{}}, nil
	}
	p := &parser{lines: lines}
	v, err := p.node(lines[0].indent)
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.lines) {
		l := p.lines[p.pos]
		return nil, errf(l.num, "unexpected indentation or content %q", l.text)
	}
	return v, nil
}

func tokenize(src string) ([]line, error) {
	var out []line
	src = strings.ReplaceAll(src, "\r\n", "\n")
	for i, raw := range strings.Split(src, "\n") {
		n := i + 1
		if strings.Contains(leadingWS(raw), "\t") {
			return nil, errf(n, "tab characters are not allowed for indentation; use spaces")
		}
		trimmed := strings.TrimLeft(raw, " ")
		indent := len(raw) - len(trimmed)
		content := strings.TrimRight(stripComment(trimmed), " \t")
		if content == "" {
			continue
		}
		if content == "---" && len(out) == 0 {
			continue
		}
		if content == "---" || content == "..." {
			return nil, errf(n, "multiple YAML documents are not supported")
		}
		out = append(out, line{num: n, indent: indent, text: content})
	}
	return out, nil
}

func leadingWS(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

// stripComment removes a trailing comment that is outside quotes.
func stripComment(s string) string {
	inS, inD := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inD:
			if c == '\\' {
				i++
			} else if c == '"' {
				inD = false
			}
		case inS:
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
				} else {
					inS = false
				}
			}
		case c == '"' && (i == 0 || strings.ContainsRune(" :[,-", rune(s[i-1]))):
			inD = true
		case c == '\'' && (i == 0 || strings.ContainsRune(" :[,-", rune(s[i-1]))):
			inS = true
		case c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return s[:i]
		}
	}
	return s
}

type parser struct {
	lines []line
	pos   int
}

func isSeqItem(t string) bool { return t == "-" || strings.HasPrefix(t, "- ") }

func (p *parser) node(indent int) (any, error) {
	if isSeqItem(p.lines[p.pos].text) {
		return p.sequence(indent)
	}
	return p.mapping(indent)
}

func (p *parser) sequence(indent int) (any, error) {
	var out []any
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		if l.indent < indent {
			break
		}
		if l.indent > indent {
			return nil, errf(l.num, "unexpected indentation")
		}
		if !isSeqItem(l.text) {
			break
		}
		rest := strings.TrimLeft(strings.TrimPrefix(l.text, "-"), " ")
		if rest == "" {
			p.pos++
			if p.pos >= len(p.lines) || p.lines[p.pos].indent <= indent {
				out = append(out, "")
				continue
			}
			v, err := p.node(p.lines[p.pos].indent)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		childIndent := indent + (len(l.text) - len(rest))
		if _, _, isKV := splitKey(rest); isKV || isSeqItem(rest) {
			// Re-read this item as the first line of a nested block.
			p.lines[p.pos] = line{num: l.num, indent: childIndent, text: rest}
			v, err := p.node(childIndent)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		v, err := scalarOrFlow(rest, l.num)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		p.pos++
	}
	return out, nil
}

func (p *parser) mapping(indent int) (any, error) {
	m := &Map{Values: map[string]any{}, Lines: map[string]int{}}
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		if l.indent < indent {
			break
		}
		if l.indent > indent {
			return nil, errf(l.num, "unexpected indentation")
		}
		if isSeqItem(l.text) {
			return nil, errf(l.num, "list item found where a 'key: value' entry was expected")
		}
		key, val, ok := splitKey(l.text)
		if !ok {
			return nil, errf(l.num, "expected 'key: value', found %q", l.text)
		}
		if _, dup := m.Values[key]; dup {
			return nil, errf(l.num, "duplicate key %q (first defined on line %d)", key, m.Lines[key])
		}
		p.pos++
		var v any
		var err error
		if val == "" {
			switch {
			case p.pos < len(p.lines) && p.lines[p.pos].indent > indent:
				v, err = p.node(p.lines[p.pos].indent)
			case p.pos < len(p.lines) && p.lines[p.pos].indent == indent && isSeqItem(p.lines[p.pos].text):
				v, err = p.sequence(indent)
			default:
				v = ""
			}
		} else {
			v, err = scalarOrFlow(val, l.num)
		}
		if err != nil {
			return nil, err
		}
		m.Keys = append(m.Keys, key)
		m.Values[key] = v
		m.Lines[key] = l.num
	}
	return m, nil
}

// splitKey splits "key: value" (value may be empty). Keys must be plain words.
func splitKey(t string) (string, string, bool) {
	if t == "" || strings.ContainsAny(t[:1], "\"'[{&*!|>%@`") {
		return "", "", false
	}
	idx := -1
	for i := 0; i < len(t); i++ {
		if t[i] == ':' && (i == len(t)-1 || t[i+1] == ' ') {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(t[:idx])
	if strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	return key, strings.TrimSpace(t[idx+1:]), true
}

func scalarOrFlow(s string, n int) (any, error) {
	switch {
	case strings.HasPrefix(s, "["):
		return flowSeq(s, n)
	case strings.HasPrefix(s, "{"):
		return nil, errf(n, "flow mappings ({...}) are not supported; use indented 'key: value' lines")
	case s == "|" || s == ">" || strings.HasPrefix(s, "|-") || strings.HasPrefix(s, ">-") || s == "|+" || s == ">+":
		return nil, errf(n, "block scalars (| or >) are not supported; put the value on one line in quotes")
	case strings.HasPrefix(s, "&") || strings.HasPrefix(s, "*"):
		return nil, errf(n, "anchors and aliases (& and *) are not supported")
	case strings.HasPrefix(s, "!"):
		return nil, errf(n, "YAML tags (!!type) are not supported")
	}
	return scalar(s, n)
}

func scalar(s string, n int) (string, error) {
	if s == "" {
		return "", nil
	}
	switch s[0] {
	case '"':
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				i++
				switch s[i] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				case '"', '\\', '/':
					b.WriteByte(s[i])
				default:
					return "", errf(n, "unsupported escape \\%c in double-quoted string", s[i])
				}
				continue
			}
			if c == '"' {
				if strings.TrimSpace(s[i+1:]) != "" {
					return "", errf(n, "unexpected text after closing quote: %q", s[i+1:])
				}
				return b.String(), nil
			}
			b.WriteByte(c)
		}
		return "", errf(n, "unterminated double-quoted string")
	case '\'':
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			if s[i] == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					b.WriteByte('\'')
					i++
					continue
				}
				if strings.TrimSpace(s[i+1:]) != "" {
					return "", errf(n, "unexpected text after closing quote: %q", s[i+1:])
				}
				return b.String(), nil
			}
			b.WriteByte(s[i])
		}
		return "", errf(n, "unterminated single-quoted string")
	}
	if s == "~" || s == "null" {
		return "", nil
	}
	return s, nil
}

func flowSeq(s string, n int) (any, error) {
	if !strings.HasSuffix(s, "]") {
		return nil, errf(n, "flow list must start with [ and end with ] on the same line")
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	out := []any{}
	if inner == "" {
		return out, nil
	}
	var cur strings.Builder
	inS, inD := false, false
	flush := func() error {
		item := strings.TrimSpace(cur.String())
		cur.Reset()
		if item == "" {
			return errf(n, "empty item in flow list")
		}
		if strings.HasPrefix(item, "[") || strings.HasPrefix(item, "{") {
			return errf(n, "nested flow collections are not supported")
		}
		v, err := scalar(item, n)
		if err != nil {
			return err
		}
		out = append(out, v)
		return nil
	}
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch {
		case inD:
			if c == '\\' && i+1 < len(inner) {
				cur.WriteByte(c)
				i++
				c = inner[i]
			} else if c == '"' {
				inD = false
			}
		case inS:
			if c == '\'' {
				inS = false
			}
		case c == '"':
			inD = true
		case c == '\'':
			inS = true
		case c == ',':
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		cur.WriteByte(c)
	}
	if inS || inD {
		return nil, errf(n, "unterminated quote in flow list")
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}
