package wizard

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/config"
)

// ErrCanceled is returned by Run when the human quits without saving.
var ErrCanceled = errors.New("setup canceled; nothing was saved")

// Run shows the wizard with the values of c pre-selected, and returns the
// settings it saved. It takes over the terminal until the wizard ends.
func Run(ctx context.Context, deps Deps, c config.Config) (config.Config, error) {
	// WithAltScreen draws on the terminal's alternate screen, so the
	// shell's scrollback is untouched; WithMouseCellMotion turns on
	// clicks, so a chip can be clicked.
	p := tea.NewProgram(New(deps, c), tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) || errors.Is(err, tea.ErrProgramKilled) {
			return config.Config{}, ErrCanceled
		}
		return config.Config{}, fmt.Errorf("setup wizard: %w", err)
	}
	// p.Run returns the last model as a tea.Model, an interface; the type
	// assertion gets our Model back to ask it what was saved.
	saved, ok := final.(Model).Saved()
	if !ok {
		return config.Config{}, ErrCanceled
	}
	return saved, nil
}
