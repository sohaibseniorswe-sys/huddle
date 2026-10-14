package companion

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  Reply
	}{
		{
			name: "well-formed",
			reply: "MapLibre needs no key. Here's the decision:\n" +
				"<decision>Use MapLibre with OpenStreetMap tiles for #65 now.</decision>\n" +
				"<reason>no Mapbox token yet</reason>",
			want: Reply{
				Text:     "MapLibre needs no key. Here's the decision:",
				Decision: "Use MapLibre with OpenStreetMap tiles for #65 now.",
				Reason:   "no Mapbox token yet",
			},
		},
		{
			name:  "missing reason",
			reply: "Done.\n<decision>Skip the map for now.</decision>",
			want:  Reply{Text: "Done.", Decision: "Skip the map for now."},
		},
		{
			name:  "tags inside code fences are examples",
			reply: "The format is:\n```\n<decision>...</decision>\n<reason>one line</reason>\n```\nNothing to decide yet.",
			want: Reply{
				Text: "The format is:\n```\n<decision>...</decision>\n<reason>one line</reason>\n```\nNothing to decide yet.",
			},
		},
		{
			name: "two decisions: the last wins",
			reply: "<decision>Use Mapbox.</decision><reason>first</reason>\nActually no.\n" +
				"<decision>Use MapLibre.</decision>\n<reason>second</reason>",
			want: Reply{Text: "Actually no.", Decision: "Use MapLibre.", Reason: "second"},
		},
		{
			name:  "no tags",
			reply: "  Codex is on #62 and CI for PR #73 is running.  ",
			want:  Reply{Text: "Codex is on #62 and CI for PR #73 is running."},
		},
		{
			name:  "a decision across lines becomes one line",
			reply: "<decision>\n  Use MapLibre\n  for #65.\n</decision>",
			want:  Reply{Decision: "Use MapLibre for #65."},
		},
		{
			name:  "action",
			reply: "CI is red. <action>pause</action>",
			want:  Reply{Text: "CI is red.", Action: "pause"},
		},
		{
			name:  "log action with a time",
			reply: "<action>log 22:52</action> shows it.",
			want:  Reply{Text: "shows it.", Action: "log 22:52"},
		},
		{
			name:  "unknown action is ignored",
			reply: "<action>merge PR #73</action>",
			want:  Reply{},
		},
		{
			name:  "a decision beats an action",
			reply: "<decision>Use MapLibre.</decision><action>pause</action>",
			want:  Reply{Decision: "Use MapLibre."},
		},
		{
			name:  "a reason without a decision is dropped",
			reply: "Hmm. <reason>because</reason>",
			want:  Reply{Text: "Hmm."},
		},
		{
			name:  "an unclosed fence hides the rest",
			reply: "Example:\n```\n<decision>x</decision>",
			want:  Reply{Text: "Example:\n```\n<decision>x</decision>"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(tt.reply); got != tt.want {
				t.Errorf("Parse() =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}
