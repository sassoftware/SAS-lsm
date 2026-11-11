package config

import (
	"strings"
	"testing"
)

const good = `
version: 1
settings:
  status_root: /var/tmp/x
defaults:
  user: sas
  configdir: /opt/sas/config/Lev1
tiers:
  - name: metadata
    type: metadata-cluster
    hosts: [m1, m2, m3]
  - name: compute
    hosts: [c1, c2]
    parallel: true
    userexits:
      pre_stop: [/opt/ue/drain.sh]
      post_start_success:
        - run: /opt/ue/agent.sh
          required: true
          on: all
  - name: midtier
    hosts: [w1]
    configdir: /opt/sas/configmid/Lev1/
  - name: grid
    type: lsf-grid
    lsf:
      master: lsfm
      nodes: [n1, n2]
      envdir: /opt/lsf/conf
      serverdir: /opt/lsf/10.1/linux/etc
  - name: app
    type: custom
    hosts: [a1]
    start: "{configdir}/app.sh start"
    stop: "{configdir}/app.sh stop"
    status: "{configdir}/app.sh status"
    up_pattern: running
    down_pattern: "not running"
`

func TestGood(t *testing.T) {
	c, err := Parse("/etc/site.yaml", good)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "site" || len(c.Tiers) != 5 {
		t.Fatalf("name=%s tiers=%d", c.Name, len(c.Tiers))
	}
	if c.Tiers[0].StopCluster != "/opt/sas/config/Lev1/SASMeta/MetadataServer/MetadataServer.sh stopCluster" {
		t.Fatalf("stopCluster=%q", c.Tiers[0].StopCluster)
	}
	if c.Tiers[2].Start != "/opt/sas/configmid/Lev1/sas.servers start" {
		t.Fatalf("mid start=%q", c.Tiers[2].Start)
	}
	if c.Tiers[1].Start != "/opt/sas/config/Lev1/sas.servers start" {
		t.Fatalf("compute start=%q (configdir must not leak between tiers)", c.Tiers[1].Start)
	}
	if ue := c.Tiers[1].UserExits["post_start_success"][0]; !ue.Required || ue.On != "all" {
		t.Fatalf("ue=%+v", ue)
	}
	if g := c.Tiers[3]; len(g.Hosts) != 3 || g.Hosts[0] != "lsfm" {
		t.Fatalf("grid hosts=%v", g.Hosts)
	}
	if a := c.Tiers[4]; a.Start != "/opt/sas/config/Lev1/app.sh start" || !a.Up.MatchString("is running") {
		t.Fatalf("custom=%+v", a)
	}
	if tt, err := c.Lookup("MIDTIER"); err != nil || tt.Index != 3 {
		t.Fatalf("lookup by name: %v %v", tt, err)
	}
	if _, err := c.Lookup("9"); err == nil {
		t.Fatal("lookup 9 should fail")
	}
}

func TestProblemsAreCollected(t *testing.T) {
	src := `
version: 1
defaults:
  retries: ten
tiers:
  - name: a
    hosts: [h]
    typo_key: 1
  - name: A
    type: metadata-cluster
    hosts: [m1]
    configdir: /x
  - name: c
    type: custom
    hosts: [h]
    start: x
    up_pattern: "("
`
	_, err := Parse("bad.yaml", src)
	if err == nil {
		t.Fatal("expected errors")
	}
	msg := err.Error()
	for _, want := range []string{"retries: must be a whole number", "unknown setting \"typo_key\"",
		"configdir' is required", "name already used", "at least 3 hosts", "requires 'stop'", "not a valid regular expression"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
}
