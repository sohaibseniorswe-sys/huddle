package runner

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Claude Code names IANA zones; don't depend on the host's zoneinfo
)

// UsageLimit is what a usage-limit check found in an agent's output.
type UsageLimit struct {
	Hit     bool
	ResetAt time.Time // zero if the output gave no reset time or it could not be parsed
}

// The usage-limit messages below were taken from the installed CLIs
// (codex-cli 0.160.0, Claude Code 2.1.296); README.md lists each one. Only
// these specific messages count, at the start of a line (optionally after an
// "ERROR:"-style prefix), so other output that mentions a limit does not.
// Both CLIs use a typographic apostrophe in places; a plain one is accepted
// too.

// linePrefix allows an "ERROR: " / "Error: " / "API Error: " prefix before a
// message at the start of a line.
const linePrefix = `(?m)^(?:.*?\b(?:ERROR|API Error|Error|error):\s*)?`

// codexLimitRe matches every codex usage-limit message:
//   - UsageLimitReachedError, "You’ve hit your usage limit." with plan-specific
//     advice, or "You’ve hit your usage limit for <model>." for a model's own
//     limit, each ending in "Try again at <time>." / "or try again at <time>."
//     or "Try again later.";
//   - the workspace spend-control errors: "Your workspace is out of credits.
//     ..." and "You hit your spend cap set ...";
//   - the TUI's "Usage limit reached. You’ve reached your usage limit. ...",
//     "You’ve reached your workspace credit limit" and "You’re out of credits.".
var codexLimitRe = regexp.MustCompile(linePrefix + `(?:` +
	`You[’']ve hit your usage limit\b` +
	`|You[’']ve reached your (?:usage|workspace credit) limit\b` +
	`|Usage limit reached\.` +
	`|Your workspace is out of credits\.` +
	`|You[’']re out of credits\.` +
	`|You hit your spend cap\b` +
	`).*$`)

// codexRetryRe extracts the reset time from " Try again at <time>." or
// ", or try again at <time>.".
var codexRetryRe = regexp.MustCompile(`[Tt]ry again at ([^\n]+?)\.(?:\s|$)`)

// codexDateRe is codex's reset time on another day, in local time:
// "Oct 13th, 2026 3:45 PM".
var codexDateRe = regexp.MustCompile(`^([A-Z][a-z]{2}) (\d{1,2})(?:st|nd|rd|th), (\d{4}) (\d{1,2}:\d{2} [AP]M)$`)

// CodexUsageLimit reports whether codex output shows a usage limit, and the
// reset time if it gives one. codex prints the time in the machine's local
// time zone, as "3:45 PM" for today or "Oct 13th, 2026 3:45 PM" otherwise;
// now supplies the date and location.
func CodexUsageLimit(output string, now time.Time) UsageLimit {
	line := codexLimitRe.FindString(output)
	if line == "" {
		return UsageLimit{}
	}
	u := UsageLimit{Hit: true}
	m := codexRetryRe.FindStringSubmatch(line)
	if m == nil {
		return u
	}
	s, loc := strings.TrimSpace(m[1]), now.Location()
	if t, err := time.ParseInLocation("3:04 PM", s, loc); err == nil {
		u.ResetAt = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	} else if d := codexDateRe.FindStringSubmatch(s); d != nil {
		if t, err := time.ParseInLocation("Jan 2 2006 3:04 PM", d[1]+" "+d[2]+" "+d[3]+" "+d[4], loc); err == nil {
			u.ResetAt = t
		}
	}
	return u
}

// claudeLimitRe matches every Claude Code usage-limit message:
//   - "You've hit your <limit>[ · ...][ · resets <time>][ · progress saved]"
//     where <limit> is "session limit" (5-hour), "weekly limit", "Opus limit",
//     "Sonnet limit", "Fable limit", "usage limit", "limit", or a spend
//     variant ("monthly spend limit", "org's monthly spend limit", "individual
//     spend limit", "individual usage limit", "org's monthly usage limit",
//     "channel's monthly spend limit", "channel's monthly usage limit",
//     "team's shared budget"); spend variants give the reset as
//     " · your <limit> resets <time>";
//   - "You've reached your Fable limit.";
//   - "You're out of usage credits[ · resets <time>]", "You're out of extra
//     usage" and "Your org is out of usage · ...";
//   - "Usage limit reached[ · limit resets <time>]".
//
// Fast mode's "You've hit your fast limit" is a cooldown, not an end of the
// run, and is excluded in ClaudeUsageLimit.
var claudeLimitRe = regexp.MustCompile(linePrefix + `(?:` +
	`You[’']ve hit your ((?:[A-Za-z']+ )*?)(?:limit|budget)\b` +
	`|You[’']ve reached your (?:[A-Za-z']+ )*?limit\b` +
	`|You[’']re out of (?:usage credits|extra usage)\b` +
	`|Your org is out of usage\b` +
	`|Usage limit reached\b` +
	`)([^\n]*)$`)

// claudeResetRe extracts the reset time from " · resets <time>",
// " · your <limit> resets <time>" or " · limit resets <time>".
var claudeResetRe = regexp.MustCompile(`· (?:your [^·\n]+? |limit )?resets ([^·\n]+)`)

// claudeLegacyRe is the older "Claude AI usage limit reached|<unix seconds>".
var claudeLegacyRe = regexp.MustCompile(linePrefix + `Claude AI usage limit reached(?:\|(\d+))?`)

// claudeTimeRe is Claude Code's reset time: "3pm", "3:30pm", "Oct 13, 3pm"
// or "Oct 13, 2027, 3pm", followed by the IANA time zone in parentheses.
var claudeTimeRe = regexp.MustCompile(`^(?:([A-Z][a-z]{2}) (\d{1,2}), (?:(\d{4}), )?)?(\d{1,2})(?::(\d{2}))?([ap]m)(?: \(([^)]+)\))?$`)

// ClaudeUsageLimit reports whether Claude Code output shows a usage limit,
// and the reset time if it gives one. It checks the JSON envelope's result
// field and the raw output (stderr). Times within 24 hours are given without
// a date ("3pm"), so a time already past today means tomorrow. now supplies
// the date, and the location when the message names no time zone.
func ClaudeUsageLimit(output string, now time.Time) UsageLimit {
	texts := []string{output}
	if env, err := ParseClaude(output); err == nil {
		texts = append(texts, env.Result)
	}
	for _, text := range texts {
		if m := claudeLegacyRe.FindStringSubmatch(text); m != nil {
			u := UsageLimit{Hit: true}
			if secs, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				u.ResetAt = time.Unix(secs, 0).In(now.Location())
			}
			return u
		}
		for _, m := range claudeLimitRe.FindAllStringSubmatch(text, -1) {
			if strings.TrimSpace(m[1]) == "fast" {
				continue
			}
			u := UsageLimit{Hit: true}
			if r := claudeResetRe.FindStringSubmatch(m[2]); r != nil {
				u.ResetAt = parseClaudeReset(strings.TrimSpace(r[1]), now)
			}
			return u
		}
	}
	return UsageLimit{}
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
