package gh

import "testing"

func TestPublishBranchValidation(t *testing.T) {
	for _, tt := range []struct {
		branch string
		valid  bool
	}{{"v4", true}, {"feature/project-lifecycle", true}, {"main", true}, {"", false}, {"HEAD", false}, {"../escape", false}, {"a//b", false}, {"name.lock", false}, {"name/", false}, {"a b", false}, {"a@{1}", false}} {
		if got := ValidBranch(tt.branch); got != tt.valid {
			t.Fatalf("branch %q valid=%v want %v", tt.branch, got, tt.valid)
		}
	}
}
