package start

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

func TestQueue(t *testing.T) {
	home := t.TempDir()
	q := NewQueue(home, "o/r")
	if want := filepath.Join(home, "queue", "o_r.json"); q.Path != want {
		t.Errorf("Path = %q, want %q", q.Path, want)
	}
	got, err := q.Load()
	if err != nil || len(got) != 0 {
		t.Fatalf("empty queue: %v, %v", got, err)
	}
	if err := q.Add(12, 14); err != nil {
		t.Fatal(err)
	}
	if err := q.Add(14, 20); err != nil {
		t.Fatal(err)
	}
	// A new Queue value, as after a restart, reads the same file.
	got, _ = NewQueue(home, "o/r").Load()
	if !slices.Equal(got, []int{12, 14, 20}) {
		t.Errorf("queue = %v, want [12 14 20]", got)
	}
	if err := q.Remove(14); err != nil {
		t.Fatal(err)
	}
	if err := q.Remove(99); err != nil {
		t.Fatal(err)
	}
	got, _ = q.Load()
	if !slices.Equal(got, []int{12, 20}) {
		t.Errorf("queue = %v, want [12 20]", got)
	}
}

func TestQueueCorruptFile(t *testing.T) {
	q := NewQueue(t.TempDir(), "o/r")
	if err := os.MkdirAll(filepath.Dir(q.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(q.Path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Load(); err == nil {
		t.Error("no error for a corrupt queue file")
	}
}

func TestQueueNext(t *testing.T) {
	f := &gh.Fake{Issues: []gh.Issue{
		{Number: 5, Milestone: "M4", Labels: []string{"control"}, Closed: true}, // closed by hand
		{Number: 7, Milestone: "M5", Labels: []string{"control"}},
		{Number: 9, Milestone: "M6", Labels: []string{"control", gh.StateHumanRequired}},
	}}
	q := NewQueue(t.TempDir(), "o/r")
	if err := q.Add(5, 3, 7, 9); err != nil { // #3 does not exist
		t.Fatal(err)
	}
	is, ok, err := q.Next(context.Background(), f, "o/r")
	if err != nil || !ok || is.Number != 7 || is.Milestone != "M5" {
		t.Fatalf("Next = %+v, %v, %v; want #7", is, ok, err)
	}
	if !slices.Contains(f.Issues[1].Labels, gh.StateDevReady) {
		t.Errorf("#7 labels = %q, want state:dev-ready", f.Issues[1].Labels)
	}
	if got, _ := q.Load(); !slices.Equal(got, []int{7, 9}) {
		t.Errorf("queue = %v, want the closed and missing ones gone", got)
	}

	// A queued issue that already has a state label keeps it.
	_ = q.Remove(7)
	is, _, _ = q.Next(context.Background(), f, "o/r")
	if is.Number != 9 || !slices.Equal(f.Issues[2].Labels, []string{"control", gh.StateHumanRequired}) {
		t.Errorf("Next = #%d with labels %q", is.Number, f.Issues[2].Labels)
	}

	_ = q.Remove(9)
	if _, ok, err := q.Next(context.Background(), f, "o/r"); ok || err != nil {
		t.Errorf("empty queue: ok %v, err %v", ok, err)
	}
}
