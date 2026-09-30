package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

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
