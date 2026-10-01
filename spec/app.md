# Overview

Skill Atlas is a tool that analyzes git repositories and generates a map of available agent skills.
The map can be presented in the terminal (CLI) or in a web browser (web interface).

# Input

Any GitHub repository URL.

# Output

A map of available agent skills.

By default, the map is a single list of all skills, sorted by name. Grouping similar skills is optional and off by
default. When it is enabled, skills whose names start with the same word (the part before the first `-`) form a group.
The group is labeled with the leading words that all its names share, e.g. `analysis-api` for
`analysis-api-create-cherry-pick-issue` and `analysis-api-mark-internal-apis`. Groups are sorted by label and skills
within a group by name. Skills that share their first word with no other skill are listed last, under "Other".

# Presentation

Both presentation options show the same map; they differ only in how it is displayed.

## CLI

`skill-atlas <github-repository-url>` prints the map to the terminal.
`skill-atlas --group <github-repository-url>` prints it with grouping enabled.

## Web interface

`skill-atlas serve` starts a local web server at `http://127.0.0.1:8080` and prints its address.
The server accepts connections from the local machine only.

The repository URL is passed as a query parameter: opening `/scan?repo=<github-repository-url>` shows the map for that
repository, e.g. `http://127.0.0.1:8080/scan?repo=https://github.com/JetBrains/kotlin`.
The start page `/` explains how to do this. If the parameter is not a GitHub repository URL, or the repository cannot
be analyzed, the page shows an error instead of the map.

### Filter

The map page has a filter field. Submitting it reloads the page with the text in an optional `filter` query parameter,
e.g. `/scan?repo=https://github.com/JetBrains/kotlin&filter=test`, so a filtered map can be bookmarked or shared.
The field shows the current filter text.

- A skill matches if its name or description contains the filter text, ignoring case. Leading and trailing whitespace
  in the filter is ignored.
- Only matching skills are shown; groups with no matching skills are hidden.
- The page shows how many of the repository's skills match, e.g. `2 of 6 skills match "test"`.
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
- Stars are saved in `skill-atlas/stars.json` in the user's configuration directory (e.g. `~/.config` on Linux,
  `~/Library/Application Support` on macOS), so they are kept when the server restarts.
- Other websites cannot change stars: the server rejects star requests sent from their pages.

Stars apply to the web interface only; the CLI always prints the map without them.

# Technologies

- Go, standard library only.
- The `git` command-line tool, used at runtime to fetch repositories.
- The web interface uses Go's `net/http` and `html/template`; its page assets are embedded in the binary.
