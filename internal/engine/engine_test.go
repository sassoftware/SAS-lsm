package engine

import (
	"regexp"
	"testing"
)

func TestClassify(t *testing.T) {
	up, down := regexp.MustCompile(`is UP`), regexp.MustCompile(`is NOT up`)
	cases := []struct {
		out  string
		want State
	}{
		{"SAS servers status:\nSAS Metadata Server 1 is UP\nSAS Object Spawner 1 is UP\n", Up},
		{"SAS Metadata Server 1 is NOT up\nSAS Object Spawner 1 is NOT up\n", Down},
		{"SAS Metadata Server 1 is UP\nSAS Object Spawner 1 is NOT up\n", Partial},
		{"ssh: connect to host x port 22: Connection refused", Unknown},
	}
	for _, c := range cases {
		if got, _ := classify(c.out, up, down); got != c.want {
			t.Errorf("classify(%q)=%s want %s", c.out, got, c.want)
		}
	}
}

func TestCombine(t *testing.T) {
	h := func(s ...State) []HostState {
		var o []HostState
		for _, x := range s {
			o = append(o, HostState{State: x})
		}
		return o
	}
	if combine(h(Up, Up)) != Up || combine(h(Down, Down)) != Down || combine(h(Up, Down)) != Partial || combine(h(Up, Unknown)) != Unknown {
		t.Fatal("combine rules wrong")
	}
}

func TestLegacyMatch(t *testing.T) {
	yes := map[string]string{
		"01_pre_start_notify.sh":             "pre_start",
		"01_PRE_START_notify.sh":             "pre_start", // 4.x guide used upper case
		"02_post_start_success_x.sh":         "post_start_success",
		"02_post_start_x.sh":                 "post_start_success", // no result -> both
		"02_post_start_y.sh":                 "post_start_failure",
		"03_post_stop_failure_required_a.sh": "post_stop_failure",
		"04_pre_status_check.sh":             "pre_status",
	}
	for name, ev := range yes {
		if !legacyMatch(name, ev) {
			t.Errorf("%s should match %s", name, ev)
		}
	}
	no := map[string]string{
		"02_post_start_success_x.sh": "post_start_failure",
		"01_pre_start_notify.sh":     "pre_stop",
		"01_pre_status_a.sh":         "pre_start",
	}
	for name, ev := range no {
		if legacyMatch(name, ev) {
			t.Errorf("%s should NOT match %s", name, ev)
		}
	}
}
