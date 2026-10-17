package integration

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/publish"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

func TestSandboxPublish(t *testing.T) {
	if os.Getenv("HUDDLE_V4_SANDBOX") != "1" {
		t.Skip("set HUDDLE_V4_SANDBOX=1 to publish a throwaway sandbox milestone")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := gh.CLI{}
	stamp := time.Now().UTC().Format("20060102-150405")
	branch := "v4-publish-evidence-" + stamp
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, "gh", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("gh %v: %s %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	sha := run("api", "repos/"+sandbox+"/git/ref/heads/main", "--jq", ".object.sha")
	run("api", "-X", "POST", "repos/"+sandbox+"/git/refs", "-f", "ref=refs/heads/"+branch, "-f", "sha="+sha)
	// Retain this isolated branch as reviewable evidence; no default branch changes.
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, store.Project{ID: "publish-evidence", Repo: sandbox, Branch: branch, Phase: "planning"}); err != nil {
		t.Fatal(err)
	}
	w := planning.Workspace{Store: s, ProjectID: "publish-evidence"}
	b, err := os.ReadFile("../planning/testdata/good.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := planning.Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	d.Title = "v4 publish evidence " + stamp
	d.Slug = "v4-publish-" + stamp
	old, err := client.CreateIssue(ctx, sandbox, gh.NewIssue{Title: "Superseded fixture " + stamp, Body: "Throwaway adoption fixture."})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		out, err := exec.Command("gh", "issue", "close", fmt.Sprint(old), "--repo", sandbox).CombinedOutput()
		if err != nil {
			t.Errorf("cleanup old: %s %v", out, err)
		}
	})
	d.Issues[0].Supersedes = []int{old}
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := w.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	preview, err := publish.Review(w)
	if err != nil {
		t.Fatal(err)
	}
	result, err := publish.Publish(ctx, client, w, publish.Approval{Confirmed: true, Revision: preview.Revision})
	t.Cleanup(func() {
		for _, n := range result.Issues {
			out, err := exec.Command("gh", "issue", "close", fmt.Sprint(n), "--repo", sandbox).CombinedOutput()
			if err != nil {
				t.Errorf("cleanup issue: %s %v", out, err)
			}
		}
		if result.Control != 0 {
			out, err := exec.Command("gh", "issue", "close", fmt.Sprint(result.Control), "--repo", sandbox).CombinedOutput()
			if err != nil {
				t.Errorf("cleanup control: %s %v", out, err)
			}
		}
		if result.Milestone != 0 {
			out, err := exec.Command("gh", "api", "-X", "PATCH", fmt.Sprintf("repos/%s/milestones/%d", sandbox, result.Milestone), "-f", "state=closed").CombinedOutput()
			if err != nil {
				t.Errorf("cleanup milestone: %s %v", out, err)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	labels, err := client.ListLabels(ctx, sandbox)
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"control", gh.StateDevReady, gh.StateDevWorking, gh.StatePMReady, gh.StatePMWorking, gh.StateHumanRequired, gh.StateDone} {
		if !slices.Contains(labels, label) {
			t.Fatalf("missing label %s", label)
		}
	}
	issues, err := client.ListIssues(ctx, sandbox, "open")
	if err != nil {
		t.Fatal(err)
	}
	var control gh.Issue
	for _, is := range issues {
		if is.Number == result.Control {
			control = is
		}
	}
	if !strings.HasPrefix(control.Body, gh.ControlMarker+"\n") || !slices.Contains(control.Labels, gh.StateDevReady) {
		t.Fatalf("bad control %+v", control)
	}
	for _, file := range []gh.File{{Path: "README.md", Content: d.README}, {Path: "issues/" + d.Issues[0].ID + ".md", Content: d.Issues[0].Text}} {
		encoded := run("api", "repos/"+sandbox+"/contents/docs/"+d.Slug+"/"+file.Path+"?ref="+branch, "--jq", ".content")
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(encoded, "\n", ""))
		if err != nil || string(decoded) != file.Content {
			t.Fatalf("spec mismatch %s: %v", file.Path, err)
		}
	}
	superseded, err := client.IssueView(ctx, sandbox, old)
	if err != nil || !superseded.Closed {
		t.Fatalf("old issue not closed %+v %v", superseded, err)
	}
	comments, err := client.Comments(ctx, sandbox, old)
	if err != nil || len(comments) == 0 || !strings.Contains(comments[len(comments)-1].Body, fmt.Sprintf("/issues/%d", result.Issues[d.Issues[0].ID])) {
		t.Fatal("missing adoption link")
	}
	head := run("api", "repos/"+sandbox+"/git/ref/heads/"+branch, "--jq", ".object.sha")
	d.Title += " next"
	d.Slug += "-next"
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	next, err := publish.Review(w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publish.Publish(ctx, client, w, publish.Approval{Confirmed: true, Revision: next.Revision}); err == nil || !strings.Contains(err.Error(), "active control") {
		t.Fatalf("second active publish accepted: %v", err)
	}
	after := run("api", "repos/"+sandbox+"/git/ref/heads/"+branch, "--jq", ".object.sha")
	if after != head {
		t.Fatal("gated publish changed branch")
	}
	t.Logf("REAL sandbox publish: milestone https://github.com/%s/milestone/%d; control https://github.com/%s/issues/%d; implementation issue %v; superseded #%d closed and linked; labels ready; exact specs at https://github.com/%s/tree/%s/docs/%s; second active publish refused without branch changes.", sandbox, result.Milestone, sandbox, result.Control, result.Issues, old, sandbox, branch, preview.Draft.Slug)
}
