package lanes

import (
	"strings"
	"testing"
)

func TestSingleBuildAndWarning(t *testing.T) {
	root := t.TempDir()
	lease, err := Acquire(root, "X")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, err := Acquire(root, "Y"); err == nil || !strings.Contains(err.Error(), "planning remains available") {
		t.Fatalf("second build accepted: %v", err)
	}
	if err := lease.SetCLI("claude"); err != nil {
		t.Fatal(err)
	}
	b, active, err := Active(root)
	if err != nil || !active || b.Project != "X" || b.CLI != "claude" {
		t.Fatalf("active %+v %v %v", b, active, err)
	}
	for _, tt := range []struct {
		cli          string
		active, want bool
	}{{"claude", true, true}, {"codex", true, false}, {"claude", false, false}} {
		got := Warning(tt.cli, b, tt.active)
		if (got != "") != tt.want {
			t.Fatalf("warning %q", got)
		}
	}
	isolated, err := Acquire(t.TempDir(), "isolated")
	if err != nil {
		t.Fatal(err)
	}
	isolated.Close()
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	_, active, err = Active(root)
	if err != nil || active {
		t.Fatal("stale metadata appears active")
	}
	resumed, err := Acquire(root, "Y")
	if err != nil {
		t.Fatal(err)
	}
	resumed.Close()
}
