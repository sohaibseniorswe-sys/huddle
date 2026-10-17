package start

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

func TestProjectRoutes(t *testing.T) {
	for _, tt := range []struct {
		phase string
		want  Route
	}{{"planning", RoutePlan}, {"published", RouteStart}, {"building", RouteResume}, {"done", RouteDone}} {
		got, err := ProjectRoute(tt.phase)
		if err != nil || got != tt.want {
			t.Fatalf("%s: %s %v", tt.phase, got, err)
		}
	}
	if _, err := ProjectRoute("unknown"); err == nil {
		t.Fatal("accepted unknown phase")
	}
}
func TestProjectListingAndPicker(t *testing.T) {
	ps := []store.Project{{ID: "one", Name: "Example", Phase: "building", Status: "ACTIVE"}, {ID: "two", Phase: "planning"}, {ID: "hidden", Archived: true}}
	rows := ProjectRows(ps)
	if len(rows) != 2 || !strings.Contains(rows[0], "Example · ACTIVE · building") || !strings.Contains(rows[1], "Untitled project") {
		t.Fatalf("rows %v", rows)
	}
	m := projectMenu{projects: ps[:2]}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(projectMenu).choice.ID != "one" {
		t.Fatal("wrong project selected")
	}
	m.sel = 3
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !next.(projectMenu).choice.New {
		t.Fatal("new unavailable")
	}
}
