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
			[]string{Status, Companion, Pause, Log, Quit}},
		{"paused", State{AgentActive: true, Paused: true},
			[]string{Status, Companion, Resume, Log, Quit}},
		{"human required", State{HumanRequired: true},
			[]string{Status, Companion, Reply, Log, Quit}},
		{"done, nothing to run", State{},
			[]string{Status, Companion, Log, Quit}},
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
	visible := Visible(State{AgentActive: true, HumanRequired: true})
	for _, tt := range []struct {
		typed string
		want  []string
	}{
		{"/", []string{Status, Companion, Pause, Reply, Log, Quit}},
		{"/s", []string{Status}},
		{"/p", []string{Pause}},
		{"/re", []string{Reply}},
		{"/reply yes, go ahead", []string{Reply}},
		{"/re yes", nil}, // with arguments only the exact name matches
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
	name, args := Split("  /reply   yes, use MapLibre  ")
	if name != Reply || args != "yes, use MapLibre" {
		t.Errorf("Split() = %q, %q", name, args)
	}
}
