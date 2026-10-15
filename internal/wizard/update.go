package wizard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// fieldID names a field of a step's card.
type fieldID int

const (
	fieldSource fieldID = iota // companion: same as the PM, or its own
	fieldCLI
	fieldModel
	fieldCustom // the Custom… model name, a text input
	fieldEffort
	fieldFile   // step 8: create a missing agent file or not
	fieldCommit // step 9: commit the new agent files or not
)

// field is one row of a card the human can focus and change. Fields are
// not stored: fields() builds them from the model's choices each time, so
// they never disagree with the choices.
type field struct {
	id       fieldID
	label    string
	options  []string // the chips; nil for fieldCustom
	selected int
	file     string // fieldFile: which file
}

// Chip texts.
const (
	customChip   = "Custom…"
	sameAsPM     = "Same as PM"
	separately   = "Choose separately"
	createChip   = "Create starter"
	skipChip     = "Skip"
	commitChip   = "Commit them"
	noCommitChip = "Don't commit"
)

// fields returns the current step's fields, top to bottom.
func (m Model) fields() []field {
	switch m.step {
	case stepDev, stepPM, stepComp:
		return m.agentFields(m.step - stepDev)
	case stepPrepare:
		var out []field
		for _, f := range m.missingFiles() {
			out = append(out, field{id: fieldFile, label: f, file: f,
				options: []string{createChip, skipChip}, selected: boolIndex(!m.create[f])})
		}
		return out
	case stepSave:
		if len(m.filesToCreate()) == 0 {
			return nil
		}
		return []field{{id: fieldCommit, label: "Commit", options: []string{commitChip, noCommitChip}, selected: boolIndex(!m.commit)}}
	}
	return nil
}

// agentFields are the fields of role's step: CLI, model (plus the custom
// name when Custom… is chosen) and effort, after the Source field for the
// companion. A companion that follows the PM has only the Source field.
func (m Model) agentFields(role int) []field {
	var out []field
	if role == roleComp {
		out = append(out, field{id: fieldSource, label: "Source", options: []string{sameAsPM, separately}, selected: boolIndex(!m.companionSame)})
		if m.companionSame {
			return out
		}
	}
	c := m.roles[role]
	cli := c.agentCLI()
	var clis, models []string
	for _, x := range cliOrder {
		clis = append(clis, x.Name)
	}
	for _, k := range cli.KnownModels {
		models = append(models, k.Label)
	}
	models = append(models, customChip)
	out = append(out,
		field{id: fieldCLI, label: "CLI", options: clis, selected: c.cli},
		field{id: fieldModel, label: "Model", options: models, selected: c.model})
	if c.isCustom() {
		out = append(out, field{id: fieldCustom, label: "Name"})
	}
	out = append(out, field{id: fieldEffort, label: "Effort", options: cli.Efforts, selected: c.effort})
	return out
}

// boolIndex is 1 for true and 0 for false: the chip of a two-chip field.
func boolIndex(b bool) int {
	if b {
		return 1
	}
	return 0
}

// focusedField returns the focused field; ok is false if the step has none.
func (m Model) focusedField() (field, bool) {
	fs := m.fields()
	if m.focus < 0 || m.focus >= len(fs) {
		return field{}, false
	}
	return fs[m.focus], true
}

// Update handles one message and returns the changed model, plus any
// command for Bubble Tea to run next.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m.sizeInputs(), nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg), nil
	case folderMsg:
		return m.handleFolder(msg)
	case repoMsg:
		return m.handleRepo(msg), nil
	case checkMsg:
		return m.handleCheck(msg)
	case prepMsg:
		return m.handlePrep(msg), nil
	case savedMsg:
		return m.handleSaved(msg)
	}
	return m, nil
}

// handleKey routes a key: Ctrl+C and Esc work everywhere; the rest
// depends on the step and the focused field.
func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit // nothing has been saved
	case "esc":
		return m.back(), nil
	case "enter":
		return m.confirm()
	}
	switch m.step {
	case stepFolder:
		var cmd tea.Cmd
		m.folder, cmd = m.folder.Update(k)
		m.folderErr = ""
		return m, cmd
	case stepAutonomy:
		switch k.String() {
		case "left", "h":
			m.autonomy = max(m.autonomy-1, 0)
		case "right", "l":
			m.autonomy = min(m.autonomy+1, 10)
		}
		return m, nil
	}

	f, ok := m.focusedField()
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "up":
		m = m.setFocus(m.focus - 1)
		return m, nil
	case "down", "tab":
		m = m.setFocus(m.focus + 1)
		return m, nil
	}
	if f.id == fieldCustom {
		// The custom name takes every other key, ←/→ included: they move
		// the cursor in the text.
		role := m.step - stepDev
		var cmd tea.Cmd
		m.roles[role].custom, cmd = m.roles[role].custom.Update(k)
		return m, cmd
	}
	switch k.String() {
	case "left", "h":
		m = m.choose(f, f.selected-1)
	case "right", "l":
		m = m.choose(f, f.selected+1)
	}
	return m, nil
}

// setFocus moves the focus to field i, kept within the step's fields. The
// custom name's text input shows a cursor only while it has the focus.
func (m Model) setFocus(i int) Model {
	n := len(m.fields())
	m.focus = min(max(i, 0), max(n-1, 0))
	if m.step >= stepDev && m.step <= stepComp {
		role := m.step - stepDev
		if f, ok := m.focusedField(); ok && f.id == fieldCustom {
			m.roles[role].custom.Focus()
		} else {
			m.roles[role].custom.Blur()
		}
	}
	return m
}

// choose selects chip i of field f, if there is one.
func (m Model) choose(f field, i int) Model {
	if i < 0 || i >= len(f.options) {
		return m
	}
	role := m.step - stepDev
	switch f.id {
	case fieldSource:
		m.companionSame = i == 0
	case fieldCLI:
		m.roles[role].setCLI(i)
	case fieldModel:
		m.roles[role].model = i
	case fieldEffort:
		m.roles[role].effort = i
	case fieldFile:
		m.create[f.file] = i == 0
	case fieldCommit:
		m.commit = i == 0
	}
	return m
}

// back goes to the previous step. Every choice stays as it is.
func (m Model) back() Model {
	if m.step == stepFolder {
		return m
	}
	m.checkRun++ // results of step 6 still running are dropped
	return m.enter(m.step - 1)
}

// enter shows step s with its first field focused. Steps that check
// something start their check in confirm or below.
func (m Model) enter(s int) Model {
	m.step = s
	m.checking = false
	m.focus = 0
	m = m.setFocus(0)
	if s == stepFolder {
		m.folder.Focus()
	} else {
		m.folder.Blur()
	}
	return m
}

// confirm handles Enter: it checks the step's input and moves on.
func (m Model) confirm() (tea.Model, tea.Cmd) {
	if m.checking {
		return m, nil
	}
	switch m.step {
	case stepFolder:
		m.checking = true
		return m, m.checkFolder(m.folder.Value())
	case stepRepo:
		if !m.repoDone {
			return m.startRepo()
		}
		return m.next()
	case stepDev, stepPM, stepComp:
		role := m.step - stepDev
		c := m.roles[role]
		if (role != roleComp || !m.companionSame) && c.isCustom() && c.modelValue() == "" {
			return m, nil // the custom name is empty: nothing to confirm
		}
		return m.next()
	case stepChecks:
		return m.confirmChecks()
	case stepAutonomy:
		return m.next()
	case stepPrepare:
		if !m.prepDone {
			return m.startPrepare()
		}
		return m.next()
	case stepSave:
		m.checking = true
		m.saveErr = ""
		return m, m.save()
	}
	return m, nil
}

// next moves to the step after this one and starts its check, if it has
// one.
func (m Model) next() (tea.Model, tea.Cmd) {
	m = m.enter(m.step + 1)
	switch m.step {
	case stepRepo:
		if m.repoDone && m.repoForDir == m.dir {
			return m, nil // checked before; Enter moves on
		}
		return m.startRepo()
	case stepChecks:
		return m.startChecks()
	case stepPrepare:
		return m.startPrepare()
	}
	return m, nil
}

// handleFolder takes the result of a folder check.
func (m Model) handleFolder(msg folderMsg) (tea.Model, tea.Cmd) {
	if msg.detected {
		// The detection at startup: pre-fill the folder found. If there
		// is none, the text input keeps the folder from the settings.
		m.folderSeen = true
		if msg.err == nil {
			m.folder.SetValue(msg.dir)
		}
		m.folder.CursorEnd()
		return m, nil
	}
	m.checking = false
	if msg.err != nil {
		m.folderErr = msg.err.Error()
		return m, nil
	}
	m.dir = msg.dir
	m.folder.SetValue(msg.dir)
	return m.next()
}

// startRepo starts step 2's check for the chosen folder.
func (m Model) startRepo() (tea.Model, tea.Cmd) {
	m.checking = true
	m.repoDone = false
	m.repoErr = ""
	return m, m.checkRepo(m.dir)
}

// handleRepo takes the result of step 2's check.
func (m Model) handleRepo(msg repoMsg) Model {
	if msg.dir != m.dir {
		return m // the folder changed meanwhile
	}
	m.checking = false
	if msg.err != nil {
		m.repoErr = msg.err.Error()
		return m
	}
	m.repo, m.repoDone, m.repoForDir = msg.result, true, msg.dir
	return m
}

// roleChoice is what plays role: the companion's own choice, or the PM's.
func (m Model) roleChoice(role int) agentChoice {
	if role == roleComp && m.companionSame {
		return m.roles[rolePM]
	}
	return m.roles[role]
}

// startChecks builds step 6's rows and starts the first: the API keys
// (checked at once), each distinct CLI's login, then one test run per
// role. A choice that passed a test run earlier is not run again.
func (m Model) startChecks() (tea.Model, tea.Cmd) {
	m.checkRun++
	m.checks = nil
	m.logins = nil

	keys := checkRow{label: "API keys", role: -1, state: checkOK, detail: "none set"}
	if err := checks.CheckAPIKeys(m.deps.LookupEnv); err != nil {
		keys.state, keys.detail = checkFailed, err.Error()
	}
	var bins []string
	for role := range roleCount {
		cli := m.roleChoice(role).agentCLI()
		if !slices.Contains(bins, cli.Binary) {
			bins = append(bins, cli.Binary)
			m.checks = append(m.checks, checkRow{label: cli.Name, binary: cli.Binary, role: -1})
		}
	}
	m.checks = append(m.checks, keys)
	for role := range roleCount {
		c := m.roleChoice(role)
		what := c.summary()
		if role == roleComp && m.companionSame {
			what = "same as PM"
		}
		row := checkRow{label: roleNames[role], what: what, role: role, key: c.choiceKey()}
		switch {
		case role == roleComp && m.companionSame:
			row.state, row.detail = checkOK, "tested as the PM"
		case m.validated[row.key]:
			row.state, row.detail = checkOK, "tested earlier"
		}
		m.checks = append(m.checks, row)
	}
	return m.nextCheck()
}

// nextCheck starts the first pending row, if any. Rows run one at a time,
// so two agent CLIs never run at once.
func (m Model) nextCheck() (tea.Model, tea.Cmd) {
	i := slices.IndexFunc(m.checks, func(r checkRow) bool { return r.state == checkPending })
	if i < 0 {
		m.checking = false
		return m, nil
	}
	m.checking = true
	m.checks[i].state = checkRunning
	return m, m.runCheck(i)
}

// handleCheck records one row's result and starts the next.
func (m Model) handleCheck(msg checkMsg) (tea.Model, tea.Cmd) {
	if msg.run != m.checkRun || m.step != stepChecks {
		return m, nil // from an earlier visit to step 6
	}
	row := &m.checks[msg.index]
	if msg.err != nil {
		row.state, row.detail = checkFailed, msg.err.Error()
	} else {
		row.state, row.detail = checkOK, msg.detail
		if row.role >= 0 {
			m.validated[row.key] = true
			row.detail = "OK"
		} else {
			cli := cliByBinary(row.binary)
			m.logins = append(m.logins, cli.Name+": "+cli.SignIn)
			if msg.detail != "" {
				row.detail = msg.detail + " · " + cli.SignIn + " login"
			} else {
				row.detail = cli.SignIn + " login"
			}
		}
	}
	return m.nextCheck()
}

// cliByBinary returns the CLI whose executable is binary.
func cliByBinary(binary string) agents.CLI {
	for _, c := range cliOrder {
		if c.Binary == binary {
			return c
		}
	}
	return cliOrder[0]
}

// confirmChecks handles Enter on step 6: on to step 7 when every row
// passed; back to the first failed role's step when a test run failed;
// otherwise (a login or an API key) the checks run again.
func (m Model) confirmChecks() (tea.Model, tea.Cmd) {
	failed := false
	for _, r := range m.checks {
		if r.state != checkFailed {
			continue
		}
		failed = true
		if r.role >= 0 {
			return m.enter(roleStep(r.role)), nil
		}
	}
	if failed {
		return m.startChecks()
	}
	return m.next()
}

// startPrepare starts step 8's job.
func (m Model) startPrepare() (tea.Model, tea.Cmd) {
	m.checking = true
	m.prepDone = false
	m.prepErr = ""
	return m, m.prepare()
}

// handlePrep takes the result of step 8's job. Missing files are offered
// as starters, pre-selected to be created.
func (m Model) handlePrep(msg prepMsg) Model {
	m.checking = false
	if msg.err != nil {
		m.prepErr = msg.err.Error()
		return m
	}
	m.prep, m.prepDone = msg.result, true
	for _, f := range m.prep.files {
		if _, seen := m.create[f.name]; !seen && !f.exists {
			m.create[f.name] = true
		}
	}
	return m.setFocus(0)
}

// missingFiles are the agent files step 8 found missing.
func (m Model) missingFiles() []string {
	var out []string
	for _, f := range m.prep.files {
		if !f.exists {
			out = append(out, f.name)
		}
	}
	return out
}

// filesToCreate are the starter files to write in step 9.
func (m Model) filesToCreate() []struct{ name, template string } {
	var out []struct{ name, template string }
	for _, f := range agentFiles {
		if slices.Contains(m.missingFiles(), f.name) && m.create[f.name] {
			out = append(out, f)
		}
	}
	return out
}

// handleSaved ends the wizard once the settings are written.
func (m Model) handleSaved(msg savedMsg) (tea.Model, tea.Cmd) {
	m.checking = false
	if msg.err != nil {
		m.saveErr = msg.err.Error()
		return m, nil
	}
	m.saved, m.result = true, msg.config
	return m, tea.Quit
}

// handleMouse selects the chip under a left click, and focuses its field.
func (m Model) handleMouse(msg tea.MouseMsg) Model {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m
	}
	fs := m.fields()
	for i, f := range fs {
		x, y := m.fieldOrigin(i)
		if msg.Y != y || f.options == nil {
			continue
		}
		if chip, ok := ui.ChipAt(f.options, msg.X-x); ok {
			m = m.setFocus(i)
			return m.choose(f, chip)
		}
	}
	return m
}

// summaryLine is the one-line description of what step s chose, for the
// step list.
func (m Model) summaryLine(s int) string {
	switch s {
	case stepFolder:
		return tilde(m.dir, m.deps.Home)
	case stepRepo:
		return fmt.Sprintf("%s   (gh: %s)", m.repo.repo, m.repo.login)
	case stepDev, stepPM:
		return m.roles[s-stepDev].summary()
	case stepComp:
		if m.companionSame {
			return "same as PM"
		}
		return m.roles[roleComp].summary()
	case stepChecks:
		return strings.Join(append(slices.Clone(m.logins), "no API keys"), " · ")
	case stepAutonomy:
		return autonomyTitle(m.autonomy)
	case stepPrepare:
		return m.prepSummary()
	}
	return ""
}
