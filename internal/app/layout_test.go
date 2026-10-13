package app

import "testing"

func TestSplit(t *testing.T) {
	for _, tt := range []struct {
		name                string
		flex                int
		companion, slot     bool
		wantA, wantC, wantS int
	}{
		{"activity only", 28, false, false, 28, 0, 0},
		{"activity and companion", 28, true, false, 14, 14, 0},
		{"activity and slot", 28, false, true, 21, 0, 7},
		{"all three", 28, true, true, 7, 14, 7},
		// Odd heights: divisions round down, the rest goes to the
		// Companion, or to the Activity when the Companion is closed.
		{"odd, activity only", 29, false, false, 29, 0, 0},
		{"odd, activity and companion", 29, true, false, 14, 15, 0},
		{"odd, activity and slot", 29, false, true, 22, 0, 7},
		{"odd, all three", 31, true, true, 7, 17, 7},
		{"smallest terminal, all three", 18, true, true, 4, 10, 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, c, s := split(tt.flex, tt.companion, tt.slot)
			if a != tt.wantA || c != tt.wantC || s != tt.wantS {
				t.Errorf("split(%d, %t, %t) = %d, %d, %d; want %d, %d, %d",
					tt.flex, tt.companion, tt.slot, a, c, s, tt.wantA, tt.wantC, tt.wantS)
			}
			if a+c+s != tt.flex {
				t.Errorf("rows add up to %d, want %d", a+c+s, tt.flex)
			}
		})
	}
}

func TestFixedRowsNeverChange(t *testing.T) {
	// Whatever is open and however tall the terminal, the project pane,
	// text field and help line keep their heights: the text field always
	// starts inputRows+helpRows above the bottom.
	for height := minHeight; height <= 60; height++ {
		for _, companion := range []bool{false, true} {
			for _, slot := range []bool{false, true} {
				l := computeLayout(height, companion, slot)
				if l.activityTop != projectRows {
					t.Errorf("height %d: activity starts at row %d", height, l.activityTop)
				}
				if l.inputTop != height-inputRows-helpRows {
					t.Errorf("height %d, companion %t, slot %t: text field at row %d, want %d",
						height, companion, slot, l.inputTop, height-inputRows-helpRows)
				}
			}
		}
	}
}
