package start

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Run shows the start menu for facts and returns what the human chose. It
// takes over the terminal until a choice is made; Ctrl+C, Esc and q
// return KindQuit. settings is the text of the read-only settings page.
func Run(ctx context.Context, facts Facts, settings string) (Choice, error) {
	// WithAltScreen draws on the terminal's alternate screen, so the
	// shell's scrollback is untouched afterwards.
	p := tea.NewProgram(New(facts, settings), tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := p.Run()
	if errors.Is(err, tea.ErrInterrupted) || errors.Is(err, tea.ErrProgramKilled) {
		return Choice{Kind: KindQuit}, nil
	}
	if err != nil {
		return Choice{}, fmt.Errorf("start menu: %w", err)
	}
	// p.Run returns the last model as a tea.Model, an interface; the type
	// assertion gets our Model back.
	return final.(Model).Choice(), nil
}
