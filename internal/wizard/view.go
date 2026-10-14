package wizard

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// Layout of the wizard (screens 11–13): the setup bar, a blank row, the
// step list, a blank row, then the card, indented; the help line is the
// last row.
const (
	cardWidth   = 70 // the card's width inside its border
	cardMargin  = 2  // columns left of the card
	cardPadX    = 2  // the card's padding left and right
	cardPadY    = 1  // the card's padding above and below
	helpRows    = 1
	previewRows = 6 // lines of a starter file shown in step 8
)

// View draws the wizard at the terminal's size.
func (m Model) View() string {
	where := m.dir
	if where == "" {
		where = m.folder.Value()
	}
	bar := ui.SetupBar(m.width, tilde(where, m.deps.Home), m.step, stepCount)
	card := ui.SetupCard(m.cardWidth(), m.title(), m.body())
	card = lipgloss.NewStyle().MarginLeft(cardMargin).Render(card)

	parts := []string{bar, ""}
	if m.showSteps(card) {
		parts = append(parts, m.stepList(), "")
	}
	parts = append(parts, card)
	frame := lipgloss.NewStyle().Width(m.width).Height(m.height - helpRows).MaxHeight(m.height - helpRows).
		Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return frame + "\n" + m.help()
}

// cardWidth is the card's width: 70 columns, or less in a narrow terminal.
func (m Model) cardWidth() int { return min(cardWidth, m.width-cardMargin-2) }

// showSteps reports whether the step list fits above card. In a short
// terminal the list is left out, so the card keeps all its rows.
func (m Model) showSteps(card string) bool {
	return 2+stepCount+1+lipgloss.Height(card) <= m.height-helpRows
}

// cardTop is the screen row of the card's top border.
func (m Model) cardTop() int {
	card := ui.SetupCard(m.cardWidth(), m.title(), m.body())
	if m.showSteps(card) {
		return 2 + stepCount + 1
	}
	return 2
}

// fieldOrigin is the screen column and row where field i's value (its
// first chip) starts. It follows the layout of View and fieldsBlock.
func (m Model) fieldOrigin(i int) (x, y int) {
	x = cardMargin + 1 + cardPadX + ui.FieldMarkWidth + ui.FieldLabelWidth
	// Below the border and the padding: the title and a blank row, then
	// the lines before the fields, then two rows per field.
	y = m.cardTop() + 1 + cardPadY + 2 + m.preRows() + 2*i
	return x, y
}

// stepList draws the nine steps.
func (m Model) stepList() string {
	var rows []string
	for s := 1; s <= stepCount; s++ {
		value := ""
		if s < m.step {
			value = m.summaryLine(s)
		}
		rows = append(rows, ansi.Truncate(ui.StepRow(s, stepNames[s-1], value, m.step), m.width, "…"))
	}
	return strings.Join(rows, "\n")
}

// title is the card's title.
func (m Model) title() string {
	switch m.step {
	case stepFolder:
		return "Where is the project?"
	case stepChecks:
		return "Logins and test runs"
	case stepAutonomy:
		return "How independently should the agents work?"
	}
	return stepNames[m.step-1]
}

// body is the card's content below the title: the lines before the
// fields, the fields, and the notes below them.
func (m Model) body() string {
	var blocks []string
	if pre := m.pre(); pre != "" {
		blocks = append(blocks, pre)
	}
	if fs := m.fieldsBlock(); fs != "" {
		blocks = append(blocks, fs)
	}
	if note := m.note(); note != "" {
		blocks = append(blocks, note)
	}
	return strings.Join(blocks, "\n\n")
}

// preRows is the number of rows the lines before the fields take,
// including the blank row after them.
func (m Model) preRows() int {
	pre := m.pre()
	if pre == "" {
		return 0
	}
	return lipgloss.Height(pre) + 1
}

// fieldsBlock draws the step's fields, a blank row between two.
func (m Model) fieldsBlock() string {
	var rows []string
	for i, f := range m.fields() {
		focused := i == m.focus
		value := ""
		if f.id == fieldCustom {
			value = m.roles[m.step-stepDev].custom.View()
		} else {
			value = ui.Chips(f.options, f.selected, focused)
		}
		rows = append(rows, ui.Field(f.label, focused, value))
	}
	return strings.Join(rows, "\n\n")
}

// innerWidth is the width of the card's content.
func (m Model) innerWidth() int { return m.cardWidth() - 2*cardPadX }

// pre is what comes before the fields: step 9's summary.
func (m Model) pre() string {
	if m.step != stepSave {
		return ""
	}
	row := func(k, v string) string { return ui.Muted.Width(11).Render(k) + ui.Plain.Render(v) }
	rows := []string{
		row("Folder", tilde(m.dir, m.deps.Home)),
		row("Repo", m.deps.Target),
		row("Dev", m.roles[roleDev].summary()),
		row("PM", m.roles[rolePM].summary()),
		row("Companion", m.summaryLine(stepComp)),
		row("Autonomy", autonomyTitle(m.autonomy)),
		row("Settings", tilde(config.Path(m.deps.Home, m.deps.Target), m.deps.Home)),
	}
	if files := m.filesToCreate(); len(files) > 0 {
		var names []string
		for _, f := range files {
			names = append(names, f.name)
		}
		rows = append(rows, row("New files", strings.Join(names, ", ")+" (starters)"))
	}
	return strings.Join(rows, "\n")
}

// note is what comes after the fields.
func (m Model) note() string {
	switch m.step {
	case stepFolder:
		return m.folderNote()
	case stepRepo:
		return m.repoNote()
	case stepDev, stepPM:
		cli := m.roles[m.step-stepDev].agentCLI()
		note := fmt.Sprintf("Known models for %s, or %s to type any model\nname. Step 6 checks every choice with a short test run.", cli.Name, customChip)
		if m.step == stepPM {
			note += "\nEither CLI can play either role; Codex as PM saves Claude\nCode's limits."
		}
		return ui.Muted.Render(note)
	case stepComp:
		if m.companionSame {
			return ui.Muted.Render("The companion uses the PM's CLI, model and effort:\n" + m.roles[rolePM].summary())
		}
		return ui.Muted.Render("The companion only talks and reads, so a lighter model\nanswers faster and saves the PM's limits.")
	case stepChecks:
		return m.checksNote()
	case stepAutonomy:
		return m.autonomyNote()
	case stepPrepare:
		return m.prepNote()
	case stepSave:
		switch {
		case m.checking:
			return m.spinner.View() + " Saving…"
		case m.saveErr != "":
			return m.failure(m.saveErr) + "\n" + ui.Muted.Render("Enter tries again.")
		}
		return ui.Muted.Render("Enter writes the settings file. Nothing has been written yet.")
	}
	return ""
}

// failure draws an error in red, wrapped to the card.
func (m Model) failure(msg string) string {
	return ui.Fg(ui.Red).Width(m.innerWidth()).Render("✗ " + msg)
}

// ok draws a passed check.
func ok(s string) string { return ui.Fg(ui.Green).Render("✓ ") + ui.Plain.Render(s) }

func (m Model) folderNote() string {
	field := ui.Field("Folder", true, m.folder.View())
	switch {
	case m.checking:
		return field + "\n\n" + m.spinner.View() + " Checking the folder…"
	case m.folderErr != "":
		return field + "\n\n" + m.failure(m.folderErr)
	case !m.folderSeen:
		return field + "\n\n" + m.spinner.View() + " Looking for a git repo here…"
	}
	return field + "\n\n" + ui.Muted.Width(m.innerWidth()).Render(
		"The local clone of "+m.deps.Target+". Huddle runs the agents in it. Enter checks the folder; type another one to change it.")
}

func (m Model) repoNote() string {
	switch {
	case m.checking:
		return m.spinner.View() + " Reading origin and checking gh…"
	case m.repoErr != "":
		return m.failure(m.repoErr) + "\n\n" + ui.Muted.Render("Enter tries again · esc changes the folder")
	case !m.repoDone:
		return ""
	}
	row := func(k, v string) string { return ui.Muted.Width(9).Render(k) + v }
	return strings.Join([]string{
		row("Repo", ok(m.repo.repo+"   (origin)")),
		row("gh", ok(m.repo.login)),
		row("Access", ok(strings.ToLower(m.repo.permission)+": read and write")),
	}, "\n")
}

func (m Model) checksNote() string {
	var rows []string
	failedRole, failedLogin := -1, false
	for _, r := range m.checks {
		var mark string
		switch r.state {
		case checkPending:
			mark = ui.Muted.Render("·")
		case checkRunning:
			mark = m.spinner.View()
		case checkOK:
			mark = ui.Fg(ui.Green).Render("✓")
		case checkFailed:
			mark = ui.Fg(ui.Red).Render("✗")
			if r.role >= 0 && failedRole < 0 {
				failedRole = r.role
			}
			if r.role < 0 {
				failedLogin = true
			}
		}
		text := r.what
		if r.role < 0 {
			text = r.detail
			if r.state == checkRunning {
				text = "checking…"
			}
			if r.state == checkFailed {
				text = ""
			}
		} else if r.state == checkOK {
			text += ui.Muted.Render("   " + r.detail)
		}
		line := ansi.Truncate(mark+" "+ui.Plain.Width(11).Render(r.label)+text, m.innerWidth(), "…")
		rows = append(rows, line)
		if r.state == checkFailed {
			msg := ui.Fg(ui.Red).Width(m.innerWidth() - 2).Render(r.detail)
			if lines := strings.Split(msg, "\n"); len(lines) > 3 {
				msg = strings.Join(lines[:3], "\n")
			}
			rows = append(rows, lipgloss.NewStyle().MarginLeft(2).Render(msg))
		}
	}
	var hint string
	switch {
	case m.checking:
		hint = "Each test run sends a short prompt and takes a few seconds."
	case failedRole >= 0:
		hint = "Enter goes back to the " + roleNames[failedRole] + " step to change it."
	case failedLogin:
		hint = "Fix it in another terminal, then press Enter to check again."
	default:
		hint = "Every login and choice works. Enter continues."
	}
	return strings.Join(rows, "\n") + "\n\n" + ui.Muted.Render(hint)
}

// autonomyRows says, per band of the dial, when the Dev goes to the PM
// and when the PM comes to the human (docs/v3/issues/05-autonomy-level.md).
func autonomyRows(level int) (dev, pm string) {
	switch {
	case level <= 2:
		return "after every issue, and before every push or merge",
			"for every product decision and before each merge"
	case level <= 5:
		return "after every issue; small technical choices it makes itself",
			"for product decisions; it approves merges itself"
	case level <= 8:
		return "only for conflicts between issues, requirement changes or real blockers",
			"only when the world is on fire: credentials, money, access, or no way forward"
	default:
		return "only when completely blocked or done; it settles even conflicts itself",
			"only for credentials or money"
	}
}

// autonomyTitle is "8 · Autonomous".
func autonomyTitle(level int) string {
	return fmt.Sprintf("%d · %s", level, config.AutonomyName(level))
}

func (m Model) autonomyNote() string {
	var track strings.Builder
	for i := config.MinAutonomy; i <= config.MaxAutonomy; i++ {
		switch {
		case i == m.autonomy:
			track.WriteString(ui.Fg(ui.Purple).Bold(true).Render("●"))
			if i < config.MaxAutonomy {
				track.WriteString(ui.Fg(ui.Faint).Render("━━━"))
			}
		case i < m.autonomy:
			track.WriteString(ui.Fg(ui.Purple).Render("━━━━"))
		default:
			track.WriteString(ui.Fg(ui.Faint).Render("━━━━"))
		}
	}
	scale := ui.Muted.Render("0") + " " + track.String() + " " + ui.Muted.Render("10")
	row := func(k, v string) string {
		return lipgloss.JoinHorizontal(lipgloss.Top, ui.Muted.Width(21).Render(k), ui.Plain.Width(43).Render(v))
	}
	dev, pm := autonomyRows(m.autonomy)
	return scale + "\n\n" + ui.Bold.Render(autonomyTitle(m.autonomy)) + "\n\n" +
		row("Dev goes to the PM", dev) + "\n\n" + row("PM comes to you", pm)
}

// conflicts returns the rules in the agent files (or the starters about to
// be written) that fight the chosen autonomy level, as "AGENTS.md:4" and
// the line.
func (m Model) conflicts() []string {
	var out []string
	for _, f := range m.prep.files {
		if !f.exists {
			continue // a starter: written to fit every level
		}
		for _, c := range Conflicts(f.text, m.autonomy) {
			out = append(out, fmt.Sprintf("%s:%d  %s", f.name, c.Line, c.Text))
		}
	}
	return out
}

func (m Model) prepNote() string {
	switch {
	case m.checking:
		return m.spinner.View() + " Creating missing labels and reading the agent files…"
	case m.prepErr != "":
		return m.failure(m.prepErr) + "\n\n" + ui.Muted.Render("Enter tries again.")
	case !m.prepDone:
		return ""
	}
	var rows []string
	if len(m.prep.created) > 0 {
		rows = append(rows, ok("Labels ready (created "+strings.Join(m.prep.created, ", ")+")"))
	} else {
		rows = append(rows, ok("Labels ready"))
	}
	for _, f := range m.prep.files {
		if f.exists {
			rows = append(rows, ok(f.name+" present"))
		} else {
			rows = append(rows, ui.Fg(ui.Amber).Render("○ ")+ui.Plain.Render(f.name+" missing"))
		}
	}
	if cs := m.conflicts(); len(cs) > 0 {
		rows = append(rows, "", ui.Fg(ui.Amber).Width(m.innerWidth()).Render(
			fmt.Sprintf("⚠ These rules make the agents stop and wait, which fights autonomy %d (not blocking):", m.autonomy)))
		for _, c := range cs {
			rows = append(rows, ansi.Truncate("  "+c, m.innerWidth(), "…"))
		}
	}
	if f, ok := m.focusedField(); ok && f.id == fieldFile {
		rows = append(rows, "", ui.Muted.Render("Starter "+f.file+":"))
		rows = append(rows, m.preview(f.file)...)
	}
	return strings.Join(rows, "\n")
}

// preview returns the first lines of the starter for file, dimmed behind
// a bar, cut to the card's width.
func (m Model) preview(file string) []string {
	text := ""
	for _, f := range agentFiles {
		if f.name == file {
			text = f.template
		}
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	var out []string
	for i, l := range lines {
		if i == previewRows {
			out = append(out, ui.Muted.Render(fmt.Sprintf("│ … %d more lines", len(lines)-previewRows)))
			break
		}
		out = append(out, ansi.Truncate(ui.Muted.Render("│ "+l), m.innerWidth(), "…"))
	}
	return out
}

// prepSummary is step 8's line in the step list.
func (m Model) prepSummary() string {
	parts := []string{"labels ready"}
	var present, starters []string
	for _, f := range m.prep.files {
		switch {
		case f.exists:
			present = append(present, f.name)
		case m.create[f.name]:
			starters = append(starters, f.name)
		}
	}
	if len(present) > 0 {
		parts = append(parts, strings.Join(present, ", ")+" present")
	}
	if len(starters) > 0 {
		parts = append(parts, "starter "+strings.Join(starters, ", "))
	}
	if n := len(m.conflicts()); n > 0 {
		parts = append(parts, fmt.Sprintf("%d warnings", n))
	}
	return strings.Join(parts, " · ")
}

// help draws the help line: the keys that work on this step.
func (m Model) help() string {
	back := []string{"esc", "back"}
	var pairs []string
	switch m.step {
	case stepFolder:
		pairs = []string{"enter", "check folder", "ctrl+c", "quit"}
		back = nil
	case stepRepo, stepChecks:
		pairs = []string{"enter", "continue"}
	case stepAutonomy:
		pairs = []string{"←/→", "change", "enter", "confirm"}
	case stepSave:
		if len(m.fields()) > 0 {
			pairs = []string{"←/→", "choose"}
		}
		pairs = append(pairs, "enter", "save")
	default:
		if f, ok := m.focusedField(); ok && f.id == fieldCustom {
			pairs = []string{"type", "model name", "↑/↓", "field", "enter", "confirm"}
		} else if len(m.fields()) > 0 {
			pairs = []string{"↑/↓", "field", "←/→", "choose", "enter", "confirm"}
		} else {
			pairs = []string{"enter", "confirm"}
		}
	}
	return ui.Help(m.width, append(pairs, back...)...)
}

// tilde shortens a path under home to start with "~".
func tilde(path, home string) string {
	if home == "" || path == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}
