package prompts

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestLaunchGolden(t *testing.T) {
	for _, role := range []Role{Dev, PM} {
		t.Run(string(role), func(t *testing.T) {
			got, err := Launch(role, "V3", 58)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", string(role)+".golden")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/prompts -update to create it)", err)
			}
			if got != string(want) {
				t.Errorf("Launch(%s) differs from %s:\n%s", role, path, got)
			}
		})
	}
}

func TestLaunchFilled(t *testing.T) {
	for _, role := range []Role{Dev, PM} {
		got, err := Launch(role, "Sandbox M1", 3)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "{{") || strings.Contains(got, "}}") {
			t.Errorf("Launch(%s) has an unfilled placeholder:\n%s", role, got)
		}
		for _, want := range []string{`milestone "Sandbox M1"`, "Control issue #3 is", `--milestone "Sandbox M1"`,
			"\n\nIt's your turn. Read control issue #3 and continue.\n"} {
			if !strings.Contains(got, want) {
				t.Errorf("Launch(%s) missing %q", role, want)
			}
		}
		if !strings.HasPrefix(got, "HOW HUDDLE WORKS\n") {
			t.Errorf("Launch(%s) does not start with the shared section", role)
		}
	}
	if _, err := Launch("reviewer", "V3", 1); err == nil {
		t.Error("Launch(unknown role) error = nil")
	}
}

// TestEmbeddedMatchesPlan checks the embedded files against the three code
// blocks under "Operating principles (embed exactly)" in MVP_PLAN.md.
func TestEmbeddedMatchesPlan(t *testing.T) {
	plan, err := os.ReadFile(filepath.Join("..", "..", "MVP_PLAN.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(plan), "### Operating principles (embed exactly)")
	if !ok {
		t.Fatal("MVP_PLAN.md has no operating principles section")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	blocks := regexp.MustCompile("(?s)```\n(.*?)```").FindAllStringSubmatch(section, -1)
	if len(blocks) != 3 {
		t.Fatalf("found %d code blocks in the section, want 3", len(blocks))
	}
	for i, embedded := range []struct{ name, text string }{{"shared.md", shared}, {"dev.md", dev}, {"pm.md", pm}} {
		if embedded.text != blocks[i][1] {
			t.Errorf("%s differs from MVP_PLAN.md section 9:\n got: %q\nwant: %q", embedded.name, embedded.text, blocks[i][1])
		}
	}
	// The only placeholders are the two the plan names.
	for _, m := range regexp.MustCompile(`\{\{[^}]*\}\}`).FindAllString(shared+dev+pm, -1) {
		if m != "{{milestone}}" && m != "{{control_issue}}" {
			t.Errorf("unexpected placeholder %s", m)
		}
	}
}
