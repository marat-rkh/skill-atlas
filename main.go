package main

import (
	"cmp"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

func main() {
	repo, ok := githubRepo{}, false
	if len(os.Args) == 2 {
		repo, ok = parseGitHubRepo(os.Args[1])
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "Usage: skill-atlas <github-repository-url>")
		os.Exit(2)
	}

	fmt.Fprintf(os.Stderr, "Analyzing %s ...\n", repo.webURL())
	if err := run(repo); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(repo githubRepo) error {
	checkout, err := checkoutSkillFiles(repo)
	if err != nil {
		return err
	}
	defer checkout.close()

	groups, err := loadSkillGroups(checkout)
	if err != nil {
		return err
	}
	fmt.Print(renderMap(repo, checkout, groups))
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
