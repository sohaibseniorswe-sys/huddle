package planning

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

func good(t *testing.T) Draft {
	t.Helper()
	b, err := os.ReadFile("testdata/good.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestStandardFixtures(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*Draft)
		want   string
	}{
		{name: "good"},
		{name: "missing README section", change: func(d *Draft) { d.README = "# Objective\nOnly objective" }, want: "Definition of Done"},
		{name: "empty required section", change: func(d *Draft) { d.Issues[0].Text = strings.Replace(d.Issues[0].Text, "Return the word READY.", "", 1) }, want: "Requirements"},
		{name: "process language", change: func(d *Draft) { d.Issues[0].Text += "\nCommit your changes and open a PR." }, want: "process language"},
		{name: "dependency cycle", change: func(d *Draft) { d.Issues[0].DependsOn = []string{d.Issues[0].ID} }, want: "cycle"},
		{name: "unknown dependency", change: func(d *Draft) { d.Issues[0].DependsOn = []string{"missing"} }, want: "unknown dependency"},
		{name: "duplicate dependency", change: func(d *Draft) { d.Issues[0].DependsOn = []string{"missing", "missing"} }, want: "duplicate dependency"},
		{name: "invalid id", change: func(d *Draft) { d.Issues[0].ID = "../../escape" }, want: "invalid issue id"},
		{name: "empty issues", change: func(d *Draft) { d.Issues = nil }, want: "at least one issue"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := good(t)
			if tt.change != nil {
				tt.change(&d)
			}
			var text []string
			for _, p := range d.Check() {
				text = append(text, p.String())
			}
			joined := strings.Join(text, ";")
			if tt.want == "" && joined != "" || tt.want != "" && !strings.Contains(joined, tt.want) {
				t.Fatalf("problems %s", joined)
			}
		})
	}
}

func TestParseAndDependencyOrder(t *testing.T) {
	for _, text := range []string{`{"titel":"typo"}`, `{} {}`, `not json`} {
		if _, err := Parse(text); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
	d := good(t)
	first := d.Issues[0]
	second := first
	second.ID = "02-dependent"
	second.DependsOn = []string{first.ID}
	d.Issues = []Issue{second, first}
	ordered, err := d.Order()
	if err != nil || ordered[0].ID != first.ID || ordered[1].ID != second.ID {
		t.Fatalf("order %+v %v", ordered, err)
	}
}

func TestPortableDraftAndConversationResume(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, store.Project{ID: "id-only", Phase: "planning"}); err != nil {
		t.Fatal(err)
	}
	w := Workspace{Store: s, ProjectID: "id-only"}
	d := good(t)
	if err := w.History().Append(companion.Entry{Time: time.Now(), Who: companion.WhoYou, Text: "Keep the READY response."}); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := w.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.Open(ctx, t.TempDir(), s.Data())
	if err != nil {
		t.Fatal(err)
	}
	resumed := Workspace{Store: fresh, ProjectID: w.ProjectID}
	got, exists, err := resumed.Load()
	if err != nil || !exists || got.Title != d.Title {
		t.Fatalf("resume %+v %v %v", got, exists, err)
	}
	context, err := resumed.Context()
	if err != nil || !strings.Contains(context, "Keep the READY response") || !strings.Contains(context, d.Slug) {
		t.Fatalf("missing durable context %s %v", context, err)
	}
	if _, err := os.Stat(filepath.Join(resumed.Dir(), "issues", d.Issues[0].ID+".md")); err != nil {
		t.Fatal(err)
	}
	d.README = "incomplete"
	if err := resumed.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, _, err = resumed.Load()
	if err != nil || len(got.Check()) == 0 {
		t.Fatal("incomplete draft was lost")
	}
}

func TestInstructionsAndRoundTrip(t *testing.T) {
	text := Instructions("")
	if !strings.Contains(text, "MILESTONE SPEC STANDARD") || strings.Contains(text, "<plan>\nmilestone:") {
		t.Fatal("legacy plan protocol remains")
	}
	d := good(t)
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(string(b))
	if err != nil || len(got.Check()) != 0 {
		t.Fatalf("roundtrip %+v %v", got, err)
	}
}
