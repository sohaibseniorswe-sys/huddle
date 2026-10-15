// Package start is what Huddle shows before a milestone runs, and what
// happens between milestones (docs/v3/issues/06-start-modes.md):
//
//   - the start menu: plan new milestones, run an existing one, resume an
//     interrupted one, settings, quit (menu.go, view.go);
//   - plans: the <plan> block the companion writes in plan mode, parsed
//     here and created on GitHub by Huddle itself (plan.go, apply.go);
//   - the milestone queue: the control issues of agreed plans, run one
//     after another without the human (queue.go).
//
// The run screen (package app) shows plan mode and runs the queue; this
// package holds the parts that do not need the run screen, so they can be
// tested alone.
package start

import (
	"fmt"
	"strings"
)

// Plan is a set of milestones the human and the companion agreed, in the
// order they are to be built.
type Plan struct {
	Milestones []PlannedMilestone
}

// PlannedMilestone is one milestone of a plan, with its issues in order.
type PlannedMilestone struct {
	Title       string
	Description string // may be ""
	Issues      []PlannedIssue
}

// PlannedIssue is one issue of a planned milestone.
type PlannedIssue struct {
	Title string
	Body  string // the issue's text, acceptance criteria included
}

// IssueCount returns the number of issues in the plan, control issues not
// counted.
func (p Plan) IssueCount() int {
	n := 0
	for _, m := range p.Milestones {
		n += len(m.Issues)
	}
	return n
}

// Summary describes the plan in a few words: "2 milestones, 9 issues".
func (p Plan) Summary() string {
	return plural(len(p.Milestones), "milestone") + ", " + plural(p.IssueCount(), "issue")
}

// plural is "1 issue" or "9 issues".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// Text writes the whole plan out for the human to read (Show all).
func (p Plan) Text() string {
	var b strings.Builder
	for i, m := range p.Milestones {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s\n", m.Title)
		if m.Description != "" {
			fmt.Fprintf(&b, "%s\n", m.Description)
		}
		for j, is := range m.Issues {
			fmt.Fprintf(&b, "\n  %d. %s\n", j+1, is.Title)
			for _, line := range strings.Split(is.Body, "\n") {
				fmt.Fprintf(&b, "     %s\n", line)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// The keys of the plan format. Each starts a line, directly followed by a
// colon. See the companion's instructions (internal/companion) for how
// the format is explained to it.
const (
	keyMilestone   = "milestone"
	keyDescription = "description"
	keyIssue       = "issue"
	keyBody        = "body"
)

// ParsePlan reads the text between <plan> and </plan>:
//
//	milestone: Milestone 4 · Maps and exports
//	description: Maps on every page, and exports.
//	issue: Add the /export endpoint
//	body: GET /export returns a CSV of ...
//	Acceptance: ...
//	issue: ...
//	milestone: Milestone 5 · ...
//
// A line that starts with "milestone:", "description:", "issue:" or
// "body:" (no indentation) starts a field; every other line continues the
// field above it, so a description or a body may have many lines. Blank
// lines at the start and end of a field are dropped.
//
// It is a plain line-by-line parser on purpose: agents write a few
// "key: value" lines more reliably than YAML or JSON. Anything missing (a
// milestone without issues, an issue without a body, ...) is an error
// that names the milestone or issue, and then no plan is returned, so
// nothing is created.
func ParsePlan(text string) (Plan, error) {
	var p Plan
	// field is the description or body that continuation lines go into;
	// nil after a title line, which is one line only.
	var field *string
	for n, line := range strings.Split(text, "\n") {
		lineNo := n + 1
		key, value, isKey := splitKey(line)
		if !isKey {
			switch {
			case field != nil:
				*field += "\n" + line
			case strings.TrimSpace(line) == "":
				// a blank line between fields
			case len(p.Milestones) == 0:
				return Plan{}, fmt.Errorf("line %d: %q comes before the first \"milestone:\" line", lineNo, strings.TrimSpace(line))
			default:
				return Plan{}, fmt.Errorf("line %d: %q follows a title; a title is one line, and an issue's text starts with \"body:\"", lineNo, strings.TrimSpace(line))
			}
			continue
		}

		field = nil
		switch key {
		case keyMilestone:
			p.Milestones = append(p.Milestones, PlannedMilestone{Title: value})
		case keyDescription:
			m, err := lastMilestone(&p, key, lineNo)
			if err != nil {
				return Plan{}, err
			}
			if len(m.Issues) > 0 || m.Description != "" {
				return Plan{}, fmt.Errorf("line %d: milestone %q: \"description:\" must come once, right after \"milestone:\"", lineNo, m.Title)
			}
			m.Description = value
			field = &m.Description
		case keyIssue:
			m, err := lastMilestone(&p, key, lineNo)
			if err != nil {
				return Plan{}, err
			}
			m.Issues = append(m.Issues, PlannedIssue{Title: value})
		case keyBody:
			m, err := lastMilestone(&p, key, lineNo)
			if err != nil {
				return Plan{}, err
			}
			if len(m.Issues) == 0 {
				return Plan{}, fmt.Errorf("line %d: milestone %q: \"body:\" comes before its \"issue:\" line", lineNo, m.Title)
			}
			is := &m.Issues[len(m.Issues)-1]
			if is.Body != "" {
				return Plan{}, fmt.Errorf("line %d: issue %q has two \"body:\" lines", lineNo, is.Title)
			}
			// "body:" alone, with the text on the next lines, is fine: a
			// space marks the body as started, and tidy trims it.
			is.Body = value
			if is.Body == "" {
				is.Body = " "
			}
			field = &is.Body
		}
	}
	if err := p.tidy(); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// splitKey reports whether line starts a field, and splits it into the
// key and the value after the colon.
func splitKey(line string) (key, value string, ok bool) {
	k, v, found := strings.Cut(line, ":")
	if !found {
		return "", "", false
	}
	switch k {
	case keyMilestone, keyDescription, keyIssue, keyBody:
		return k, strings.TrimSpace(v), true
	}
	return "", "", false
}

// lastMilestone returns the milestone that a description, issue or body
// line belongs to: the last one so far.
func lastMilestone(p *Plan, key string, lineNo int) (*PlannedMilestone, error) {
	if len(p.Milestones) == 0 {
		return nil, fmt.Errorf("line %d: %q comes before the first \"milestone:\" line", lineNo, key+":")
	}
	return &p.Milestones[len(p.Milestones)-1], nil
}

// tidy trims every field and checks that nothing is missing.
func (p *Plan) tidy() error {
	if len(p.Milestones) == 0 {
		return fmt.Errorf("the plan has no \"milestone:\" line")
	}
	for i := range p.Milestones {
		m := &p.Milestones[i]
		m.Title = strings.TrimSpace(m.Title)
		m.Description = strings.TrimSpace(m.Description)
		if m.Title == "" {
			return fmt.Errorf("milestone %d has no title", i+1)
		}
		if len(m.Issues) == 0 {
			return fmt.Errorf("milestone %q has no issues", m.Title)
		}
		for j := range m.Issues {
			is := &m.Issues[j]
			is.Title = strings.TrimSpace(is.Title)
			is.Body = strings.TrimSpace(is.Body)
			if is.Title == "" {
				return fmt.Errorf("milestone %q: issue %d has no title", m.Title, j+1)
			}
			if is.Body == "" {
				return fmt.Errorf("milestone %q: issue %q has no body", m.Title, is.Title)
			}
		}
	}
	return nil
}
