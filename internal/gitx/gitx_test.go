package gitx

import (
	"reflect"
	"testing"
)

func TestDirtyPaths(t *testing.T) {
	tests := []struct {
		name   string
		status string
		ignore []string
		want   []string
	}{
		{"empty", "", nil, nil},
		{"untracked Session.md ignored", "?? Session.md\x00", []string{"Session.md"}, nil},
		{"modified Session.md ignored", " M Session.md\x00", []string{"Session.md"}, nil},
		{"Session.md not ignored without ignore list", "?? Session.md\x00", nil, []string{"Session.md"}},
		{"Session.md in subdirectory is not ignored", "?? docs/Session.md\x00", []string{"Session.md"}, []string{"docs/Session.md"}},
		{
			"other changes reported",
			" M main.go\x00?? Session.md\x00A  new.go\x00?? notes.txt\x00",
			[]string{"Session.md"},
			[]string{"main.go", "new.go", "notes.txt"},
		},
		{
			"rename reports new path and skips original",
			"R  new.go\x00old.go\x00 M other.go\x00",
			[]string{"Session.md"},
			[]string{"new.go", "other.go"},
		},
		{"path with spaces", "?? my file.go\x00", nil, []string{"my file.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DirtyPaths(tt.status, tt.ignore...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DirtyPaths() = %q, want %q", got, tt.want)
			}
		})
	}
}
