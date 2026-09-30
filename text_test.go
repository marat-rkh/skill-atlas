package main

import (
	"slices"
	"strings"
	"testing"
)

func TestRenderMap(t *testing.T) {
	m := skillMap{
		repo:   githubRepo{"owner", "repo"},
		branch: "main",
		commit: "0123456789abcdef0123456789abcdef01234567",
		groups: []skillGroup{
			{directory: "", skills: []skill{{name: "root-skill", description: "At the root."}}},
			{directory: ".claude/skills", skills: []skill{
				{name: "alpha", description: strings.Repeat("word ", 30), path: ".claude/skills/alpha"},
				{name: "beta", description: " ", path: ".claude/skills/beta"},
			}},
		},
	}

	want := "owner/repo · main @ 0123456 · 3 skills\n" +
		"\n" +
		"./\n" +
		"└── root-skill\n" +
		"    At the root.\n" +
		"\n" +
		".claude/skills/\n" +
		"├── alpha\n" +
		"│   " + words(19) + "\n" +
		"│   " + words(11) + "\n" +
		"└── beta\n" +
		"    (no description)\n"
	if got := renderMap(m); got != want {
		t.Errorf("renderMap() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderMapWithoutSkills(t *testing.T) {
	got := renderMap(skillMap{repo: githubRepo{"owner", "repo"}, branch: "main", commit: "abcdef0123"})
	if want := "owner/repo · main @ abcdef0 · 0 skills\n\nNo SKILL.md files found.\n"; got != want {
		t.Errorf("renderMap() = %q; want %q", got, want)
	}
}

func TestRenderMapOfLocalRepository(t *testing.T) {
	remote, commit := newRemote(t, map[string]string{
		".claude/skills/review/SKILL.md": "---\nname: review\ndescription: >\n  Reviews a pull\n  request.\n---\n",
		".claude/skills/debug/SKILL.md":  "---\nname: debug\ndescription: \"Finds bugs.\"\n---\n",
		"src/main.go":                    "package main",
	})
	m, err := scanRemote(githubRepo{"owner", "repo"}, remote)
	if err != nil {
		t.Fatal(err)
	}

	want := "owner/repo · main @ " + commit[:7] + " · 2 skills\n" +
		"\n" +
		".claude/skills/\n" +
		"├── debug\n" +
		"│   Finds bugs.\n" +
		"└── review\n" +
		"    Reviews a pull request.\n"
	if got := renderMap(m); got != want {
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
