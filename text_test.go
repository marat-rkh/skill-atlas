package main

import (
	"slices"
	"strings"
	"testing"
)

var textMap = skillMap{
	repo:   githubRepo{"owner", "repo"},
	branch: "main",
	commit: "0123456789abcdef0123456789abcdef01234567",
	skills: []skill{
		{name: "analysis-api-a", description: strings.Repeat("word ", 30), path: ".claude/skills/analysis-api-a"},
		{name: "analysis-api-b", description: " ", path: ".claude/skills/analysis-api-b"},
		{name: "root-skill", description: "At the root."},
	},
}

func TestRenderMap(t *testing.T) {
	want := "owner/repo · main @ 0123456 · 3 skills\n" +
		"\n" +
		"├── analysis-api-a\n" +
		"│   " + words(19) + "\n" +
		"│   " + words(11) + "\n" +
		"├── analysis-api-b\n" +
		"│   (no description)\n" +
		"└── root-skill\n" +
		"    At the root.\n"
	if got := renderMap(textMap, false); got != want {
		t.Errorf("renderMap() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderMapWithGrouping(t *testing.T) {
	want := "owner/repo · main @ 0123456 · 3 skills\n" +
		"\n" +
		"analysis-api\n" +
		"├── analysis-api-a\n" +
		"│   " + words(19) + "\n" +
		"│   " + words(11) + "\n" +
		"└── analysis-api-b\n" +
		"    (no description)\n" +
		"\n" +
		"Other\n" +
		"└── root-skill\n" +
		"    At the root.\n"
	if got := renderMap(textMap, true); got != want {
		t.Errorf("renderMap() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderMapWithoutSkills(t *testing.T) {
	for _, grouping := range []bool{false, true} {
		got := renderMap(skillMap{repo: githubRepo{"owner", "repo"}, branch: "main", commit: "abcdef0123"}, grouping)
		if want := "owner/repo · main @ abcdef0 · 0 skills\n\nNo SKILL.md files found.\n"; got != want {
			t.Errorf("renderMap(grouping: %v) = %q; want %q", grouping, got, want)
		}
	}
}

func TestRenderMapOfLocalRepository(t *testing.T) {
	remote, commit := newRemote(t, map[string]string{
		".claude/skills/review/SKILL.md": "---\nname: review\ndescription: >\n  Reviews a pull\n  request.\n---\n",
		"tools/debug/SKILL.md":           "---\nname: debug\ndescription: \"Finds bugs.\"\n---\n",
		"src/main.go":                    "package main",
	})
	m, err := scanRemote(githubRepo{"owner", "repo"}, remote)
	if err != nil {
		t.Fatal(err)
	}

	want := "owner/repo · main @ " + commit[:7] + " · 2 skills\n" +
		"\n" +
		"├── debug\n" +
		"│   Finds bugs.\n" +
		"└── review\n" +
		"    Reviews a pull request.\n"
	if got := renderMap(m, false); got != want {
		t.Errorf("map =\n%s\nwant\n%s", got, want)
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"empty", "", 10, nil},
		{"only whitespace", " \n\t ", 10, nil},
		{"fits on one line", "one two", 10, []string{"one two"}},
		{"exactly the width", "one two", 7, []string{"one two"}},
		{"breaks between words", "one two three", 7, []string{"one two", "three"}},
		{"collapses whitespace", "one\n  two\t\tthree", 20, []string{"one two three"}},
		{"keeps words longer than the width", "tiny enormousword tiny", 5, []string{"tiny", "enormousword", "tiny"}},
		{"counts characters, not bytes", "héllo wörld", 11, []string{"héllo wörld"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wrap(tt.text, tt.width); !slices.Equal(got, tt.want) {
				t.Errorf("wrap(%q, %d) = %q; want %q", tt.text, tt.width, got, tt.want)
			}
		})
	}
}

func words(n int) string {
	return strings.TrimSpace(strings.Repeat("word ", n))
}
