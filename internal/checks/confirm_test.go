package checks

import (
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
