#!/usr/bin/env bash
# create-milestone.sh creates the "Huddle Version 3" milestone and one issue per
# spec in docs/v3/issues/ on GitHub, using the gh CLI.
#
# Each issue body is its spec, with image and file links rewritten to point at
# docs/v3 on GitHub, so the screenshots show up in the issue. That only works
# once docs/v3 is pushed, so the script checks that first.
#
# Usage, from the root of the huddle repo:
#   ./docs/v3/create-milestone.sh --dry-run        # preview, touches nothing
#   ./docs/v3/create-milestone.sh                  # create on sohaibseniorswe-sys/huddle
#   ./docs/v3/create-milestone.sh owner/repo       # create on another repo
#
# Safe to run twice: an existing milestone or issue with the same title is skipped.

set -euo pipefail

MILESTONE="Huddle Version 3"
BRANCH="main"
DIR="docs/v3"
PREVIEW_DIR="/tmp/huddle-v3-preview"

DRY_RUN=false
REPO="sohaibseniorswe-sys/huddle"
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    */*) REPO="$arg" ;;
    *) echo "Unknown argument: $arg" >&2; exit 2 ;;
  esac
done

BASE="https://github.com/$REPO/blob/$BRANCH/$DIR"

# issue_body prints the GitHub issue body for one spec file:
#   - a pointer to the spec in the repo (the source of truth),
#   - the spec without its first line (the title),
#   - ../screens/x.png links turned into image URLs that render in issues,
#   - other relative links (../README.md, ...) turned into GitHub file links.
issue_body() {
  local spec="$1"
  local name
  name="$(basename "$spec")"
  {
    echo "> Spec: [\`$DIR/issues/$name\`]($BASE/issues/$name) is the source of truth. Read it together with [README.md]($BASE/README.md) and [coding-guidelines.md]($BASE/coding-guidelines.md) before starting."
    tail -n +2 "$spec"
  } | sed -E \
      -e "s#\]\(\.\./screens/([^)]+\.png)\)#]($BASE/screens/\1?raw=true)#g" \
      -e "s#\]\(\.\./([^)]+)\)#]($BASE/\1)#g"
}

# issue_title prints the title: the spec's first line without "# ".
issue_title() {
  head -n 1 "$1" | sed -E 's/^# +//'
}

# --- Checks -----------------------------------------------------------------

if [ ! -d "$DIR/issues" ]; then
  echo "Run this from the root of the huddle repo (no $DIR/issues here)." >&2
  exit 1
fi

if $DRY_RUN; then
  rm -rf "$PREVIEW_DIR"
  mkdir -p "$PREVIEW_DIR"
  echo "Dry run for $REPO. Nothing is created."
  echo "Milestone: $MILESTONE"
  echo "Description: $(cat "$DIR/milestone-description.md")"
  echo
  for spec in "$DIR"/issues/*.md; do
    title="$(issue_title "$spec")"
    issue_body "$spec" > "$PREVIEW_DIR/$(basename "$spec")"
    echo "Issue: $title  ->  preview in $PREVIEW_DIR/$(basename "$spec")"
  done
  exit 0
fi

command -v gh >/dev/null || { echo "gh is not installed." >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "gh is not logged in. Run: gh auth login" >&2; exit 1; }

if [ -n "$(git status --porcelain -- "$DIR")" ]; then
  echo "$DIR has changes that are not committed. Commit and push them first:" >&2
  echo "  git add $DIR && git commit -m 'Add Huddle v3 specs' && git push" >&2
  exit 1
fi
git fetch -q origin "$BRANCH"
if ! git ls-tree -r --name-only "origin/$BRANCH" -- "$DIR/issues" | grep -q . ||
   ! git diff --quiet "origin/$BRANCH" -- "$DIR"; then
  echo "$DIR on origin/$BRANCH is missing or out of date. Push it first: git push" >&2
  exit 1
fi

# --- Milestone --------------------------------------------------------------

number="$(gh api "repos/$REPO/milestones?state=all&per_page=100" \
  --jq ".[] | select(.title == \"$MILESTONE\") | .number")"
if [ -z "$number" ]; then
  gh api "repos/$REPO/milestones" -f title="$MILESTONE" \
    -f description="$(cat "$DIR/milestone-description.md")" >/dev/null
  echo "Created milestone: $MILESTONE"
else
  echo "Milestone already exists: $MILESTONE (#$number)"
fi

# --- Issues -----------------------------------------------------------------

for spec in "$DIR"/issues/*.md; do
  title="$(issue_title "$spec")"
  existing="$(gh issue list --repo "$REPO" --milestone "$MILESTONE" --state all \
    --limit 100 --json title --jq '.[].title')"
  if grep -Fxq "$title" <<<"$existing"; then
    echo "Already exists: $title"
    continue
  fi
  url="$(gh issue create --repo "$REPO" --milestone "$MILESTONE" \
    --title "$title" --body "$(issue_body "$spec")")"
  echo "Created: $title  $url"
done

echo
echo "Done. Milestone: https://github.com/$REPO/milestones"
