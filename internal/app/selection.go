package app

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

type selectionPane int

const (
	selectNone selectionPane = iota
	selectLog
	selectChat
)

type textPoint struct{ row, col int }

// textSelection uses content rows and terminal cell columns, independent of the
// viewport's scroll offset. A resize/source change clears it rather than copying
// unrelated text. Mouse capture stays enabled for the rest of the application.
type textSelection struct {
	pane           selectionPane
	key            string
	anchor, end    textPoint
	dragging       bool
	mouseX, mouseY int
}

func (s textSelection) active() bool { return s.pane != selectNone }
func (s textSelection) bounds() (textPoint, textPoint) {
	a, b := s.anchor, s.end
	if a.row > b.row || a.row == b.row && a.col > b.col {
		a, b = b, a
	}
	return a, b
}
func (s textSelection) columns(row, width int) (int, int, bool) {
	a, b := s.bounds()
	if row < a.row || row > b.row {
		return 0, 0, false
	}
	left, right := 0, width
	if row == a.row {
		left = a.col
	}
	if row == b.row {
		right = b.col + 1
	}
	return min(max(left, 0), width), min(max(right, 0), width), true
}

// plainText removes presentation styles and clips by cells, preserving Unicode.
func (s textSelection) plainText(content string) string {
	if !s.active() {
		return ""
	}
	var selected []string
	for row, line := range strings.Split(content, "\n") {
		plain := ansi.Strip(line)
		left, right, ok := s.columns(row, ansi.StringWidth(plain))
		if !ok {
			continue
		}
		selected = append(selected, strings.TrimRight(ansi.Cut(plain, left, max(left, right)), " "))
	}
	return strings.Join(selected, "\n")
}
func (s textSelection) highlight(content, key string) string {
	if !s.active() || s.key != key {
		return content
	}
	rows := strings.Split(content, "\n")
	for row, line := range rows {
		left, right, ok := s.columns(row, ansi.StringWidth(line))
		if !ok || right <= left {
			continue
		}
		rows[row] = ansi.Cut(line, 0, left) + ui.Plain.Background(ui.SelectBg).Foreground(ui.White).Render(ansi.Cut(ansi.Strip(line), left, right)) + ansi.Cut(line, right, ansi.StringWidth(line))
	}
	return strings.Join(rows, "\n")
}

func (m Model) logSelectionKey() string { return fmt.Sprintf("log:%s:%d", m.log.path(), m.log.from) }
func (m *Model) selectionSource(pane selectionPane) (string, string, *viewport.Model, int) {
	l := m.layout()
	width := m.width - ui.PaneChromeCols
	switch pane {
	case selectLog:
		if m.view == viewLog {
			return m.logSelectionKey(), m.log.content(width), &m.log.vp, l.activityTop + 2
		}
	case selectChat:
		if m.companionOpen {
			return "chat", m.chatContent(width), &m.chatVP, l.companionTop + 2
		}
	}
	return "", "", nil, 0
}

// selectionMouse handles drag gestures only in output bodies. Activity/card
// clicks and all wheel scrolling keep their existing behavior.
func (m Model) selectionMouse(msg tea.MouseMsg) (Model, tea.Cmd, bool) {
	if m.selection.dragging && (msg.Action == tea.MouseActionMotion || msg.Action == tea.MouseActionRelease) {
		m.selection.mouseX, m.selection.mouseY = msg.X, msg.Y
		m = m.moveSelection(false)
		if msg.Action == tea.MouseActionRelease {
			m.selection.dragging = false
			if m.selection.anchor == m.selection.end {
				m.selection = textSelection{}
			}
		}
		return m, nil, true
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil, false
	}
	for _, pane := range []selectionPane{selectLog, selectChat} {
		key, _, vp, top := m.selectionSource(pane)
		if vp == nil || msg.X < 2 || msg.X >= m.width-2 || msg.Y < top || msg.Y >= top+vp.Height {
			continue
		}
		point := textPoint{row: vp.YOffset + msg.Y - top, col: msg.X - 2}
		m.selection = textSelection{pane: pane, key: key, anchor: point, end: point, dragging: true, mouseX: msg.X, mouseY: msg.Y}
		m.focus = focusActivity
		if pane == selectChat {
			m.focus = focusCompanion
		}
		return m, selectionTick(), true
	}
	return m, nil, false
}

type clipboardClearMsg struct{ epoch int }

type selectionTickMsg struct{}

func selectionTick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return selectionTickMsg{} })
}

// moveSelection scrolls at the output pane edges, including when the pointer is
// stationary outside the pane. The anchor remains in content coordinates.
func (m Model) moveSelection(auto bool) Model {
	key, content, vp, top := m.selectionSource(m.selection.pane)
	if vp == nil || key != m.selection.key {
		m.selection = textSelection{}
		return m
	}
	if auto {
		if m.selection.mouseY <= top {
			scrollBy(vp, -1)
		} else if m.selection.mouseY >= top+vp.Height-1 {
			scrollBy(vp, 1)
		}
	}
	rows := strings.Split(content, "\n")
	row := min(max(vp.YOffset+min(max(m.selection.mouseY-top, 0), vp.Height-1), 0), len(rows)-1)
	m.selection.end = textPoint{row: row, col: min(max(m.selection.mouseX-2, 0), max(ansi.StringWidth(rows[row])-1, 0))}
	return m
}

// copySelection uses OSC 52 so clipboard output also works over SSH. The escape
// is rendered through Bubble Tea, avoiding concurrent writes to its terminal.
func (m Model) copySelection() Model {
	key, content, _, _ := m.selectionSource(m.selection.pane)
	if key != m.selection.key {
		return m
	}
	text := m.selection.plainText(content)
	if text == "" {
		return m
	}
	m.clipboardEpoch++
	m.clipboardFrame = "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
	return m
}
