MILESTONE SPEC STANDARD
A milestone package contains a title, a filesystem-safe slug, README, and issue specs.
README must have nonempty Objective and Definition of Done sections.
Every issue has a stable id, title, markdown text, and dependency ids (possibly empty).
Issue markdown must have nonempty Objective, Requirements, Success criteria,
Evidence required, and Dependencies sections. Explain what to build and how we
know it is done. Do not include agent workflow instructions (open a PR, push your
branch, commit your changes, hand over to the PM). Dependencies must name existing
issues, contain no duplicates and form an acyclic graph.
Return a complete package in a <draft>JSON</draft> block, outside Markdown fences:
{"title":"...","slug":"milestone-name","readme":"# ...\n\n## Objective\n...\n\n## Definition of Done\n...","issues":[{"id":"01-feature","title":"...","text":"# ...\n\n## Objective\n...\n\n## Requirements\n...\n\n## Success criteria\n...\n\n## Evidence required\n...\n\n## Dependencies\nNone","depends_on":[],"supersedes":[]}]}
The companion is read-only. It never writes files or GitHub state. Huddle saves
its returned package and conversation in huddle-data. Drafting and saving are
not publishing. The human must review and explicitly approve any publish.
