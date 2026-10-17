package start

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/store"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// PathRepair is the human's decision for a stale project checkout.
type PathRepair struct {
	Dir  string
	Drop bool
}

// StalePath reports a configured checkout that no longer exists as a directory.
func StalePath(p store.Project) (bool, error) {
	if p.Dir == "" {
		return false, nil
	}
	info, err := os.Stat(p.Dir)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !info.IsDir(), nil
}

// Repair validates and persists a replacement directory or archives the stale
// registry entry. The original repository and stable id never change.
func Repair(ctx context.Context, s store.Store, p store.Project, r PathRepair) (store.Project, error) {
	if r.Drop {
		p.Archived = true
		return p, s.Put(ctx, p)
	}
	if strings.TrimSpace(r.Dir) == "" {
		return p, fmt.Errorf("replacement directory is required")
	}
	abs, err := filepath.Abs(r.Dir)
	if err != nil {
		return p, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return p, err
	}
	if !info.IsDir() {
		return p, fmt.Errorf("%s is not a directory", abs)
	}
	p.Dir = abs
	return p, s.Put(ctx, p)
}

// Relocate prompts for a new path or an explicit drop. No build starts first.
func Relocate(ctx context.Context, p store.Project) (PathRepair, error) {
	in := textinput.New()
	in.Placeholder = "New checkout path"
	in.Focus()
	final, err := tea.NewProgram(repairMenu{project: p, input: in}, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err != nil {
		return PathRepair{}, err
	}
	m := final.(repairMenu)
	if m.canceled {
		return PathRepair{}, context.Canceled
	}
	return m.choice, nil
}

type repairMenu struct {
	project  store.Project
	input    textinput.Model
	choice   PathRepair
	canceled bool
}

func (m repairMenu) Init() tea.Cmd { return textinput.Blink }
func (m repairMenu) View() string {
	return ui.Bold.Render("Project directory unavailable") + "\n\n" + m.project.Dir + "\n\n" + m.input.View() + "\n\nEnter relocate · ctrl+d drop from listing · esc cancel\n"
}
func (m repairMenu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "ctrl+c":
			m.canceled = true
			return m, tea.Quit
		case "ctrl+d":
			m.choice.Drop = true
			return m, tea.Quit
		case "enter":
			m.choice.Dir = m.input.Value()
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
