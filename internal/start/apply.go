package start

import (
	"context"
	"fmt"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

// DefaultPrompt is the First Dev task of a control issue Huddle creates
// when the human gives no first message.
const DefaultPrompt = "Implement this milestone."

// firstTaskHeading opens the section of a control issue's body that holds
// the Dev's first task.
const firstTaskHeading = "## First Dev task"

// promptPrefix starts the line of that section that holds the prompt.
const promptPrefix = "Prompt: "

// ControlTitle is the title of milestone's control issue: "M4 Control".
func ControlTitle(milestone string) string { return milestone + " Control" }

// ControlBody is the body of a control issue Huddle creates. prompt is
// the Dev's first task; "" means DefaultPrompt. The marker lets Huddle
// find the issue even if its label is removed.
func ControlBody(milestone, prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		prompt = DefaultPrompt
	}
	return fmt.Sprintf("Control issue for %s. The Dev and PM agents talk here, and Huddle watches its state:* label.\n\n%s\n\n%s%s\n\n%s\n",
		milestone, firstTaskHeading, promptPrefix, strings.TrimSpace(prompt), gh.ControlMarker)
}

// WithPrompt returns body with its First Dev task replaced by prompt. A
// body without that section gets one at the end.
func WithPrompt(body, prompt string) string {
	section := firstTaskHeading + "\n\n" + promptPrefix + strings.TrimSpace(prompt)
	start := strings.Index(body, firstTaskHeading)
	if start < 0 {
		return strings.TrimRight(body, "\n") + "\n\n" + section + "\n"
	}
	// The section runs to the next heading or the marker, whichever comes
	// first, or to the end.
	end := len(body)
	rest := body[start+len(firstTaskHeading):]
	for _, stop := range []string{"\n#", "\n" + gh.ControlMarker} {
		if i := strings.Index(rest, stop); i >= 0 {
			end = min(end, start+len(firstTaskHeading)+i)
		}
	}
	return body[:start] + section + "\n" + body[end:]
}

// Created is one milestone of a plan as it now exists on GitHub.
type Created struct {
	Milestone string // its title
	Control   int    // its control issue's number
	Issues    []int  // its issues' numbers, in plan order
}

// Apply creates everything in plan on repo, in order: for each milestone,
// the milestone, then its issues, then its control issue (label
// "control", with the default First Dev task). report is told about every
// step as it is done, so the run screen can show it in Activity.
//
// Huddle does this itself, not the PM, because it is mechanical and must
// not be interpreted. If a step fails, Apply stops there and returns the
// milestones finished so far with the error; nothing is undone, and
// report has said what exists.
func Apply(ctx context.Context, client gh.Client, repo string, plan Plan, report func(string)) ([]Created, error) {
	var done []Created
	for _, m := range plan.Milestones {
		number, err := client.CreateMilestone(ctx, repo, m.Title, m.Description)
		if err != nil {
			return done, fmt.Errorf("create milestone %q: %w", m.Title, err)
		}
		report(fmt.Sprintf("Created milestone %q (#%d)", m.Title, number))
		c := Created{Milestone: m.Title}
		for _, is := range m.Issues {
			n, err := client.CreateIssue(ctx, repo, gh.NewIssue{Title: is.Title, Body: is.Body, Milestone: m.Title})
			if err != nil {
				return done, fmt.Errorf("create issue %q in %q: %w", is.Title, m.Title, err)
			}
			c.Issues = append(c.Issues, n)
			report(fmt.Sprintf("Created issue #%d %s", n, is.Title))
		}
		c.Control, err = CreateControl(ctx, client, repo, m.Title, "")
		if err != nil {
			return done, err
		}
		report(fmt.Sprintf("Created control issue #%d for %q", c.Control, m.Title))
		done = append(done, c)
	}
	return done, nil
}

// CreateControl creates milestone's control issue with prompt as the
// First Dev task ("" for the default) and returns its number.
func CreateControl(ctx context.Context, client gh.Client, repo, milestone, prompt string) (int, error) {
	n, err := client.CreateIssue(ctx, repo, gh.NewIssue{
		Title:     ControlTitle(milestone),
		Body:      ControlBody(milestone, prompt),
		Milestone: milestone,
		Labels:    []string{gh.ControlLabel},
	})
	if err != nil {
		return 0, fmt.Errorf("create the control issue of %q: %w", milestone, err)
	}
	return n, nil
}

// Prepare gets an existing milestone ready to run: it creates its control
// issue if control is 0, or else puts prompt into the existing one's First
// Dev task if prompt is not empty, and then sets state:dev-ready. It
// returns the control issue's number.
func Prepare(ctx context.Context, client gh.Client, repo, milestone string, control int, prompt string) (int, error) {
	if control == 0 {
		n, err := CreateControl(ctx, client, repo, milestone, prompt)
		if err != nil {
			return 0, err
		}
		control = n
	} else if strings.TrimSpace(prompt) != "" {
		if err := setPrompt(ctx, client, repo, control, prompt); err != nil {
			return 0, err
		}
	}
	c := gh.Control{Client: client, Repo: repo, Number: control}
	if err := c.SetState(ctx, gh.StateDevReady); err != nil {
		return 0, err
	}
	return control, nil
}

// setPrompt replaces the First Dev task in control issue number's body.
func setPrompt(ctx context.Context, client gh.Client, repo string, number int, prompt string) error {
	open, err := client.ListIssues(ctx, repo, "open")
	if err != nil {
		return fmt.Errorf("read control issue #%d: %w", number, err)
	}
	for _, is := range open {
		if is.Number == number {
			if err := client.EditBody(ctx, repo, number, WithPrompt(is.Body, prompt)); err != nil {
				return fmt.Errorf("write the first Dev task into #%d: %w", number, err)
			}
			return nil
		}
	}
	return fmt.Errorf("control issue #%d is not open", number)
}
