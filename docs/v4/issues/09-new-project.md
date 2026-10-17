# 09 — New project from scratch (Case 2)

## Objective
Start a project from nothing: no name, directory or repo required up front.

## Context
See README §2.

## Requirements
- Begin with a planning session under a new internal id; no name/dir/repo required.
- Name → directory → repo are created as **outputs** when decided (repo creation via gh/API, human-approved).
- Publish the first milestone via issue 08.
- Persist across sessions; the project appears in the registry as `planning` until published.

## Out of scope
Adopting existing projects (10).

## Success criteria
- A from-scratch flow creates a registry entry, then a directory + repo when decided, then a published milestone.
- Closing and reopening mid-way resumes exactly where it left off.

## Evidence required
A sandbox walk-through creating a throwaway new project.

## Dependencies
07, 08

## PM decision (2026-10-17, control #25)
Resolves the conflict raised in #21 between this spec, AGENTS.md and the README "Huddle never runs git" invariant. This spec is the source of truth and already requires creating a throwaway repo.
- **Acceptance repo:** the walkthrough MAY create one new **private** throwaway repo under `sohaibseniorswe-sys`, named `huddle-sandbox-new-<YYYYMMDD-HHMM>`, using an isolated `HUDDLE_HOME`. Record the repo name in the evidence. Afterwards archive it (`gh repo archive`); deletion is optional and not required. No other new repos.
- **Repo creation path:** Huddle creates the GitHub repo through gh/API only after the human approves it in the existing publish/approval gate (issue 08 write path). In automated acceptance the approval may be scripted exactly as in 08's evidence.
- **Directory + initial checkout:** Huddle creates the empty project directory. Initializing the checkout is a **setup-only bootstrap**: either the first agent turn clones/initializes it (preferred, keeps the invariant intact), or Huddle performs a single `git clone` of the repo it just created into the directory it just created. If Huddle does the clone, document it in the README as the only exception to the invariant. After bootstrap, all project Git work belongs to the agents. The Dev picks the mechanism.
- **Ordering:** registry entry (`planning`, internal id) → name → directory → repo → bootstrap → publish first milestone → build loop → done.
