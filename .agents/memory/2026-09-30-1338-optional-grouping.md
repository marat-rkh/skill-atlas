# Optional grouping of similar skills

- **When:** 2026-09-30, 13:38–15:04
- **Where:** a separate worktree on the `optional-grouping` branch
- **Transcript:** `b12f4f14-65a5-4040-9190-501dfddbfe10`
- **Result:** commits `125f227` (spec) and `86b7c7a` (code); PR #3, green in CI and merged as `829b4d2`

## What happened

1. The user asked for grouping to become optional, turned on with a checkbox, starting with the spec. The first draft
   kept the existing grouping by directory.
2. The user wanted grouping by similarity instead. Four options were proposed: a shared name prefix, word overlap in
   names and descriptions (TF-IDF), a fixed list of themes with keywords, or asking an LLM. The user chose the **shared
   name prefix**.
3. **Spec and code:**
   - By default the map is one list sorted by name.
   - With grouping on, skills whose names share their first word (the part before the first `-`) form a group. The
     group's label is the leading words all its names share, e.g. `analysis-api`. Skills that share their first word
     with no other skill go last, under "Other".
   - The CLI got a `--group` flag. The web page got a "Group similar skills" checkbox, kept in the URL as `&group=on`;
     one line of inline JavaScript submits the form when it's clicked.
   - The map itself is now a flat sorted list, and grouping happens only when the output is rendered.
4. **Rebase onto PR #2 (filter),** which was merged in the meantime and touched the same files:
   - The filter field and the checkbox share one form, so changing either keeps the other.
   - Groups are built from all skills before the filter is applied, so a skill that matches stays in its group.
   - The filter tests were rewritten for the new example data, with added cases that combine filter and grouping.
5. Checked on JetBrains/kotlin with `filter=gradle&group=on`: "3 of 6 skills match", and only the `build` group is left.

## Decisions

- **Name prefix grouping:** it's small and predictable, and it already gives sensible groups on kotlin. Word overlap can
  be added later for repos that don't name skills consistently.
- **The CLI's default changed:** it prints a flat list unless `--group` is passed, so the CLI and the web show the same
  map.

## Left open at the end of the session

- Without directory grouping, the map no longer shows where each skill lives. Showing the path next to each skill was
  suggested but not done.
- An older `./skill-atlas serve` build, probably started from the IDE, was still running on port 8080. Restart it before
  checking the page in a browser.
