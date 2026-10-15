package companion

import (
	"regexp"
	"strings"
)

// Reply is a companion reply split into what the human reads and the card
// it proposes, if any.
type Reply struct {
	Text     string // the reply without its <decision>, <reason> and <action> tags
	Decision string // the proposed decision; "" if none
	Reason   string // the decision's one-line reason; "" if none was given
	Action   string // a proposed Huddle action: "pause", "resume", "status" or "log HH:MM"; "" if none
	// Plan is the text inside a <plan> block, line breaks kept, for
	// start.ParsePlan; "" if none.
	Plan string
}

// The tags Parse looks for. (?s) lets "." match line breaks too, so a tag
// may span lines; ".*?" stops at the first closing tag.
var (
	decisionRe = regexp.MustCompile(`(?s)<decision>(.*?)</decision>`)
	reasonRe   = regexp.MustCompile(`(?s)<reason>(.*?)</reason>`)
	actionRe   = regexp.MustCompile(`(?s)<action>(.*?)</action>`)
	planRe     = regexp.MustCompile(`(?s)<plan>(.*?)</plan>`)
	// actionNameRe is the list of actions the instructions allow.
	actionNameRe = regexp.MustCompile(`^(pause|resume|status|log [0-2]?[0-9]:[0-5][0-9])$`)
	// blankLinesRe matches the runs of empty lines that removed tags leave.
	blankLinesRe = regexp.MustCompile(`\n{3,}`)
)

// Parse splits a companion reply. Tags inside code fences (``` blocks) are
// examples, not proposals: they are left in the text and ignored. If the
// reply has several decisions, the last one wins, and the same for reasons
// and actions and plans. An action that is not one of the allowed names is
// ignored. Only one card is shown at a time, so a plan beats a decision
// and an action, and a decision beats an action.
func Parse(reply string) Reply {
	var r Reply
	var text strings.Builder
	for i, part := range splitFences(reply) {
		if i%2 == 1 { // inside a fence: keep it as it is
			text.WriteString(part)
			continue
		}
		if s, ok := last(decisionRe, part); ok {
			r.Decision = s
		}
		if s, ok := last(reasonRe, part); ok {
			r.Reason = s
		}
		if s, ok := last(actionRe, part); ok {
			r.Action = s
		}
		if all := planRe.FindAllStringSubmatch(part, -1); len(all) > 0 {
			// Unlike the others, a plan keeps its lines: they are its format.
			r.Plan = strings.TrimSpace(all[len(all)-1][1])
		}
		for _, re := range []*regexp.Regexp{decisionRe, reasonRe, actionRe, planRe} {
			part = re.ReplaceAllString(part, "")
		}
		text.WriteString(part)
	}
	if r.Plan != "" {
		r.Decision = ""
	}
	if !actionNameRe.MatchString(r.Action) || r.Decision != "" || r.Plan != "" {
		r.Action = ""
	}
	if r.Decision == "" {
		r.Reason = ""
	}
	r.Text = strings.TrimSpace(blankLinesRe.ReplaceAllString(text.String(), "\n\n"))
	return r
}

// last returns the trimmed content of re's last match in s.
func last(re *regexp.Regexp, s string) (string, bool) {
	all := re.FindAllStringSubmatch(s, -1)
	if len(all) == 0 {
		return "", false
	}
	return strings.Join(strings.Fields(all[len(all)-1][1]), " "), true
}

// splitFences splits s at code fence lines (lines starting with ```). The
// parts at even indexes are outside fences, those at odd indexes inside,
// fence lines included. An unclosed fence runs to the end.
func splitFences(s string) []string {
	var parts []string
	var cur strings.Builder
	inside := false
	for _, line := range strings.SplitAfter(s, "\n") {
		isFence := strings.HasPrefix(strings.TrimSpace(line), "```")
		if isFence && !inside {
			parts = append(parts, cur.String())
			cur.Reset()
			inside = true
			cur.WriteString(line)
			continue
		}
		cur.WriteString(line)
		if isFence && inside {
			parts = append(parts, cur.String())
			cur.Reset()
			inside = false
		}
	}
	return append(parts, cur.String())
}
