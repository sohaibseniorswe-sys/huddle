package agents

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Claude Code names IANA zones; don't depend on the host's zoneinfo
)

// UsageLimit is what a usage-limit check found in an agent's output
// (MVP_PLAN.md section 8.6). At most one of Hit and Escalate is set.
type UsageLimit struct {
	// Hit means a usage limit to wait out: until ResetAt, or with 15-minute
	// retries if ResetAt is zero.
	Hit     bool
	ResetAt time.Time // zero if the output gave no reset time or it could not be parsed
	// Escalate means an account or credit error that only the human can fix:
	// an account-setup error, or a spend-cap / out-of-credits message without
	// a reset time. Huddle sets state:human-required with Message as the reason.
	Escalate bool
	Message  string // the CLI's message line, for Hit and Escalate
}

// Usage checks a run's output for a usage limit or an account or credit
// error, using cli's messages (CodexUsageLimit or ClaudeUsageLimit).
func Usage(cli CLI, output string, now time.Time) UsageLimit {
	if cli.ID == ClaudeID {
		return ClaudeUsageLimit(output, now)
	}
	return CodexUsageLimit(output, now)
}

// pick chooses between the findings in one run's output: a message with a
// reset time will clear by itself, so it wins; then an escalation; then a
// plain usage limit.
func pick(found []UsageLimit) UsageLimit {
	for _, rank := range []func(UsageLimit) bool{
		func(u UsageLimit) bool { return u.Hit && !u.ResetAt.IsZero() },
		func(u UsageLimit) bool { return u.Escalate },
		func(u UsageLimit) bool { return u.Hit },
	} {
		for _, u := range found {
			if rank(u) {
				return u
			}
		}
	}
	return UsageLimit{}
}

// The messages below were taken from the installed CLIs (codex-cli 0.160.0,
// Claude Code 2.1.296); README.md lists each one and how it is handled. Only
// these specific messages count, at the start of a line (optionally after an
// "ERROR:"-style prefix), so other output that mentions a limit does not.
// Both CLIs use a typographic apostrophe in places; a plain one is accepted
// too.

// linePrefix allows an "ERROR: " / "Error: " / "API Error: " prefix before a
// message at the start of a line.
const linePrefix = `(?m)^(?:.*?\b(?:ERROR|API Error|Error|error):\s*)?`

var errorPrefixRe = regexp.MustCompile(`^.*?\b(?:ERROR|API Error|Error|error):\s*`)

// message returns a matched line as the CLI's message, without the
// "ERROR:"-style prefix and surrounding space.
func message(line string) string {
	return strings.TrimSpace(errorPrefixRe.ReplaceAllString(strings.TrimSpace(line), ""))
}

// codexLimitRe matches codex's usage limits, which wait:
//   - UsageLimitReachedError, "You’ve hit your usage limit." with plan-specific
//     advice, or "You’ve hit your usage limit for <model>." for a model's own
//     limit, each ending in "Try again at <time>." / "or try again at <time>."
//     or "Try again later.";
//   - the TUI's "Usage limit reached. You’ve reached your usage limit. ...".
var codexLimitRe = regexp.MustCompile(linePrefix + `(?:` +
	`You[’']ve hit your usage limit\b` +
	`|You[’']ve reached your usage limit\b` +
	`|Usage limit reached\.` +
	`).*$`)

// codexEscalateRe matches codex's account and credit errors, none of which
// gives a reset time:
//   - the workspace spend-control errors: "Your workspace is out of credits.
//     ..." and "You hit your spend cap set ...";
//   - the TUI's "You’ve reached your workspace credit limit" and "You’re out
//     of credits.";
//   - UsageNotIncluded: "To use Codex with your ChatGPT plan, upgrade to Plus: ...";
//   - QuotaExceeded: "Quota exceeded. Check your plan and billing details.",
//     which only API-key billing produces. Huddle never uses an API key, so
//     seeing it means something is wrong with the login.
var codexEscalateRe = regexp.MustCompile(linePrefix + `(?:` +
	`Your workspace is out of credits\.` +
	`|You[’']re out of credits\.` +
	`|You hit your spend cap\b` +
	`|You[’']ve reached your workspace credit limit\b` +
	`|To use Codex with your ChatGPT plan, upgrade to\b` +
	`|Quota exceeded\. Check your plan and billing details\.` +
	`).*$`)

// codexRetryRe extracts the reset time from " Try again at <time>." or
// ", or try again at <time>.".
var codexRetryRe = regexp.MustCompile(`[Tt]ry again at ([^\n]+?)\.(?:\s|$)`)

// codexDateRe is codex's reset time on another day, in local time:
// "Oct 13th, 2026 3:45 PM".
var codexDateRe = regexp.MustCompile(`^([A-Z][a-z]{2}) (\d{1,2})(?:st|nd|rd|th), (\d{4}) (\d{1,2}:\d{2} [AP]M)$`)

// CodexUsageLimit checks codex output for a usage limit, with its reset time
// if it gives one, or an account or credit error to escalate. codex prints
// the time in the machine's local time zone, as "3:45 PM" for today or
// "Oct 13th, 2026 3:45 PM" otherwise; now supplies the date and location.
func CodexUsageLimit(output string, now time.Time) UsageLimit {
	var found []UsageLimit
	for _, line := range codexEscalateRe.FindAllString(output, -1) {
		found = append(found, UsageLimit{Escalate: true, Message: message(line)})
	}
	for _, line := range codexLimitRe.FindAllString(output, -1) {
		found = append(found, UsageLimit{Hit: true, Message: message(line), ResetAt: codexReset(line, now)})
	}
	return pick(found)
}

func codexReset(line string, now time.Time) time.Time {
	m := codexRetryRe.FindStringSubmatch(line)
	if m == nil {
		return time.Time{}
	}
	s, loc := strings.TrimSpace(m[1]), now.Location()
	if t, err := time.ParseInLocation("3:04 PM", s, loc); err == nil {
		return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	}
	if d := codexDateRe.FindStringSubmatch(s); d != nil {
		if t, err := time.ParseInLocation("Jan 2 2006 3:04 PM", d[1]+" "+d[2]+" "+d[3]+" "+d[4], loc); err == nil {
			return t
		}
	}
	return time.Time{}
}

// claudeLimitRe matches Claude Code's limit messages. Its groups tell them
// apart:
//
//  1. "You've hit your <limit>[ · ...][ · resets <time>][ · progress saved]",
//     with <limit> in group 2: a plan limit ("session limit" (5-hour),
//     "weekly limit", "Opus limit", "Sonnet limit", "Fable limit", "usage
//     limit", "limit"), or a spend limit ("monthly spend limit", "org's
//     monthly spend limit", "individual spend limit", "individual usage
//     limit", "org's monthly usage limit", "channel's monthly spend limit",
//     "channel's monthly usage limit", "team's shared budget"), which gives
//     the reset as " · your <limit> resets <time>" if at all;
//  3. credit messages: "You've reached your Fable limit.", "You're out of
//     usage credits[ · resets <time>]", "You're out of extra usage", "Your
//     org is out of usage · ...";
//  4. "Usage limit reached[ · limit resets <time>]", a plan limit.
//
// Group 5 is the rest of the line. Fast mode's "You've hit your fast limit"
// is a cooldown, not an end of the run, and is skipped.
var claudeLimitRe = regexp.MustCompile(linePrefix + `(?:` +
	`(You[’']ve hit your ((?:[A-Za-z']+ )*?)(?:limit|budget)\b)` +
	`|(You[’']ve reached your (?:[A-Za-z']+ )*?limit\b|You[’']re out of (?:usage credits|extra usage)\b|Your org is out of usage\b)` +
	`|(Usage limit reached\b)` +
	`)([^\n]*)$`)

// claudePlanLimits are the <limit> names of plan limits, which reset by
// themselves; any other "You've hit your ..." is a spend limit.
var claudePlanLimits = []string{"", "session", "weekly", "Opus", "Sonnet", "Fable", "usage"}

// claudeAccountRe matches Claude Code's account-setup errors, which always
// escalate: "Your seat type doesn't include usage credits" (or "usage",
// "extra usage"), "This service is disabled for your org", "Your usage
// allocation has been disabled by your admin[ · ...]", "Your group's usage
// limit is set to $0[ · ...]" and "<model> requires usage credits.".
var claudeAccountRe = regexp.MustCompile(linePrefix + `(?:` +
	`Your seat type doesn[’']t include\b` +
	`|This service is disabled for your org\b` +
	`|Your usage allocation has been disabled by your admin\b` +
	`|Your group[’']s usage limit is set to \$0` +
	`|(?:Fable|Haiku|Mythos|Opus|Sonnet)(?: [^·\n]{1,40})? requires usage credits\.` +
	`)[^\n]*$`)

// claudeResetRe extracts the reset time from " · resets <time>",
// " · your <limit> resets <time>" or " · limit resets <time>".
var claudeResetRe = regexp.MustCompile(`· (?:your [^·\n]+? |limit )?resets ([^·\n]+)`)

// claudeLegacyRe is the older "Claude AI usage limit reached|<unix seconds>".
var claudeLegacyRe = regexp.MustCompile(linePrefix + `Claude AI usage limit reached(?:\|(\d+))?`)

// claudeTimeRe is Claude Code's reset time: "3pm", "3:30pm", "Oct 13, 3pm"
// or "Oct 13, 2027, 3pm", followed by the IANA time zone in parentheses.
var claudeTimeRe = regexp.MustCompile(`^(?:([A-Z][a-z]{2}) (\d{1,2}), (?:(\d{4}), )?)?(\d{1,2})(?::(\d{2}))?([ap]m)(?: \(([^)]+)\))?$`)

// ClaudeUsageLimit checks Claude Code output for a usage limit, with its
// reset time if it gives one, or an account or credit error to escalate. It
// checks the JSON envelope's result field and the raw output (stderr). Times
// within 24 hours are given without a date ("3pm"), so a time already past
// today means tomorrow. now supplies the date, and the location when the
// message names no time zone.
func ClaudeUsageLimit(output string, now time.Time) UsageLimit {
	texts := []string{output}
	if env, err := ParseClaude(output); err == nil {
		texts = append(texts, env.Result)
	}
	var found []UsageLimit
	for _, text := range texts {
		for _, m := range claudeLegacyRe.FindAllStringSubmatch(text, -1) {
			u := UsageLimit{Hit: true, Message: message(m[0])}
			if secs, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				u.ResetAt = time.Unix(secs, 0).In(now.Location())
			}
			found = append(found, u)
		}
		for _, line := range claudeAccountRe.FindAllString(text, -1) {
			found = append(found, UsageLimit{Escalate: true, Message: message(line)})
		}
		for _, m := range claudeLimitRe.FindAllStringSubmatch(text, -1) {
			name := strings.TrimSpace(m[2])
			if m[1] != "" && name == "fast" {
				continue
			}
			var reset time.Time
			if r := claudeResetRe.FindStringSubmatch(m[5]); r != nil {
				reset = parseClaudeReset(strings.TrimSpace(r[1]), now)
			}
			plan := m[4] != "" || (m[1] != "" && slices.Contains(claudePlanLimits, name))
			u := UsageLimit{Hit: true, ResetAt: reset, Message: message(m[0])}
			if !plan && reset.IsZero() {
				// A spend or credit limit with no reset: only money fixes it.
				u = UsageLimit{Escalate: true, Message: u.Message}
			}
			found = append(found, u)
		}
	}
	return pick(found)
}

func parseClaudeReset(s string, now time.Time) time.Time {
	m := claudeTimeRe.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}
	}
	loc := now.Location()
	if m[7] != "" {
		l, err := time.LoadLocation(m[7])
		if err != nil {
			return time.Time{}
		}
		loc = l
	}
	hour, _ := strconv.Atoi(m[4])
	if hour < 1 || hour > 12 {
		return time.Time{}
	}
	hour %= 12
	if m[6] == "pm" {
		hour += 12
	}
	minute := 0
	if m[5] != "" {
		minute, _ = strconv.Atoi(m[5])
	}
	local := now.In(loc)
	if m[1] == "" {
		t := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
		if t.Before(now) {
			t = t.AddDate(0, 0, 1)
		}
		return t
	}
	month, err := time.Parse("Jan", m[1])
	if err != nil {
		return time.Time{}
	}
	day, _ := strconv.Atoi(m[2])
	year := local.Year()
	if m[3] != "" {
		year, _ = strconv.Atoi(m[3])
	}
	return time.Date(year, month.Month(), day, hour, minute, 0, 0, loc)
}
