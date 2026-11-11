package yamlite

import (
	"strings"
	"testing"
)

func TestParseConfigShape(t *testing.T) {
	src := `
# comment
version: 1
defaults:
  user: sas            # trailing comment
  configdir: "/opt/sas/config/Lev1"
tiers:
  - name: metadata
    type: metadata-cluster
    hosts: [meta1, meta2, 'meta 3']
  - name: compute
    hosts:
      - c1
      - c2
    userexits:
      pre_stop:
        - /opt/ue/a.sh
        - run: /opt/ue/b.sh # x
          required: true
    start: "echo \"hi\" # not a comment"
list_at_same_indent:
- a
- b
`
	v, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	root := v.(*Map)
	if got := root.Values["version"]; got != "1" {
		t.Fatalf("version=%v", got)
	}
	d := root.Values["defaults"].(*Map)
	if d.Values["user"] != "sas" || d.Values["configdir"] != "/opt/sas/config/Lev1" {
		t.Fatalf("defaults=%v", d.Values)
	}
	tiers := root.Values["tiers"].([]any)
	if len(tiers) != 2 {
		t.Fatalf("tiers=%d", len(tiers))
	}
	hosts := tiers[0].(*Map).Values["hosts"].([]any)
	if len(hosts) != 3 || hosts[2] != "meta 3" {
		t.Fatalf("hosts=%v", hosts)
	}
	t1 := tiers[1].(*Map)
	if h := t1.Values["hosts"].([]any); len(h) != 2 || h[1] != "c2" {
		t.Fatalf("block hosts=%v", h)
	}
	ue := t1.Values["userexits"].(*Map).Values["pre_stop"].([]any)
	if ue[0] != "/opt/ue/a.sh" {
		t.Fatalf("ue0=%v", ue[0])
	}
	b := ue[1].(*Map)
	if b.Values["run"] != "/opt/ue/b.sh" || b.Values["required"] != "true" {
		t.Fatalf("ue1=%v", b.Values)
	}
	if t1.Values["start"] != `echo "hi" # not a comment` {
		t.Fatalf("start=%q", t1.Values["start"])
	}
	if l := root.Values["list_at_same_indent"].([]any); len(l) != 2 {
		t.Fatalf("same indent list=%v", l)
	}
}

func TestErrors(t *testing.T) {
	cases := map[string]string{
		"a: 1\na: 2":    "duplicate key",
		"a: |\n  x":     "block scalars",
		"a: {b: 1}":     "flow mappings",
		"a: &x 1":       "anchors",
		"a:\n\tb: 1":    "tab",
		"a: \"open":     "unterminated",
		"a: 1\n   b: 2": "unexpected indentation",
		"a: [x, [y]]":   "nested",
	}
	for src, want := range cases {
		_, err := Parse(src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("src %q: want error containing %q, got %v", src, want, err)
		}
	}
}
