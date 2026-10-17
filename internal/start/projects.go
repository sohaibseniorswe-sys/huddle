package start

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/lifecycle"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// ProjectChoice is a registry selection. New creates an unnamed planning identity.
type ProjectChoice struct {
	ID    string
	New   bool
	Adopt bool
}

// Route identifies the screen offered when a project is opened.
type Route string

const (
	RoutePlan   Route = "resume planning"
	RouteStart  Route = "start published milestone"
	RouteResume Route = "reconcile and resume build"
	RouteDone   Route = "plan next or archive"
)

// ProjectRoute selects a destination from a freshly derived phase.
func ProjectRoute(phase string) (Route, error) {
	switch lifecycle.Phase(phase) {
	case lifecycle.Planning:
		return RoutePlan, nil
	case lifecycle.Published:
		return RouteStart, nil
	case lifecycle.Building:
		return RouteResume, nil
	case lifecycle.Done:
		return RouteDone, nil
	default:
		return "", fmt.Errorf("unknown project phase %q", phase)
	}
}

// ProjectRows produces cheap listing rows from cached registry data only.
func ProjectRows(ps []store.Project) []string {
	rows := make([]string, 0, len(ps))
	for _, p := range ps {
		if p.Archived {
			continue
		}
		name := p.Name
		if name == "" {
			name = "Untitled project"
		}
		status := p.Status
		if status == "" {
			status = "PLANNED"
			if p.Phase == "building" {
				status = "ACTIVE"
			}
			if p.Phase == "done" {
				status = "DONE"
			}
		}
		rows = append(rows, fmt.Sprintf("%s · %s · %s [%s]", name, status, p.Phase, p.ID))
	}
	return rows
}

// projectMenu is deliberately local-only: selection performs no network reads.
type projectMenu struct {
	projects []store.Project
	sel      int
	choice   ProjectChoice
}

func (m projectMenu) Init() tea.Cmd { return nil }
func (m projectMenu) View() string {
	rows := ProjectRows(m.projects)
	rows = append(rows, "Adopt current directory (plan a fresh milestone)", "New project")
	for i := range rows {
		if i == m.sel {
			rows[i] = ui.Bold.Render("› " + rows[i])
		} else {
			rows[i] = "  " + rows[i]
		}
	}
	return ui.Bold.Render("HUDDLE · Managed projects") + "\n\n" + strings.Join(rows, "\n") + "\n\n" + ui.Muted.Render("↑/↓ choose · enter open · esc quit") + "\n"
}
func (m projectMenu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.sel = max(0, m.sel-1)
		case "down", "j":
			m.sel = min(len(m.projects)+1, m.sel+1)
		case "enter":
			switch m.sel {
			case len(m.projects):
				m.choice.Adopt = true
			case len(m.projects) + 1:
				m.choice.New = true
			default:
				m.choice.ID = m.projects[m.sel].ID
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

// PickProject shows cached registry status and allows opening or creating a project.
func PickProject(ctx context.Context, ps []store.Project) (ProjectChoice, error) {
	active := make([]store.Project, 0, len(ps))
	for _, p := range ps {
		if !p.Archived {
			active = append(active, p)
		}
	}
	final, err := tea.NewProgram(projectMenu{projects: active}, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err != nil {
		return ProjectChoice{}, err
	}
	return final.(projectMenu).choice, nil
}

// ChooseDone offers the two next actions for a completed project.
func ChooseDone(ctx context.Context, p store.Project) (bool, error) {
	final, err := tea.NewProgram(doneMenu{name: p.Name}, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err != nil {
		return false, err
	}
	m := final.(doneMenu)
	if m.canceled {
		return false, context.Canceled
	}
	return m.archive, nil
}

type doneMenu struct {
	name     string
	archive  bool
	canceled bool
}

func (m doneMenu) Init() tea.Cmd { return nil }
func (m doneMenu) View() string {
	return ui.Bold.Render(m.name+" · DONE") + "\n\np: Plan next milestone\na: Archive project\n"
}
func (m doneMenu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "p", "enter":
			return m, tea.Quit
		case "a":
			m.archive = true
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.canceled = true
			return m, tea.Quit
		}
	}
	return m, nil
}
