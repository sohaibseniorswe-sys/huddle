# 08 — Publish: draft → GitHub

## Objective
Publish a draft to a project repo on human approval: milestone, issues, control issue, labels and spec files; gate N+1; close superseded issues on adoption.

## Context
This is the single write path to GitHub. See README §1, §3–4.

## Requirements
- Create the GitHub **milestone**; create **issues** with short reference bodies (`Spec: <file>. Read <README> first. Depends on: …`) in dependency order; create the **control issue** with the `<!-- huddle:control -->` marker + `control` + `state:dev-ready`; create the **state labels** if absent; write the **README + spec files** to `docs/<milestone>/` on the project's working branch.
- **Gating**: refuse to publish a new control issue while another control issue is open/active in that repo.
- **Adoption mode**: close each superseded old issue with a link to its replacement.
- **Human approval** is required before any write.

## Out of scope
The planning conversation (07).

## Success criteria
- Publishing a draft produces the full structure on sandbox (milestone, issues, control, labels, specs).
- Labels are created if missing; the control marker is clean (no corruption).
- Gating refuses a second active control issue.
- Adoption closes and links superseded issues.

## Evidence required
A sandbox publish run showing the created milestone/issues/control/labels; a gating-refusal test.

## Dependencies
07, 02
