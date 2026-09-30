package main

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

// runCLI runs the tool with the given arguments and returns the process exit code.
func runCLI(args []string, stdout, stderr io.Writer) int {
	repo, ok := githubRepo{}, false
	if len(args) == 1 {
		repo, ok = parseGitHubRepo(args[0])
	}
	if !ok {
		fmt.Fprintln(stderr, "Usage: skill-atlas <github-repository-url>")
		return 2
	}

	fmt.Fprintf(stderr, "Analyzing %s ...\n", repo.webURL())
	if err := analyze(repo, stdout); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func analyze(repo githubRepo, stdout io.Writer) error {
	checkout, err := checkoutSkillFiles(repo.cloneURL())
	if err != nil {
		return err
	}
	defer checkout.close()

	groups, err := loadSkillGroups(checkout)
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, renderMap(repo, checkout, groups))
	return nil
}

func renderMap(repo githubRepo, checkout *repoCheckout, groups []skillGroup) string {
	var b strings.Builder
	count := 0
	for _, group := range groups {
		count += len(group.skills)
	}
	plural := "s"
	if count == 1 {
		plural = ""
	}
	fmt.Fprintf(&b, "%s · %s @ %.7s · %d skill%s\n", repo.slug(), cmp.Or(checkout.branch, "HEAD"), checkout.commit, count, plural)
	if len(groups) == 0 {
		b.WriteString("\nNo SKILL.md files found.\n")
	}

	for _, group := range groups {
		b.WriteString("\n")
		if group.directory == "" {
			b.WriteString("./\n")
		} else {
			b.WriteString(group.directory + "/\n")
		}
		for i, s := range group.skills {
			connector, indent := "├── ", "│   "
			if i == len(group.skills)-1 {
				connector, indent = "└── ", "    "
			}
			b.WriteString(connector + s.name + "\n")
			description := s.description
			if isBlank(description) {
				description = "(no description)"
			}
			for _, line := range wrap(description, 96) {
				b.WriteString(indent + line + "\n")
			}
		}
	}
	return b.String()
}

func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		if line != "" && utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
