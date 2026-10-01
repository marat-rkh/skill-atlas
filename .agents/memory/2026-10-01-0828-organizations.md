# Maps of whole GitHub organizations (issue #4)

- **When:** 2026-10-01, from 08:28
- **Where:** a cloud machine (Linux, 4 cores), on the `organizations` branch
- **Transcript:** `29e1d2d4-8f4c-4943-967e-08cff44275eb`
- **Result:** PR #7, which closes issue #4

## What happened

1. Issue #4 asks to map whole organizations, e.g. `https://github.com/JetBrains`. A shell prototype measured the job
   first: JetBrains has 865 public repositories (182 forks, 155 archived, 3 empty). 34 of them have skills, and 8 of
   those are forks; `JetBrains/hermes-agent`, a fork of NousResearch's agent, alone has 198 of their skills. Each
   repository takes 1–2.5s even for the largest ones (kotlin, intellij-community, MPS), so the total time depends on the
   number of repositories, not their size.
2. **Spec and code:**
   - Input: `https://github.com/<org>`, `https://github.com/orgs/<org>/...`; a user's URL works the same way. The CLI
     tells repositories and organizations apart by the URL; the web takes `/scan?org=<url>` next to `/scan?repo=<url>`.
   - The repositories are listed with the GitHub REST API (`/users/<org>/repos`, following the `Link` header). Forks
     and disabled repositories are skipped. `GITHUB_TOKEN` is sent if set; CI sets it for the end-to-end tests.
   - 32 repositories are analyzed at a time: 8 took 94s for JetBrains, 16 took 51s, 32 took 34s, 64 took 36s.
   - The map of an organization is a summary line, `JetBrains · 683 repositories · 26 with skills · 412 skills`,
     followed by the map of each repository with skills, exactly as it is shown alone. Grouping works within each
     repository; the web filter counts and hides across the whole organization.
   - An empty repository now gives a "repository is empty" error alone and counts as having no skills in an
     organization. A repository that fails doesn't stop the others: the CLI prints an error for it after the map and
     exits with 1, and the web page shows the errors above the map.
   - The web server keeps each map for 5 minutes (`cached` in `cache.go`), so changing the filter or grouping doesn't
     analyze an organization again. Requests for a map that is still being built wait for it.
3. Checked on JetBrains: the CLI, the web page (37s, then 1ms with a filter), and a `-race` build. The demo got an
   organization scene in both parts.

## Decisions

- **Forks are skipped:** their skills usually come from upstream and would swamp the map (198 of 638 JetBrains skill
  files are in one fork). Archived repositories are kept: they are the organization's own.
- **One section per repository** instead of one flat list: each section is the repository's own map, which also shows
  where each skill lives (an open point from earlier sessions).
- **`org` as a separate web parameter**, so `repo=` keeps meaning a repository and the URL reads well.
- **The cache was added with this feature:** without it, every filter change rescanned the organization (about 35s and
  9 API requests, of 60 per hour without a token).

## Findings

- GitHub's `ls-remote --symref HEAD` prints nothing for an empty repository, and `fetch` then fails with "couldn't
  find remote ref HEAD". `anthropics/homebrew-claude` is such a repository.
- In this environment `gh auth token` works, but `GH_TOKEN` is empty in the shell. Use
  `GITHUB_TOKEN=$(gh auth token)` for live runs; the anonymous limit of 60 requests per hour ran out during the work.
- The cloud machine had no root, C compiler, Chrome, vhs or ffmpeg, and the Playwright CDN and PyPI were blocked.
  What worked: Ubuntu packages through a user-local apt (`-o Dir::State=...`), unpacked with `dpkg-deb -x` (gcc for
  `go test -race`, Chrome's libraries and fonts); Chrome for Testing from storage.googleapis.com; ttyd and ffmpeg from
  GitHub releases (that ffmpeg also copied to `~/.cache/ms-playwright/ffmpeg-1011/ffmpeg-linux`); vhs with
  `go install`. vhs needs `VHS_NO_SANDBOX=true` there, and `demo/web.mjs` now takes `CHROME_PATH`.

## Left open

- There is no progress indicator while an organization is analyzed (about 35s for JetBrains on this machine).
- The 5-minute cache can show a map that is up to 5 minutes old; there is no way to force a fresh scan except waiting.
