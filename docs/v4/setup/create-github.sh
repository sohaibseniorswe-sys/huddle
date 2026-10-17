#!/usr/bin/env bash
# One-time GitHub setup for the Huddle v4 milestone.
# Run from anywhere; requires `gh` authenticated. Safe to re-run (labels use --force;
# milestone and issues are only created if missing by title).
#
# PREREQUISITE: the v4 branch must already exist with docs/v4/ committed and pushed,
# because the issue bodies reference docs/v4/ spec files.
set -euo pipefail

REPO="sohaibseniorswe-sys/huddle"
MILESTONE_TITLE="v4 — Project Lifecycle & Multi-Project"
MILESTONE_DESC="Multi-project, lifecycle-aware Huddle; removes the hardcoded Palantir entry. Spec: docs/v4/README.md"
README="docs/v4/README.md"

echo "== Labels =="
gh label create "control"              -R "$REPO" --color 5319E7 --force
gh label create "state:dev-ready"      -R "$REPO" --color 0E8A16 --force
gh label create "state:dev-working"    -R "$REPO" --color FBCA04 --force
gh label create "state:pm-ready"       -R "$REPO" --color 1D76DB --force
gh label create "state:pm-working"     -R "$REPO" --color C5DEF5 --force
gh label create "state:human-required" -R "$REPO" --color D93F0B --force
gh label create "state:done"           -R "$REPO" --color 6F42C1 --force

echo "== Milestone =="
MNUM=$(gh api "repos/$REPO/milestones" --jq ".[] | select(.title==\"$MILESTONE_TITLE\") | .number" || true)
if [ -z "${MNUM:-}" ]; then
  MNUM=$(gh api "repos/$REPO/milestones" -f title="$MILESTONE_TITLE" -f state=open -f description="$MILESTONE_DESC" --jq .number)
  echo "created milestone #$MNUM"
else
  echo "milestone exists #$MNUM"
fi

# number|slug|title|depends
ISSUES=(
"01|state-root-and-store|v4.01 — State root + huddle-data store + registry|—"
"02|identity-and-phase|v4.02 — Project identity & lifecycle-phase derivation|01"
"03|startup-discovery|v4.03 — Startup discovery, listing & routing (remove Palantir entry)|02"
"04|resume-reconcile|v4.04 — Resume interrupted runs & reconcile drift|03"
"05|fresh-session-per-issue|v4.05 — Fresh agent session per issue|03"
"06|concurrency-lanes|v4.06 — Concurrency: one build loop, lane separation, usage warning|03"
"07|planning-mode|v4.07 — Planning mode: companion-driven drafting|02"
"08|publish|v4.08 — Publish: draft to GitHub (milestone/issues/control/labels), gating|07, 02"
"09|new-project|v4.09 — New project from scratch (Case 2)|07, 08"
"10|onboarding-flows|v4.10 — Onboarding flows: adopt, plan-next, plan-while-building|07, 08"
"11|text-selection|v4.11 — Selectable & copyable text in agent panes|—"
"12|integration-docs|v4.12 — Integration, docs & fresh-machine story|all"
)

echo "== Implementation issues =="
for row in "${ISSUES[@]}"; do
  IFS='|' read -r num slug title deps <<< "$row"
  # skip if an open issue with this title already exists
  existing=$(gh issue list -R "$REPO" --state open --search "\"$title\" in:title" --json title --jq 'length' || echo 0)
  if [ "$existing" != "0" ]; then echo "skip (exists): $title"; continue; fi
  body="Spec: \`docs/v4/issues/${num}-${slug}.md\`. Read \`${README}\` first.

Depends on: ${deps}"
  gh issue create -R "$REPO" --title "$title" --body "$body" --milestone "$MILESTONE_TITLE" >/dev/null
  echo "created: $title"
done

echo "== Control issue =="
ctrl_existing=$(gh issue list -R "$REPO" --state open --label control --json number --jq 'length' || echo 0)
if [ "$ctrl_existing" != "0" ]; then
  echo "control issue already exists; not creating a second one"
else
  ctrl_body="<!-- huddle:control -->
Control issue for the v4 milestone — Huddle's channel between the Dev and PM.

First Dev task:
Implement the v4 milestone. The spec is in \`docs/v4/\`; read \`${README}\` first."
  gh issue create -R "$REPO" \
    --title "v4 · Control" \
    --body "$ctrl_body" \
    --milestone "$MILESTONE_TITLE" \
    --label "control" --label "state:dev-ready" >/dev/null
  echo "created control issue with marker + control + state:dev-ready"
fi

echo "== Done =="
echo "Verify the control marker is intact:"
echo "  gh issue list -R $REPO --label control"
