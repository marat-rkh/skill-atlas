# Overview

Skill Atlas is a tool that analyzes git repositories and generates a map of available agent skills.
The map can be presented in the terminal (CLI) or in a web browser (web interface).

# Input

Any GitHub repository URL, or a GitHub organization URL, e.g. `https://github.com/JetBrains` or
`https://github.com/orgs/JetBrains/repositories`, to analyze all repositories of the organization. The URL of a user
works the same way, for the user's repositories.

# Output

A map of available agent skills.

By default, the map is a single list of all skills, sorted by name. Grouping similar skills is optional and off by
default. When it is enabled, skills whose names start with the same word (the part before the first `-`) form a group.
The group is labeled with the leading words that all its names share, e.g. `analysis-api` for
`analysis-api-create-cherry-pick-issue` and `analysis-api-mark-internal-apis`. Groups are sorted by label and skills
within a group by name. Skills that share their first word with no other skill are listed last, under "Other".

## Organizations

The map of an organization starts with a summary of how many repositories were analyzed, how many of them have skills,
and how many skills they have in total, e.g. `JetBrains · 683 repositories · 26 with skills · 412 skills`. It is
followed by the map of each repository that has skills, as it would be shown for that repository alone, sorted by
repository name ignoring case. With grouping enabled, skills are grouped within each repository.

- The organization's public repositories are listed with the GitHub REST API. Forks are skipped, since their skills
  usually come from the upstream repository. Archived repositories are analyzed like any other.
- Repositories are analyzed in parallel. An empty repository has no skills.
- If some repositories cannot be analyzed, the map of the others is still shown, along with an error for each
  repository that failed. If the organization cannot be found or its repositories cannot be listed, an error is shown
  instead of the map.
- A step of analyzing a repository that takes more than 5 minutes, e.g. because the connection to GitHub stalled, fails
  with an error, so that a single repository cannot hold up the map of the others.
- Without authentication, the GitHub API allows 60 requests per hour, and each request lists up to 100 repositories.
  If the `GITHUB_TOKEN` environment variable is set, the requests are authenticated with it, which raises the limit.
  When the limit is exceeded, the error says when to try again.

# Presentation

Both presentation options show the same map; they differ only in how it is displayed.

## CLI

`skill-atlas <github-repository-url>` prints the map to the terminal.
`skill-atlas <github-organization-url>` prints the map of an organization.
`skill-atlas --group <github-url>` prints either map with grouping enabled.

For an organization, an error for each repository that could not be analyzed is printed after the map, and the exit
status is 1 if there are such errors.

## Web interface

`skill-atlas serve` starts a local web server at `http://127.0.0.1:8080` and prints its address.
The server accepts connections from the local machine only.

The repository URL is passed as a query parameter: opening `/scan?repo=<github-repository-url>` shows the map for that
repository, e.g. `http://127.0.0.1:8080/scan?repo=https://github.com/JetBrains/kotlin`.
Likewise, `/scan?org=<github-organization-url>` shows the map of an organization, e.g.
`http://127.0.0.1:8080/scan?org=https://github.com/JetBrains`; each repository's map has its own heading.
The start page `/` explains how to do this. If the parameter is not a GitHub repository URL (or organization URL), both
parameters are given, or the repository (or organization) cannot be analyzed, the page shows an error instead of the
map. Errors for repositories of an organization that could not be analyzed are shown above the map.

The server keeps each map it builds for 5 minutes. Opening the same map again within that time, e.g. with another
filter, with grouping changed or after a star was clicked, shows it without analyzing the repository or organization
again. The map of an organization with repositories that could not be analyzed is not kept, so that opening it again
retries them.

### Filter

The map page has a filter field. Submitting it reloads the page with the text in an optional `filter` query parameter,
e.g. `/scan?repo=https://github.com/JetBrains/kotlin&filter=test`, so a filtered map can be bookmarked or shared.
The field shows the current filter text.

- A skill matches if its name or description contains the filter text, ignoring case. Leading and trailing whitespace
  in the filter is ignored.
- Only matching skills are shown; groups with no matching skills are hidden.
- The page shows how many of the repository's skills match, e.g. `2 of 6 skills match "test"`. For an organization,
  the count covers the skills of all its repositories, and repositories with no matching skills are hidden.
- If no skills match, the page says so instead of showing groups.
- An empty or missing filter shows the whole map.

The filter applies to the web interface only; the CLI always prints the whole map.

### Grouping

The map page has a "Group similar skills" checkbox, unchecked by default. Checking it shows the map with grouping
enabled; unchecking it shows the single list again. The choice is kept in the page URL as `&group=on`, so a grouped map
can be opened directly, e.g. `http://127.0.0.1:8080/scan?repo=https://github.com/JetBrains/kotlin&group=on`.
Grouping and the filter can be combined: changing one keeps the other.

### Stars

Each skill on the map page has a star button: an empty star (☆) stars the skill, and a filled one (★) unstars it.
Clicking it reloads the page with the same filter and grouping.

- Starred skills are shown first. Without grouping, they lead the list; with grouping, they form a "Starred" group
  before all other groups. Starred skills and the other skills are each sorted by name.
- Groups are formed from all skills, so starring a skill does not change the other groups: starring
  `analysis-api-mark-internal-apis` leaves `analysis-api-create-cherry-pick-issue` in the `analysis-api` group.
- The filter applies to starred skills too, and a "Starred" group with no matching skills is hidden.
- A star belongs to a skill directory in a repository, so skills with the same name in different directories are
  starred separately. Repository owners and names are compared ignoring case, as on GitHub.
- On the map of an organization, each repository's starred skills come first in that repository's section (with
  grouping, in its own "Starred" group), and a star shown there is the same as on the repository's own map. Clicking it
  reloads the organization's map.
- Stars are saved in `skill-atlas/stars.json` in the user's configuration directory (e.g. `~/.config` on Linux,
  `~/Library/Application Support` on macOS), so they are kept when the server restarts.
- Other websites cannot change stars: the server rejects star requests sent from their pages.

Stars apply to the web interface only; the CLI always prints the map without them.

# Technologies

- Go, standard library only.
- The `git` command-line tool, used at runtime to fetch repositories.
- The GitHub REST API, used to list the repositories of an organization.
- The web interface uses Go's `net/http` and `html/template`; its page assets are embedded in the binary.
