package main

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// skill is an agent skill: a directory with a SKILL.md file. path is the skill directory ("" for the repository root).
type skill struct {
	name, description, path string
}

// loadSkills reads the checkout's skills, sorted with compareSkills.
func loadSkills(checkout *repoCheckout) ([]skill, error) {
	skills := make([]skill, 0, len(checkout.skillFiles))
	for _, file := range checkout.skillFiles {
		content, err := checkout.read(file)
		if err != nil {
			return nil, err
		}
		skills = append(skills, parseSkill(parentDir(file), content))
	}
	slices.SortFunc(skills, compareSkills)
	return skills, nil
}

// compareSkills orders skills by name, and by path if names are equal.
func compareSkills(a, b skill) int {
	return cmp.Or(cmp.Compare(a.name, b.name), cmp.Compare(a.path, b.path))
}

func parseSkill(path, content string) skill {
	frontmatter := parseFrontmatter(content)
	name := frontmatter["name"]
	if isBlank(name) {
		name = cmp.Or(lastSegment(path), "(unnamed)")
	}
	return skill{name: name, description: frontmatter["description"], path: path}
}

var keyLine = regexp.MustCompile(`^([\w-]+):(.*)$`)

// parseFrontmatter reads the top-level scalar keys of a YAML frontmatter block. Nested structures are not interpreted.
func parseFrontmatter(content string) map[string]string {
	content = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(strings.TrimPrefix(content, "\uFEFF"))
	lines := strings.Split(content, "\n")
	isDelimiter := func(line string) bool { return strings.TrimRightFunc(line, unicode.IsSpace) == "---" }
	if !isDelimiter(lines[0]) {
		return nil
	}
	end := slices.IndexFunc(lines[1:], isDelimiter)
	if end < 0 {
		return nil
	}
	block := lines[1 : end+1]

	result := map[string]string{}
	for i := 0; i < len(block); {
		match := keyLine.FindStringSubmatch(block[i])
		i++
		if match == nil {
			continue
		}
		// Indented (or blank) lines that follow a key belong to its value.
		var continuation []string
		for i < len(block) && isIndentedOrBlank(block[i]) {
			continuation = append(continuation, block[i])
			i++
		}
		for len(continuation) > 0 && isBlank(continuation[len(continuation)-1]) {
			continuation = continuation[:len(continuation)-1]
		}
		result[match[1]] = scalarValue(strings.TrimSpace(match[2]), continuation)
	}
	return result
}

var escapeSequence = regexp.MustCompile(`\\(.)`)

func scalarValue(value string, continuation []string) string {
	if strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") {
		separator := " "
		if strings.HasPrefix(value, "|") {
			separator = "\n"
		}
		lines := make([]string, len(continuation))
		for i, line := range continuation {
			lines[i] = strings.TrimSpace(line)
		}
		return strings.TrimSpace(strings.Join(lines, separator))
	}

	var parts []string
	for _, line := range append([]string{value}, continuation...) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	text := strings.Join(parts, " ")
	switch {
	case len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"':
		return escapeSequence.ReplaceAllStringFunc(text[1:len(text)-1], func(sequence string) string {
			switch c := sequence[1:]; c {
			case "n":
				return "\n"
			case "t":
				return "\t"
			default:
				return c
			}
		})
	case len(text) >= 2 && text[0] == '\'' && text[len(text)-1] == '\'':
		return strings.ReplaceAll(text[1:len(text)-1], "''", "'")
	}
	return text
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

func isIndentedOrBlank(line string) bool {
	first, _ := utf8.DecodeRuneInString(line)
	return isBlank(line) || unicode.IsSpace(first)
}

// parentDir returns the part of a slash-separated path before its last slash, or "" if there is no slash.
func parentDir(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return ""
}

// lastSegment returns the part of a slash-separated path after its last slash.
func lastSegment(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}
