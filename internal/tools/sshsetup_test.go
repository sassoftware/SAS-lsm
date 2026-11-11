package tools

import (
	"reflect"
	"strings"
	"testing"
)

func withFiles(t *testing.T, files map[string]string) {
	old := fipsProbe
	fipsProbe = func(p string) string { return files[p] }
	t.Cleanup(func() { fipsProbe = old })
}

func TestFIPSDetection(t *testing.T) {
	withFiles(t, map[string]string{"/proc/sys/crypto/fips_enabled": "1"})
	if on, _ := FIPSEnabled(); !on {
		t.Fatal("kernel flag not detected")
	}
	withFiles(t, map[string]string{"/proc/sys/crypto/fips_enabled": "0", "/etc/crypto-policies/state/current": "FIPS:OSPP"})
	if on, _ := FIPSEnabled(); !on {
		t.Fatal("crypto policy not detected")
	}
	withFiles(t, map[string]string{"/proc/sys/crypto/fips_enabled": "0", "/etc/crypto-policies/state/current": "DEFAULT"})
	if on, _ := FIPSEnabled(); on {
		t.Fatal("false positive")
	}
	withFiles(t, map[string]string{})
	if on, _ := FIPSEnabled(); on {
		t.Fatal("false positive with no files (AIX/Solaris)")
	}
}

func TestKeyPreference(t *testing.T) {
	p, _ := keyPreference("auto", true)
	if !reflect.DeepEqual(p, []string{"ecdsa", "rsa"}) {
		t.Fatalf("fips auto = %v", p)
	}
	p, _ = keyPreference("", false)
	if p[0] != "ed25519" {
		t.Fatalf("non-fips auto = %v", p)
	}
	if _, err := keyPreference("ed25519", true); err == nil {
		t.Fatal("ed25519 must be refused in FIPS mode")
	}
	if p, _ := keyPreference("rsa", true); !reflect.DeepEqual(p, []string{"rsa"}) {
		t.Fatalf("explicit rsa = %v", p)
	}
	if _, err := keyPreference("dsa", false); err == nil {
		t.Fatal("unknown type accepted")
	}
}

func TestKeygenArgs(t *testing.T) {
	a := keygenArgs("ecdsa", "/h/.ssh/id_ecdsa", "c")
	if a[len(a)-2] != "-b" || a[len(a)-1] != "384" {
		t.Fatalf("ecdsa args %v", a)
	}
	a = keygenArgs("rsa", "/h/.ssh/id_rsa", "c")
	if a[len(a)-1] != "4096" {
		t.Fatalf("rsa args %v", a)
	}
}

func TestSSHReasonSkipsFIPSWarning(t *testing.T) {
	out := "sshkey_read: Ed25519 keys are not allowed in FIPS mode\nsas@host: Permission denied (publickey,gssapi-keyex,gssapi-with-mic,password)."
	if got := sshReason(out); got != "sas@host: Permission denied (publickey,gssapi-keyex,gssapi-with-mic,password)." {
		t.Fatalf("got %q", got)
	}
	if got := sshReason("sshkey_read: Ed25519 keys are not allowed in FIPS mode\nsomething odd"); got != "something odd" {
		t.Fatalf("got %q", got)
	}
}

func TestClassifySSH(t *testing.T) {
	cases := []struct {
		ok      bool
		out     string
		state   hostState
		msgPart string
	}{
		{true, "", stReady, "already installed"},
		{false, "sshkey_read: Ed25519 keys are not allowed in FIPS mode\nsas@h: Permission denied (publickey,password).", stNeedsKey, "(expected)"},
		{false, "Received disconnect: Too many authentication failures", stNeedsKey, "(expected)"},
		{false, "Host key verification failed.", stHostKey, "host key has changed"},
		{false, "ssh: Could not resolve hostname nohost: Name or service not known", stUnreachable, "could not be resolved"},
		{false, "ssh: connect to host h port 22: Connection refused", stUnreachable, "sshd"},
		{false, "ssh: connect to host h port 22: No route to host", stUnreachable, "could not be reached"},
		{false, "ssh: connect to host h port 22: Connection timed out", stUnreachable, "could not be reached"},
		{false, "kex_exchange_identification: read: Connection reset by peer", stOther, "Connection reset"},
	}
	for _, c := range cases {
		st, msg := classifySSH(c.ok, c.out)
		if st != c.state || !strings.Contains(msg, c.msgPart) {
			t.Errorf("classifySSH(%q) = %v %q, want %v containing %q", c.out, st, msg, c.state, c.msgPart)
		}
	}
	// The expected first-run case must not look like an error.
	if _, msg := classifySSH(false, "Permission denied (publickey)"); strings.Contains(strings.ToLower(msg), "fail") {
		t.Errorf("needs-key message looks like an error: %q", msg)
	}
}

func TestLabels(t *testing.T) {
	want := map[hostState]string{stReady: "[ready]", stNeedsKey: "[needs key]", stHostKey: "[host key]", stUnreachable: "[problem]", stOther: "[problem]"}
	for s, l := range want {
		if s.label() != l {
			t.Errorf("label(%d)=%q want %q", s, s.label(), l)
		}
	}
}

func TestTargetsFromList(t *testing.T) {
	got := TargetsFromList([]string{"h1", "sasmid@h2", "h1"}, "sas")
	want := []SSHTarget{{"sas", "h1"}, {"sasmid", "h2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}
