package companion

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryPath(t *testing.T) {
	got := NewHistory("/home/u", "sohaibseniorswe-sys/palantir", 58).Path
	if want := "/home/u/companion/sohaibseniorswe-sys_palantir_58.jsonl"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestHistoryRoundTrip(t *testing.T) {
	h := NewHistory(t.TempDir(), "o/r", 3)
	if got, err := h.Load(); err != nil || len(got) != 0 {
		t.Fatalf("Load() of a missing file = %v, %v; want nothing, no error", got, err)
	}
	when := time.Date(2026, 10, 14, 0, 2, 0, 0, time.UTC)
	want := []Entry{
		{Time: when, Who: WhoYou, Text: "Use OpenStreetMap for now."},
		{Time: when, Who: WhoCompanion, Text: "MapLibre needs no key.\nHere's the decision:"},
		{Time: when, Who: WhoDecision, Text: "Use MapLibre for #65.", Reason: "no token yet"},
	}
	for _, e := range want {
		if err := h.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	// A new History on the same path, as after a restart.
	got, err := History{Path: h.Path}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("Load() = %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].Time.Equal(want[i].Time) || got[i].Who != want[i].Who || got[i].Text != want[i].Text || got[i].Reason != want[i].Reason {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestHistorySkipsATornLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	data := `{"time":"2026-10-14T00:02:00Z","who":"you","text":"hi"}` + "\n" + `{"time":"2026-10-14T00:0`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := History{Path: path}.Load()
	if err != nil || len(got) != 1 || got[0].Text != "hi" {
		t.Errorf("Load() = %+v, %v; want the one whole entry", got, err)
	}
}
