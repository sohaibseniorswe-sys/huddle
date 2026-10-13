package app

// The run screen is a fixed vertical stack (docs/v3/README.md, "Screen
// layout rules"). From top to bottom:
//
//	project pane   projectRows, fixed
//	Activity slot  flexible
//	Companion      flexible, when open (v3.2)
//	slot           flexible, when the menu (or, from v3.2, a card) is open
//	text field     inputRows, fixed
//	help line      helpRows, fixed
const (
	projectRows = 2
	inputRows   = 3
	helpRows    = 1
	fixedRows   = projectRows + inputRows + helpRows
)

// The smallest terminal the run screen draws in. Below it, View shows only
// tooSmallText.
const (
	minWidth     = 80
	minHeight    = 24
	tooSmallText = "Terminal too small: 80 × 24 needed"
)

// split returns how many rows Activity, Companion and the slot get out of
// the flexRows between the project pane and the text field:
//
//	open                          Activity  Companion  slot
//	Activity only                 all       -          -
//	Activity + Companion          1/2       1/2        -
//	Activity + slot               3/4       -          1/4
//	Activity + Companion + slot   1/4       1/2        1/4
//
// Divisions round down. The Companion gets what is left after the others,
// or the Activity does when the Companion is closed, so the three always
// add up to flexRows.
func split(flexRows int, companionOpen, slotOpen bool) (activity, companion, slot int) {
	switch {
	case companionOpen && slotOpen:
		activity = flexRows / 4
		slot = flexRows / 4
		companion = flexRows - activity - slot
	case companionOpen:
		activity = flexRows / 2
		companion = flexRows - activity
	case slotOpen:
		slot = flexRows / 4
		activity = flexRows - slot
	default:
		activity = flexRows
	}
	return activity, companion, slot
}

// layout is where each part of the run screen is, in rows from the top.
// It is computed from the terminal size and what is open, and used both to
// draw the screen and to find what a mouse click hit.
type layout struct {
	activityTop, activityRows   int
	companionTop, companionRows int
	slotTop, slotRows           int
	inputTop                    int
}

// computeLayout lays out a terminal height rows high.
func computeLayout(height int, companionOpen, slotOpen bool) layout {
	a, c, s := split(height-fixedRows, companionOpen, slotOpen)
	l := layout{activityTop: projectRows, activityRows: a, companionRows: c, slotRows: s}
	l.companionTop = l.activityTop + a
	l.slotTop = l.companionTop + c
	l.inputTop = l.slotTop + s
	return l
}
