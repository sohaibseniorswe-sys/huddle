package prompts

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// goldenLevels are the autonomy levels with golden files: one in each band
// of the AUTONOMY table, plus both ends of the dial.
var goldenLevels = []int{0, 3, 8, 10}

// TestLaunchGolden renders the launch prompt for each role, CLI and level in
// goldenLevels and compares it with testdata/<role>-<cli>-<level>.golden.
// Run go test ./internal/prompts -update to rewrite the files.
func TestLaunchGolden(t *testing.T) {
	for _, role := range []Role{Dev, PM} {
		for _, cli := range []string{"Codex", "Claude Code"} {
			for _, level := range goldenLevels {
				name := fmt.Sprintf("%s-%s-%d", role, strings.ReplaceAll(strings.ToLower(cli), " ", "-"), level)
				t.Run(name, func(t *testing.T) {
					got, err := Launch(role, cli, "V3", 58, level)
					if err != nil {
						t.Fatal(err)
					}
					path := filepath.Join("testdata", name+".golden")
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
						t.Errorf("Launch(%s, %s, level %d) differs from %s:\n%s", role, cli, level, path, got)
					}
				})
			}
		}
	}
}

func TestLaunchFilled(t *testing.T) {
	for _, role := range []Role{Dev, PM} {
		got, err := Launch(role, "Claude Code", "Sandbox M1", 3, 7)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "{{") || strings.Contains(got, "}}") {
			t.Errorf("Launch(%s) has an unfilled placeholder:\n%s", role, got)
		}
		for _, want := range []string{`milestone "Sandbox M1"`, "Control issue #3 is", `--milestone "Sandbox M1"`,
			"\n\nIt's your turn. Read control issue #3 and continue.\n",
			"agent (Claude Code), working at autonomy level 7.\n",
			"Huddle runs at autonomy level 7, on a scale"} {
			if !strings.Contains(got, want) {
				t.Errorf("Launch(%s) missing %q", role, want)
			}
		}
		if !strings.HasPrefix(got, "HOW HUDDLE WORKS\n") {
			t.Errorf("Launch(%s) does not start with the shared section", role)
		}
	}
	if _, err := Launch("reviewer", "Codex", "V3", 1, 8); err == nil {
		t.Error("Launch(unknown role) error = nil")
	}
}

// TestAutonomySectionMatchesSpec checks that the AUTONOMY section is the
// code block in docs/v3/issues/05-autonomy-level.md, word for word, and
// that it comes right after HOW HUDDLE WORKS.
func TestAutonomySectionMatchesSpec(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "v3", "issues", "05-autonomy-level.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("(?s)```\n(AUTONOMY\n.*?)```").FindStringSubmatch(string(spec))
	if m == nil {
		t.Fatal("the spec has no AUTONOMY code block")
	}
	_, after, ok := strings.Cut(shared, "\n\nAUTONOMY\n")
	if !ok {
		t.Fatal("shared.md has no AUTONOMY section after a blank line")
	}
	if got := "AUTONOMY\n" + after; got != m[1] {
		t.Errorf("AUTONOMY section differs from the spec:\n got: %q\nwant: %q", got, m[1])
	}
	if !strings.HasPrefix(shared, "HOW HUDDLE WORKS\n") || strings.Count(shared, "\n\n") != 1 {
		t.Error("shared.md should be HOW HUDDLE WORKS, then AUTONOMY, and nothing else")
	}
}

// TestOldHandoverRulesGone checks that the AUTONOMY table is the only rule
// for when to hand over: the role sections' old rules, which held at one
// level only, are gone. The end-of-milestone instructions stay.
func TestOldHandoverRulesGone(t *testing.T) {
	gone := []string{
		"Hand over to the PM (state:pm-ready) only when",
		"Set state:human-required at any other time only when",
		"Do not stop to ask for review, approval or confirmation",
		"so the human is never disturbed",
		"Prefer a sensible decision over a question",
	}
	for _, level := range goldenLevels {
		for _, role := range []Role{Dev, PM} {
			got, err := Launch(role, "Codex", "V3", 58, level)
			if err != nil {
				t.Fatal(err)
			}
			for _, phrase := range gone {
				if strings.Contains(got, phrase) {
					t.Errorf("Launch(%s, level %d) still says %q", role, level, phrase)
				}
			}
		}
	}
	for _, kept := range []string{
		"Then post a final report",
		"set state:done and close the control issue",
		"list exactly what the human must do, and set state:human-required.",
	} {
		if !strings.Contains(pm, kept) {
			t.Errorf("pm.md lost the end-of-milestone instruction %q", kept)
		}
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
	// The only placeholders are the four the plan names.
	for _, m := range regexp.MustCompile(`\{\{[^}]*\}\}`).FindAllString(shared+dev+pm, -1) {
		if m != "{{milestone}}" && m != "{{control_issue}}" && m != "{{cli}}" && m != "{{level}}" {
			t.Errorf("unexpected placeholder %s", m)
		}
	}
}

// TestPrinciplesNameRolesNotTools checks that the operating principles name
// the roles only: either CLI can play either role, so a tool's name may
// appear only where {{cli}} fills it in.
func TestPrinciplesNameRolesNotTools(t *testing.T) {
	for name, text := range map[string]string{"shared.md": shared, "dev.md": dev, "pm.md": pm} {
		for _, tool := range []string{"Codex", "Claude"} {
			if strings.Contains(text, tool) {
				t.Errorf("%s names %s", name, tool)
			}
		}
	}
	got, err := Launch(Dev, "Claude Code", "V3", 58, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "YOUR ROLE: DEV\nYou are the Dev agent (Claude Code), working at autonomy level 8.\n") {
		t.Errorf("Launch(Dev, Claude Code) does not say who plays the Dev:\n%s", got)
	}
}
