package runner

import (
	"testing"
	"time"
)

func TestCodexUsageLimit(t *testing.T) {
	loc := time.FixedZone("UTC+1", 3600)
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, loc)
	header := "OpenAI Codex v0.160.0\n--------\nsession id: 01a132be-f0bc-7293-89cc-72f23dcb576c\n--------\nuser\nIt's your turn.\n\n"
	tests := []struct {
		name    string
		output  string
		hit     bool
		resetAt time.Time
	}{
		{"limit, reset later today",
			header + "ERROR: You’ve hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at 3:45 PM.\n",
			true, time.Date(2026, 10, 12, 15, 45, 0, 0, loc)},
		{"limit, reset on another day",
			header + "ERROR: You’ve hit your usage limit. Try again at Oct 13th, 2026 9:05 AM.\n",
			true, time.Date(2026, 10, 13, 9, 5, 0, 0, loc)},
		{"limit, plus plan wording, other day",
			"You’ve hit your usage limit. Upgrade to Plus to continue using Codex (https://chatgpt.com/explore/plus), or try again at Nov 1st, 2026 12:00 AM.\n",
			true, time.Date(2026, 11, 1, 0, 0, 0, 0, loc)},
		{"limit, ASCII apostrophe", "ERROR: You've hit your usage limit. Try again at 11:30 AM.\n",
			true, time.Date(2026, 10, 12, 11, 30, 0, 0, loc)},
		{"model-specific limit", header + "ERROR: You’ve hit your usage limit for GPT-6 Sol. Switch to another model now, or try again at 4:10 PM.\n",
			true, time.Date(2026, 10, 12, 16, 10, 0, 0, loc)},
		{"model-specific limit, other day", "ERROR: You’ve hit your usage limit for GPT-6 Luna. Switch to another model now, or try again at Oct 19th, 2026 8:00 AM.\n",
			true, time.Date(2026, 10, 19, 8, 0, 0, 0, loc)},
		{"pro plan wording", "ERROR: You’ve hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Oct 21st, 2026 1:00 PM.\n",
			true, time.Date(2026, 10, 21, 13, 0, 0, 0, loc)},
		{"team plan wording", "ERROR: You’ve hit your usage limit. To get more access now, send a request to your admin or try again at Oct 22nd, 2026 2:00 PM.\n",
			true, time.Date(2026, 10, 22, 14, 0, 0, 0, loc)},
		{"model-specific limit, no reset time", "ERROR: You’ve hit your usage limit for GPT-6 Sol. Switch to another model now, or try again later.\n", true, time.Time{}},
		{"workspace out of credits", "ERROR: Your workspace is out of credits. Add credits to continue.\n", true, time.Time{}},
		{"workspace out of credits, member", "ERROR: Your workspace is out of credits. Ask your workspace owner to refill in order to continue.\n", true, time.Time{}},
		{"spend cap", "ERROR: You hit your spend cap set in your workspace. Increase your spend cap to continue.\n", true, time.Time{}},
		{"spend cap set by owner", "ERROR: You hit your spend cap set by the owner of your workspace. Ask an owner to increase your spend cap to continue.\n", true, time.Time{}},
		{"TUI usage limit reached", "Usage limit reached. You’ve reached your usage limit. Increase your limits to continue using codex.\n", true, time.Time{}},
		{"TUI reached usage limit", "You've reached your usage limit. Increase your limits to continue using codex.\n", true, time.Time{}},
		{"TUI workspace credit limit", "You’ve reached your workspace credit limit\n", true, time.Time{}},
		{"TUI out of credits", "You’re out of credits. Your workspace is out of credits. Add credits to continue using Codex.\n", true, time.Time{}},
		{"limit, no reset time", header + "ERROR: You’ve hit your usage limit. Try again later.\n", true, time.Time{}},
		{"limit, admin wording, no reset time", "ERROR: You’ve hit your usage limit. To get more access now, send a request to your admin.\n", true, time.Time{}},
		{"limit, unexpected time format", "ERROR: You’ve hit your usage limit. Try again at 2026-10-13T09:05:00Z.\n", true, time.Time{}},
		{"normal output", readTestdata(t, "codex-fresh.out"), false, time.Time{}},
		{"limit mentioned in another context",
			header + "exec\ngo test ./...\n--- FAIL: TestRateLimiter (0.00s)\n    limiter_test.go:12: usage limit reached after 3 calls, want 5\ncodex\nThe rate limit test fails; you've hit your usage limit check too early.\n",
			false, time.Time{}},
		{"API quota, not a subscription limit", "ERROR: Quota exceeded. Check your plan and billing details.\n", false, time.Time{}},
		{"plan without codex, not a limit", "ERROR: To use Codex with your ChatGPT plan, upgrade to Plus: https://chatgpt.com/explore/plus.\n", false, time.Time{}},
		{"transient rate limit retries", "ERROR: exceeded retry limit, last status: 429 Too Many Requests\n", false, time.Time{}},
		{"quoted mid-line", "codex\nThe PM said “You’ve hit your usage limit” is handled.\n", false, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CodexUsageLimit(tt.output, now)
			if got.Hit != tt.hit || !got.ResetAt.Equal(tt.resetAt) {
				t.Errorf("CodexUsageLimit() = %+v, want hit %t reset %v", got, tt.hit, tt.resetAt)
			}
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
		hit     bool
		resetAt time.Time
	}{
		{"session limit, reset later today", envelope("You've hit your session limit · resets 3pm (Europe/London)"),
			true, time.Date(2026, 10, 12, 15, 0, 0, 0, london)},
		{"limit, reset with minutes, other zone", envelope("You've hit your limit · resets 3:30pm (America/New_York) · progress saved"),
			true, time.Date(2026, 10, 12, 19, 30, 0, 0, time.UTC)},
		{"time already past today means tomorrow", envelope("You've hit your session limit · resets 9am (Europe/London)"),
			true, time.Date(2026, 10, 13, 9, 0, 0, 0, london)},
		{"weekly limit, date", envelope("You've hit your weekly limit · resets Oct 15, 10am (Europe/London)"),
			true, time.Date(2026, 10, 15, 10, 0, 0, 0, london)},
		{"limit, date with year", envelope("You've hit your Opus limit · resets Jan 2, 2027, 12am (Europe/London)"),
			true, time.Date(2027, 1, 2, 0, 0, 0, 0, london)},
		{"envelope escapes the middle dot", envelope(`You've hit your usage limit · resets 3pm (Europe/London)`),
			true, time.Date(2026, 10, 12, 15, 0, 0, 0, london)},
		{"on stderr, not in an envelope", "Error: You've hit your session limit · resets 3pm (Europe/London)\n",
			true, time.Date(2026, 10, 12, 15, 0, 0, 0, london)},
		{"legacy message with unix time", envelope("Claude AI usage limit reached|1792242000"),
			true, time.Unix(1792242000, 0)},
		{"weekly limit, reset within a day", envelope("You've hit your weekly limit · resets 11pm (Europe/London) · progress saved"),
			true, time.Date(2026, 10, 12, 23, 0, 0, 0, london)},
		{"Opus limit", envelope("You've hit your Opus limit · resets Oct 16, 9:30am (Europe/London)"),
			true, time.Date(2026, 10, 16, 9, 30, 0, 0, london)},
		{"Sonnet limit", envelope("You've hit your Sonnet limit · resets Oct 14, 6pm (Europe/London)"),
			true, time.Date(2026, 10, 14, 18, 0, 0, 0, london)},
		{"Fable limit", envelope("You've hit your Fable limit · resets Oct 17, 1pm (Europe/London)"),
			true, time.Date(2026, 10, 17, 13, 0, 0, 0, london)},
		{"usage limit", envelope("You've hit your usage limit · resets 4pm (Europe/London)"),
			true, time.Date(2026, 10, 12, 16, 0, 0, 0, london)},
		{"usage limit, contact admin", envelope("You've hit your usage limit · contact your admin to increase it"), true, time.Time{}},
		{"monthly spend limit with the plan limit's reset", envelope("You've hit your monthly spend limit · raise it at https://claude.ai/settings/usage · your weekly limit resets Oct 15, 10am (Europe/London)"),
			true, time.Date(2026, 10, 15, 10, 0, 0, 0, london)},
		{"org's monthly spend limit", envelope("You've hit your org's monthly spend limit · ask your admin to raise it at https://claude.ai/admin-settings/usage · your session limit resets 2pm (Europe/London)"),
			true, time.Date(2026, 10, 12, 14, 0, 0, 0, london)},
		{"individual spend limit", envelope("You've hit your individual spend limit · ask your admin for a higher limit"), true, time.Time{}},
		{"individual usage limit", envelope("You've hit your individual usage limit · resets Nov 1, 12am (Europe/London)"),
			true, time.Date(2026, 11, 1, 0, 0, 0, 0, london)},
		{"org's monthly usage limit", envelope("You've hit your org's monthly usage limit · resets Nov 1, 12am (Europe/London)"),
			true, time.Date(2026, 11, 1, 0, 0, 0, 0, london)},
		{"channel's monthly spend limit", envelope("You've hit your channel's monthly spend limit · an org owner or channel manager can raise it in the channel's Claude settings"), true, time.Time{}},
		{"channel's monthly usage limit", envelope("You've hit your channel's monthly usage limit · an org owner or channel manager can raise it in the channel's Claude settings · resets Nov 1, 12am (Europe/London)"),
			true, time.Date(2026, 11, 1, 0, 0, 0, 0, london)},
		{"team's shared budget", envelope("You've hit your team's shared budget · raise it at https://claude.ai/admin-settings/usage · your weekly limit resets Oct 15, 10am (Europe/London)"),
			true, time.Date(2026, 10, 15, 10, 0, 0, 0, london)},
		{"reached Fable limit", envelope("You've reached your Fable limit. Switch to another model to continue."), true, time.Time{}},
		{"out of usage credits", envelope("You're out of usage credits · resets 5pm (Europe/London)"),
			true, time.Date(2026, 10, 12, 17, 0, 0, 0, london)},
		{"out of extra usage", envelope("You're out of extra usage"), true, time.Time{}},
		{"org out of usage", envelope("Your org is out of usage · contact your admin"), true, time.Time{}},
		{"wait notice", "Usage limit reached · limit resets 3pm (Europe/London)\n",
			true, time.Date(2026, 10, 12, 15, 0, 0, 0, london)},
		{"limit, no reset time", envelope("You've hit your session limit"), true, time.Time{}},
		{"spend limit, no reset time", envelope("You've hit your monthly spend limit."), true, time.Time{}},
		{"limit, unexpected time format", envelope("You've hit your session limit · resets in 2h 15m"), true, time.Time{}},
		{"limit, unknown time zone", envelope("You've hit your session limit · resets 3pm (Mars/Olympus)"), true, time.Time{}},
		{"normal output", readTestdata(t, "claude-fresh.json"), false, time.Time{}},
		{"limit mentioned in another context",
			`{"type":"result","is_error":false,"session_id":"x","result":"Status: Error\nDone: ran go test.\nOpen: TestRateLimit fails: usage limit reached after 3 calls. The session limit in config is 5; we've hit your limit check early.\nNext: Codex fixes the limiter."}` + "\n",
			false, time.Time{}},
		{"fast mode cooldown", envelope("You've hit your fast limit · resets in 5m"), false, time.Time{}},
		{"approaching a limit is a warning", envelope("You've used 92% of your weekly limit · resets Oct 15, 10am (Europe/London)"), false, time.Time{}},
		{"close to a limit is a warning", envelope("You're close to your usage credit limit"), false, time.Time{}},
		{"seat without usage credits is not a limit", envelope("Your seat type doesn't include usage credits"), false, time.Time{}},
		{"model needing credits is not a limit", envelope("Opus 5 requires usage credits. Switch to another model to continue."), false, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClaudeUsageLimit(tt.output, now)
			if got.Hit != tt.hit || !got.ResetAt.Equal(tt.resetAt) {
				t.Errorf("ClaudeUsageLimit() = %+v, want hit %t reset %v", got, tt.hit, tt.resetAt)
			}
		})
	}
}
