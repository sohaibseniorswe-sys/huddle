package commands

import (
	"slices"
	"testing"
)

func names(cmds []Command) []string {
	var out []string
	for _, c := range cmds {
		out = append(out, c.Name)
	}
	return out
}

func TestVisible(t *testing.T) {
	for _, tt := range []struct {
		name  string
		state State
		want  []string
	}{
		{"agent running", State{AgentActive: true},
			[]string{Status, Companion, Pause, Log, Settings, Quit}},
		{"paused", State{AgentActive: true, Paused: true},
			[]string{Status, Companion, Resume, Log, Settings, Quit}},
		{"after talking to the companion", State{Conversation: true},
			[]string{Status, Companion, Decide, Log, Settings, Quit}},
		{"a card waits", State{Conversation: true, CardWaiting: true},
			[]string{Status, Companion, Decide, Decision, Log, Settings, Quit}},
		{"done, nothing to run", State{},
			[]string{Status, Companion, Log, Settings, Quit}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := names(Visible(tt.state)); !slices.Equal(got, tt.want) {
				t.Errorf("Visible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompanionLitWhileOpen(t *testing.T) {
	for _, open := range []bool{false, true} {
		cmds := Filter(Visible(State{CompanionOpen: open}), Companion)
		if len(cmds) != 1 || cmds[0].Lit != open {
			t.Errorf("open=%t: %+v", open, cmds)
		}
	}
}

func TestFilter(t *testing.T) {
	visible := Visible(State{AgentActive: true, Conversation: true, CardWaiting: true})
	for _, tt := range []struct {
		typed string
		want  []string
	}{
		{"/", []string{Status, Companion, Decide, Decision, Pause, Log, Settings, Quit}},
		{"/s", []string{Status, Settings}},
		{"/p", []string{Pause}},
		{"/dec", []string{Decide, Decision}},
		{"/decision", []string{Decision}},
		{"/log now", []string{Log}},
		{"/lo now", nil}, // with arguments only the exact name matches
		{"/x", nil},
	} {
		if got := names(Filter(visible, tt.typed)); !slices.Equal(got, tt.want) {
			t.Errorf("Filter(%q) = %v, want %v", tt.typed, got, tt.want)
		}
	}
}

func TestFilterHidesCommandsThatDoNotApply(t *testing.T) {
	// /resume exists but is hidden while running, so /r finds nothing.
	if got := Filter(Visible(State{AgentActive: true}), "/r"); len(got) != 0 {
		t.Errorf("Filter(/r) while running = %v, want none", names(got))
	}
	if got := names(Filter(Visible(State{Paused: true}), "/r")); !slices.Equal(got, []string{Resume}) {
		t.Errorf("Filter(/r) while paused = %v, want /resume", got)
	}
}

func TestSplit(t *testing.T) {
	name, args := Split("  /log   older runs  ")
	if name != Log || args != "older runs" {
		t.Errorf("Split() = %q, %q", name, args)
	}
}
