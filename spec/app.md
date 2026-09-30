# Overview

Skill Atlas is a tool that analyzes git repositories and generates a map of available agent skills.
The map can be presented in the terminal (CLI) or in a web browser (web interface).

# Input

Any GitHub repository URL.

# Output

A map of available agent skills.

# Presentation

Both presentation options show the same map; they differ only in how it is displayed.

## CLI

`skill-atlas <github-repository-url>` prints the map to the terminal.

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

# Implementation details

Discovered skills can be grouped thematically, but this is not required.

# Technologies

- Go, standard library only.
- The `git` command-line tool, used at runtime to fetch repositories.
- The web interface uses Go's `net/http` and `html/template`; its page assets are embedded in the binary.
