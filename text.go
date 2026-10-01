package main

import (
	"strings"
	"unicode/utf8"
)

// renderMap renders the skill map as a text tree for the terminal, with similar skills grouped if grouping is set.
func renderMap(m skillMap, grouping bool) string {
	var b strings.Builder
	b.WriteString(m.summary() + "\n")
	if len(m.skills) == 0 {
		b.WriteString("\nNo SKILL.md files found.\n")
	}

	for _, group := range m.groups(grouping, nil) {
		b.WriteString("\n")
		if group.label != "" {
			b.WriteString(group.label + "\n")
		}
		for i, s := range group.skills {
			connector, indent := "├── ", "│   "
			if i == len(group.skills)-1 {
				connector, indent = "└── ", "    "
			}
			b.WriteString(connector + s.name + "\n")
			for _, line := range wrap(s.shownDescription(), 96) {
				b.WriteString(indent + line + "\n")
			}
		}
	}
	return b.String()
}

// renderOrgMap renders the organization's map as text for the terminal: a summary line, followed by the map of each
// repository with skills as renderMap renders it.
func renderOrgMap(m orgMap, grouping bool) string {
	var b strings.Builder
	b.WriteString(m.summary() + "\n")
	if len(m.maps) == 0 {
		b.WriteString("\nNo SKILL.md files found.\n")
	}
	for _, repo := range m.maps {
		b.WriteString("\n" + renderMap(repo, grouping))
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
