package agents

import (
	"testing"
	"time"
)

type kind int

const (
	none     kind = iota // not a usage limit
	wait                 // Hit: wait for the reset, or retry every 15 minutes
	escalate             // Escalate: set state:human-required
)

func checkUsage(t *testing.T, got UsageLimit, k kind, resetAt time.Time, msg string) {
	t.Helper()
	if got.Hit != (k == wait) || got.Escalate != (k == escalate) || !got.ResetAt.Equal(resetAt) {
		t.Errorf("got %+v, want kind %d, reset %v", got, k, resetAt)
	}
	if k != none && got.Message == "" {
		t.Error("Message is empty")
	}
	if msg != "" && got.Message != msg {
		t.Errorf("Message = %q\nwant      %q", got.Message, msg)
	}
}

func TestCodexUsageLimit(t *testing.T) {
	loc := time.FixedZone("UTC+1", 3600)
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, loc)
	header := "OpenAI Codex v0.160.0\n--------\nsession id: 01a132be-f0bc-7293-89cc-72f23dcb576c\n--------\nuser\nIt's your turn.\n\n"
	tests := []struct {
		name    string
		output  string
		kind    kind
		resetAt time.Time
		msg     string // the expected Message; checked if set
	}{
		{"limit, reset later today",
			header + "ERROR: You’ve hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at 3:45 PM.\n",
			wait, time.Date(2026, 10, 12, 15, 45, 0, 0, loc),
			"You’ve hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at 3:45 PM."},
		{"limit, reset on another day",
			header + "ERROR: You’ve hit your usage limit. Try again at Oct 13th, 2026 9:05 AM.\n",
			wait, time.Date(2026, 10, 13, 9, 5, 0, 0, loc), ""},
		{"limit, plus plan wording, other day",
			"You’ve hit your usage limit. Upgrade to Plus to continue using Codex (https://chatgpt.com/explore/plus), or try again at Nov 1st, 2026 12:00 AM.\n",
			wait, time.Date(2026, 11, 1, 0, 0, 0, 0, loc), ""},
		{"limit, ASCII apostrophe", "ERROR: You've hit your usage limit. Try again at 11:30 AM.\n",
			wait, time.Date(2026, 10, 12, 11, 30, 0, 0, loc), ""},
		{"model-specific limit", header + "ERROR: You’ve hit your usage limit for GPT-6 Sol. Switch to another model now, or try again at 4:10 PM.\n",
			wait, time.Date(2026, 10, 12, 16, 10, 0, 0, loc), ""},
		{"model-specific limit, other day", "ERROR: You’ve hit your usage limit for GPT-6 Luna. Switch to another model now, or try again at Oct 19th, 2026 8:00 AM.\n",
			wait, time.Date(2026, 10, 19, 8, 0, 0, 0, loc), ""},
		{"pro plan wording", "ERROR: You’ve hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Oct 21st, 2026 1:00 PM.\n",
			wait, time.Date(2026, 10, 21, 13, 0, 0, 0, loc), ""},
		{"team plan wording", "ERROR: You’ve hit your usage limit. To get more access now, send a request to your admin or try again at Oct 22nd, 2026 2:00 PM.\n",
			wait, time.Date(2026, 10, 22, 14, 0, 0, 0, loc), ""},
		{"model-specific limit, no reset time", "ERROR: You’ve hit your usage limit for GPT-6 Sol. Switch to another model now, or try again later.\n", wait, time.Time{}, ""},
		{"workspace out of credits", "ERROR: Your workspace is out of credits. Add credits to continue.\n", escalate, time.Time{}, "Your workspace is out of credits. Add credits to continue."},
		{"workspace out of credits, member", "ERROR: Your workspace is out of credits. Ask your workspace owner to refill in order to continue.\n", escalate, time.Time{}, "Your workspace is out of credits. Ask your workspace owner to refill in order to continue."},
		{"spend cap", "ERROR: You hit your spend cap set in your workspace. Increase your spend cap to continue.\n", escalate, time.Time{}, "You hit your spend cap set in your workspace. Increase your spend cap to continue."},
		{"spend cap set by owner", "ERROR: You hit your spend cap set by the owner of your workspace. Ask an owner to increase your spend cap to continue.\n", escalate, time.Time{}, "You hit your spend cap set by the owner of your workspace. Ask an owner to increase your spend cap to continue."},
		{"TUI usage limit reached", "Usage limit reached. You’ve reached your usage limit. Increase your limits to continue using codex.\n", wait, time.Time{}, ""},
		{"TUI reached usage limit", "You've reached your usage limit. Increase your limits to continue using codex.\n", wait, time.Time{}, ""},
		{"TUI workspace credit limit", "You’ve reached your workspace credit limit\n", escalate, time.Time{}, "You’ve reached your workspace credit limit"},
		{"TUI out of credits", "You’re out of credits. Your workspace is out of credits. Add credits to continue using Codex.\n", escalate, time.Time{}, "You’re out of credits. Your workspace is out of credits. Add credits to continue using Codex."},
		{"limit, no reset time", header + "ERROR: You’ve hit your usage limit. Try again later.\n", wait, time.Time{}, ""},
		{"limit, admin wording, no reset time", "ERROR: You’ve hit your usage limit. To get more access now, send a request to your admin.\n", wait, time.Time{}, ""},
		{"limit, unexpected time format", "ERROR: You’ve hit your usage limit. Try again at 2026-10-13T09:05:00Z.\n", wait, time.Time{}, ""},
		{"normal output", readTestdata(t, "codex-fresh.out"), none, time.Time{}, ""},
		{"limit mentioned in another context",
			header + "exec\ngo test ./...\n--- FAIL: TestRateLimiter (0.00s)\n    limiter_test.go:12: usage limit reached after 3 calls, want 5\ncodex\nThe rate limit test fails; you've hit your usage limit check too early.\n",
			none, time.Time{}, ""},
		{"API quota exceeded (API-key billing)", "ERROR: Quota exceeded. Check your plan and billing details.\n",
			escalate, time.Time{}, "Quota exceeded. Check your plan and billing details."},
		{"plan without codex", "ERROR: To use Codex with your ChatGPT plan, upgrade to Plus: https://chatgpt.com/explore/plus.\n",
			escalate, time.Time{}, "To use Codex with your ChatGPT plan, upgrade to Plus: https://chatgpt.com/explore/plus."},
		{"limit with a reset time beats an escalation", "ERROR: Your workspace is out of credits. Add credits to continue.\nERROR: You’ve hit your usage limit. Try again at 3:45 PM.\n",
			wait, time.Date(2026, 10, 12, 15, 45, 0, 0, loc), "You’ve hit your usage limit. Try again at 3:45 PM."},
		{"escalation beats a limit without a reset time", "ERROR: You’ve hit your usage limit. Try again later.\nERROR: You hit your spend cap set in your workspace. Increase your spend cap to continue.\n",
			escalate, time.Time{}, "You hit your spend cap set in your workspace. Increase your spend cap to continue."},
		{"transient rate limit retries", "ERROR: exceeded retry limit, last status: 429 Too Many Requests\n", none, time.Time{}, ""},
		{"quoted mid-line", "codex\nThe PM said “You’ve hit your usage limit” is handled.\n", none, time.Time{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkUsage(t, CodexUsageLimit(tt.output, now), tt.kind, tt.resetAt, tt.msg)
		})
	}
}

func TestClaudeUsageLimit(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, london) // BST, UTC+1
	envelope := func(result string) string {
		return `{"type":"result","subtype":"success","is_error":true,"api_error_status":429,"session_id":"a1c710e8-9bf5-4507-af87-2d493feb051c","result":"` + result + `"}` + "\n"
	}
	tests := []struct {
		name    string
		output  string
		kind    kind
		resetAt time.Time
		msg     string // the expected Message; checked if set
	}{
		{"session limit, reset later today", envelope("You've hit your session limit · resets 3pm (Europe/London)"),
			wait, time.Date(2026, 10, 12, 15, 0, 0, 0, london), "You've hit your session limit · resets 3pm (Europe/London)"},
		{"limit, reset with minutes, other zone", envelope("You've hit your limit · resets 3:30pm (America/New_York) · progress saved"),
			wait, time.Date(2026, 10, 12, 19, 30, 0, 0, time.UTC), ""},
		{"time already past today means tomorrow", envelope("You've hit your session limit · resets 9am (Europe/London)"),
			wait, time.Date(2026, 10, 13, 9, 0, 0, 0, london), ""},
		{"weekly limit, date", envelope("You've hit your weekly limit · resets Oct 15, 10am (Europe/London)"),
			wait, time.Date(2026, 10, 15, 10, 0, 0, 0, london), ""},
		{"limit, date with year", envelope("You've hit your Opus limit · resets Jan 2, 2027, 12am (Europe/London)"),
			wait, time.Date(2027, 1, 2, 0, 0, 0, 0, london), ""},
		{"envelope escapes the middle dot", envelope(`You've hit your usage limit · resets 3pm (Europe/London)`),
			wait, time.Date(2026, 10, 12, 15, 0, 0, 0, london), ""},
		{"on stderr, not in an envelope", "Error: You've hit your session limit · resets 3pm (Europe/London)\n",
			wait, time.Date(2026, 10, 12, 15, 0, 0, 0, london), ""},
		{"legacy message with unix time", envelope("Claude AI usage limit reached|1792242000"),
			wait, time.Unix(1792242000, 0), ""},
		{"weekly limit, reset within a day", envelope("You've hit your weekly limit · resets 11pm (Europe/London) · progress saved"),
			wait, time.Date(2026, 10, 12, 23, 0, 0, 0, london), ""},
		{"Opus limit", envelope("You've hit your Opus limit · resets Oct 16, 9:30am (Europe/London)"),
			wait, time.Date(2026, 10, 16, 9, 30, 0, 0, london), ""},
		{"Sonnet limit", envelope("You've hit your Sonnet limit · resets Oct 14, 6pm (Europe/London)"),
			wait, time.Date(2026, 10, 14, 18, 0, 0, 0, london), ""},
		{"Fable limit", envelope("You've hit your Fable limit · resets Oct 17, 1pm (Europe/London)"),
			wait, time.Date(2026, 10, 17, 13, 0, 0, 0, london), ""},
		{"usage limit", envelope("You've hit your usage limit · resets 4pm (Europe/London)"),
			wait, time.Date(2026, 10, 12, 16, 0, 0, 0, london), ""},
		{"usage limit, contact admin", envelope("You've hit your usage limit · contact your admin to increase it"), wait, time.Time{}, ""},
		{"monthly spend limit with the plan limit's reset", envelope("You've hit your monthly spend limit · raise it at https://claude.ai/settings/usage · your weekly limit resets Oct 15, 10am (Europe/London)"),
			wait, time.Date(2026, 10, 15, 10, 0, 0, 0, london), ""},
		{"org's monthly spend limit", envelope("You've hit your org's monthly spend limit · ask your admin to raise it at https://claude.ai/admin-settings/usage · your session limit resets 2pm (Europe/London)"),
			wait, time.Date(2026, 10, 12, 14, 0, 0, 0, london), ""},
		{"individual spend limit", envelope("You've hit your individual spend limit · ask your admin for a higher limit"), escalate, time.Time{}, "You've hit your individual spend limit · ask your admin for a higher limit"},
		{"individual usage limit", envelope("You've hit your individual usage limit · resets Nov 1, 12am (Europe/London)"),
			wait, time.Date(2026, 11, 1, 0, 0, 0, 0, london), ""},
		{"org's monthly usage limit", envelope("You've hit your org's monthly usage limit · resets Nov 1, 12am (Europe/London)"),
			wait, time.Date(2026, 11, 1, 0, 0, 0, 0, london), ""},
		{"channel's monthly spend limit", envelope("You've hit your channel's monthly spend limit · an org owner or channel manager can raise it in the channel's Claude settings"), escalate, time.Time{}, "You've hit your channel's monthly spend limit · an org owner or channel manager can raise it in the channel's Claude settings"},
		{"channel's monthly usage limit", envelope("You've hit your channel's monthly usage limit · an org owner or channel manager can raise it in the channel's Claude settings · resets Nov 1, 12am (Europe/London)"),
			wait, time.Date(2026, 11, 1, 0, 0, 0, 0, london), ""},
		{"team's shared budget", envelope("You've hit your team's shared budget · raise it at https://claude.ai/admin-settings/usage · your weekly limit resets Oct 15, 10am (Europe/London)"),
			wait, time.Date(2026, 10, 15, 10, 0, 0, 0, london), ""},
		{"reached Fable limit", envelope("You've reached your Fable limit. Switch to another model to continue."), escalate, time.Time{}, "You've reached your Fable limit. Switch to another model to continue."},
		{"out of usage credits", envelope("You're out of usage credits · resets 5pm (Europe/London)"),
			wait, time.Date(2026, 10, 12, 17, 0, 0, 0, london), ""},
		{"out of extra usage", envelope("You're out of extra usage"), escalate, time.Time{}, "You're out of extra usage"},
		{"org out of usage", envelope("Your org is out of usage · contact your admin"), escalate, time.Time{}, "Your org is out of usage · contact your admin"},
		{"wait notice", "Usage limit reached · limit resets 3pm (Europe/London)\n",
			wait, time.Date(2026, 10, 12, 15, 0, 0, 0, london), ""},
		{"limit, no reset time", envelope("You've hit your session limit"), wait, time.Time{}, ""},
		{"spend limit, no reset time", envelope("You've hit your monthly spend limit."), escalate, time.Time{}, "You've hit your monthly spend limit."},
		{"limit, unexpected time format", envelope("You've hit your session limit · resets in 2h 15m"), wait, time.Time{}, ""},
		{"limit, unknown time zone", envelope("You've hit your session limit · resets 3pm (Mars/Olympus)"), wait, time.Time{}, ""},
		{"normal output", readTestdata(t, "claude-fresh.json"), none, time.Time{}, ""},
		{"limit mentioned in another context",
			`{"type":"result","is_error":false,"session_id":"x","result":"Status: Error\nDone: ran go test.\nOpen: TestRateLimit fails: usage limit reached after 3 calls. The session limit in config is 5; we've hit your limit check early.\nNext: Codex fixes the limiter."}` + "\n",
			none, time.Time{}, ""},
		{"fast mode cooldown", envelope("You've hit your fast limit · resets in 5m"), none, time.Time{}, ""},
		{"approaching a limit is a warning", envelope("You've used 92% of your weekly limit · resets Oct 15, 10am (Europe/London)"), none, time.Time{}, ""},
		{"close to a limit is a warning", envelope("You're close to your usage credit limit"), none, time.Time{}, ""},
		{"out of usage credits, no reset time", envelope("You're out of usage credits"), escalate, time.Time{}, "You're out of usage credits"},
		{"team's shared budget, no reset time", envelope("You've hit your team's shared budget. Switch to another model to continue."),
			escalate, time.Time{}, "You've hit your team's shared budget. Switch to another model to continue."},
		{"seat without usage credits", envelope("Your seat type doesn't include usage credits"), escalate, time.Time{}, "Your seat type doesn't include usage credits"},
		{"seat without usage", envelope("Your seat type doesn't include usage"), escalate, time.Time{}, "Your seat type doesn't include usage"},
		{"seat without extra usage", envelope("Your seat type doesn't include extra usage"), escalate, time.Time{}, "Your seat type doesn't include extra usage"},
		{"service disabled", envelope("This service is disabled for your org"), escalate, time.Time{}, "This service is disabled for your org"},
		{"allocation disabled", envelope("Your usage allocation has been disabled by your admin · ask your admin for a higher limit"),
			escalate, time.Time{}, "Your usage allocation has been disabled by your admin · ask your admin for a higher limit"},
		{"group limit is $0", envelope("Your group's usage limit is set to $0 · ask your admin for a higher limit"),
			escalate, time.Time{}, "Your group's usage limit is set to $0 · ask your admin for a higher limit"},
		{"model requires usage credits", envelope("Opus 5 requires usage credits. Switch to another model to continue."),
			escalate, time.Time{}, "Opus 5 requires usage credits. Switch to another model to continue."},
		{"Fable 5 requires usage credits, on stderr", "API Error: Fable 5 requires usage credits. Run /usage-credits to keep using Fable 5 or /model to switch models.\n",
			escalate, time.Time{}, "Fable 5 requires usage credits. Run /usage-credits to keep using Fable 5 or /model to switch models."},
		{"limit with a reset time beats an escalation", envelope(`Your seat type doesn't include extra usage\nYou've hit your weekly limit · resets Oct 15, 10am (Europe/London)`),
			wait, time.Date(2026, 10, 15, 10, 0, 0, 0, london), "You've hit your weekly limit · resets Oct 15, 10am (Europe/London)"},
		{"escalation beats a limit without a reset time", envelope(`You've hit your session limit\nYou're out of extra usage`),
			escalate, time.Time{}, "You're out of extra usage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkUsage(t, ClaudeUsageLimit(tt.output, now), tt.kind, tt.resetAt, tt.msg)
		})
	}
}
