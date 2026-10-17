package state

import (
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	for _, tt := range []struct {
		name, flag, env, home, want string
		bad                         bool
	}{
		{name: "default", home: "/user", want: "/user/.huddle"},
		{name: "environment", env: "/isolated", want: "/isolated"},
		{name: "flag wins", flag: "/flag", env: "/env", want: "/flag"},
		{name: "missing home", bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.flag, func(k string) (string, bool) {
				v := map[string]string{"HOME": tt.home, "HUDDLE_HOME": tt.env}[k]
				return v, v != ""
			})
			if (err != nil) != tt.bad || (!tt.bad && got != filepath.Clean(tt.want)) {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}
