package checks

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"\n", true},
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{" YES \n", true},
		{"y", true}, // no trailing newline
		{"n\n", false},
		{"no\n", false},
		{"maybe\n", false},
		{"", false}, // EOF
	}
	for _, tt := range tests {
		if got := Confirm(strings.NewReader(tt.in), io.Discard); got != tt.want {
			t.Errorf("Confirm(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestChooseReopen(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"\n", true},
		{"1\n", true},
		{"2\n", false},
		{"x\n2\n", false}, // asks again
		{"", false},       // end of input
	}
	for _, tt := range tests {
		var out bytes.Buffer
		if got := ChooseReopen(strings.NewReader(tt.in), &out, 24, []int{22, 23}); got != tt.want {
			t.Errorf("ChooseReopen(%q) = %v, want %v", tt.in, got, tt.want)
		}
		if !strings.Contains(out.String(), "Control issue #24 is closed but 2 issues are still open (#22, #23)") ||
			!strings.Contains(out.String(), "1) Reopen and continue") || !strings.Contains(out.String(), "2) Leave it and exit") {
			t.Errorf("output = %q", out.String())
		}
	}
}
