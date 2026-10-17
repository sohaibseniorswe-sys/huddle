# 01 — State root + huddle-data store + registry

## Objective
Make Huddle's state root redirectable, introduce the `huddle-data` store and the project registry, and migrate existing per-project config into it.

## Context
Today config lives at a fixed `~/.huddle/projects/<owner>_<repo>.yaml`. v4 needs (a) a redirectable state root so instances can be isolated for testing, and (b) a portable store (`huddle-data`) that is the single home for the registry, config and planning drafts. See README §1.

## Requirements
- A single **state root** resolved from `HUDDLE_HOME` (env) or `--home` (flag), default `~/.huddle`. All config, registry, `huddle-data` clone and logs resolve under it. No hard-coded `~/.huddle` path remains.
- **`huddle-data`** store: a repo Huddle owns. If a `huddle-data` remote is configured and the local clone is absent, clone it; if absent remotely, init locally. Commit config/registry changes immediately; batch planning-draft commits.
- **Registry**: one file per project plus a light index. Schema per project: stable internal id, display name, directory path (optional), repo owner/name (optional), cached lifecycle phase, last-touched. Provide read / write / list.
- **Migration**: existing `~/.huddle/projects/*.yaml` → registry entries + config in `huddle-data`, lossless and idempotent, keeping a backup.

## Out of scope
Planning-draft contents (issue 07). GitHub phase derivation (issue 02).

## Success criteria
- With `HUDDLE_HOME=<tmp>`, every read/write lands under `<tmp>`; two processes with different roots share no state.
- Registry round-trips; `list` returns seeded projects.
- Existing yaml migrates without loss; re-running migration is a no-op.

## Evidence required
Table-driven tests against temp roots; a migration test from a sample yaml; a demonstration that two roots are isolated.

## Dependencies
—
