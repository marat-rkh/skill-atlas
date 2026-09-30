package main

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestRenderMap(t *testing.T) {
	checkout := &repoCheckout{branch: "main", commit: "0123456789abcdef0123456789abcdef01234567"}
	groups := []skillGroup{
		{directory: "", skills: []skill{{name: "root-skill", description: "At the root."}}},
		{directory: ".claude/skills", skills: []skill{
			{name: "alpha", description: strings.Repeat("word ", 30), path: ".claude/skills/alpha"},
			{name: "beta", description: " ", path: ".claude/skills/beta"},
		}},
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
	if got := renderMap(githubRepo{"owner", "repo"}, checkout, groups); got != want {
		t.Errorf("renderMap() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderMapHeader(t *testing.T) {
	oneSkill := []skillGroup{{directory: "skills", skills: []skill{{name: "x", description: "y", path: "skills/x"}}}}
	got := renderMap(githubRepo{"owner", "repo"}, &repoCheckout{commit: "abcdef0123"}, oneSkill)
	if want := "owner/repo · HEAD @ abcdef0 · 1 skill\n"; !strings.HasPrefix(got, want) {
		t.Errorf("renderMap() header = %q; want %q", strings.SplitAfter(got, "\n")[0], want)
	}
}

func TestRenderMapWithoutSkills(t *testing.T) {
	got := renderMap(githubRepo{"owner", "repo"}, &repoCheckout{branch: "main", commit: "abcdef0123"}, nil)
	if want := "owner/repo · main @ abcdef0 · 0 skills\n\nNo SKILL.md files found.\n"; got != want {
		t.Errorf("renderMap() = %q; want %q", got, want)
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

func TestMapOfLocalRepository(t *testing.T) {
	remote, commit := newRemote(t, map[string]string{
		".claude/skills/review/SKILL.md": "---\nname: review\ndescription: >\n  Reviews a pull\n  request.\n---\n",
		".claude/skills/debug/SKILL.md":  "---\nname: debug\ndescription: \"Finds bugs.\"\n---\n",
		"src/main.go":                    "package main",
	})
	checkout, err := checkoutSkillFiles(remote)
	if err != nil {
		t.Fatal(err)
	}
	defer checkout.close()
	groups, err := loadSkillGroups(checkout)
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
	if got := renderMap(githubRepo{"owner", "repo"}, checkout, groups); got != want {
		t.Errorf("map =\n%s\nwant\n%s", got, want)
	}
}

func TestRunCLIPrintsUsageForInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"not-a-url"},
		{"https://gitlab.com/owner/repo"},
		{"https://github.com/owner/repo", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runCLI(args, &stdout, &stderr); code != 2 {
			t.Errorf("runCLI(%q) = %d; want 2", args, code)
		}
		if stdout.Len() != 0 {
			t.Errorf("runCLI(%q) wrote to stdout: %q", args, stdout.String())
		}
		if want := "Usage: skill-atlas <github-repository-url>\n"; stderr.String() != want {
			t.Errorf("runCLI(%q) stderr = %q; want %q", args, stderr.String(), want)
		}
	}
}

func TestRunCLIMapsGitHubRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("needs access to github.com")
	}
	var stdout, stderr bytes.Buffer
	if code := runCLI([]string{"https://github.com/JetBrains/kotlin"}, &stdout, &stderr); code != 0 {
		t.Fatalf("runCLI() = %d; stderr:\n%s", code, stderr.String())
	}
	if want := "Analyzing https://github.com/JetBrains/kotlin ...\n"; stderr.String() != want {
		t.Errorf("stderr = %q; want %q", stderr.String(), want)
	}
	header := regexp.MustCompile(`^JetBrains/kotlin · \S+ @ [0-9a-f]{7} · [1-9]\d* skills?\n`)
	if !header.MatchString(stdout.String()) {
		t.Errorf("stdout does not start with a header listing skills:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "\n.claude/skills/\n") {
		t.Errorf("stdout has no .claude/skills/ group:\n%s", stdout.String())
	}
}

func TestRunCLIReportsMissingGitHubRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("needs access to github.com")
	}
	var stdout, stderr bytes.Buffer
	if code := runCLI([]string{"https://github.com/JetBrains/no-such-repository-for-skill-atlas"}, &stdout, &stderr); code != 1 {
		t.Errorf("runCLI() = %d; want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q; want nothing", stdout.String())
	}
	want := "error: cannot access https://github.com/JetBrains/no-such-repository-for-skill-atlas (repository not found or private)"
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q; want it to contain %q", stderr.String(), want)
	}
}

func words(n int) string {
	return strings.TrimSpace(strings.Repeat("word ", n))
}
