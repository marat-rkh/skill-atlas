package main

import (
	"bytes"
	"errors"
	"net"
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
		{"serve", "extra"},
		{"--group"},
		{"--group", "not-a-url"},
		{"--group", "serve"},
		{"-group", "https://github.com/owner/repo"},
		{"https://github.com/owner/repo", "--group"},
		{"--group", "https://github.com/owner/repo", "extra"},
		{"https://github.com/"},
		{"https://github.com/orgs"},
		{"https://github.com/owner", "extra"},
		{"https://github.com/owner", "--group"},
		{"--group", "https://github.com/owner", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runCLI(args, &stdout, &stderr); code != 2 {
			t.Errorf("runCLI(%q) = %d; want 2", args, code)
		}
		if stdout.Len() != 0 {
			t.Errorf("runCLI(%q) wrote to stdout: %q", args, stdout.String())
		}
		want := "Usage:\n" +
			"  skill-atlas <github-repository-url>           print the map of the repository's agent skills\n" +
			"  skill-atlas <github-organization-url>         print the map of the agent skills in the organization's repositories\n" +
			"  skill-atlas --group <github-url>              print the map with similar skills grouped\n" +
			"  skill-atlas serve                             start the web interface at http://127.0.0.1:8080\n"
		if stderr.String() != want {
			t.Errorf("runCLI(%q) stderr = %q; want %q", args, stderr.String(), want)
		}
	}
}

func TestRunCLIServeReportsUnavailableAddress(t *testing.T) {
	// Hold the address so that `serve` cannot listen on it (if something else holds it, the outcome is the same).
	if listener, err := net.Listen("tcp", serveAddress); err == nil {
		defer listener.Close()
	}

	var stdout, stderr bytes.Buffer
	if code := runCLI([]string{"serve"}, &stdout, &stderr); code != 1 {
		t.Errorf("runCLI(serve) = %d; want 1", code)
	}
	if want := "error: listen tcp " + serveAddress + ": "; !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("stderr = %q; want prefix %q", stderr.String(), want)
	}
}

func TestRunCLIMapsGitHubRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("needs access to github.com")
	}
	header := regexp.MustCompile(`^JetBrains/kotlin · \S+ @ [0-9a-f]{7} · [1-9]\d* skills?\n`)
	// Every line after the header is blank, part of the skill tree, or (only with grouping) a group label.
	groupLabel := regexp.MustCompile(`\n[^├└│ \n]`)
	for _, grouping := range []bool{false, true} {
		args := []string{"https://github.com/JetBrains/kotlin"}
		if grouping {
			args = append([]string{"--group"}, args...)
		}
		var stdout, stderr bytes.Buffer
		if code := runCLI(args, &stdout, &stderr); code != 0 {
			t.Fatalf("runCLI(%q) = %d; stderr:\n%s", args, code, stderr.String())
		}
		if want := "Analyzing https://github.com/JetBrains/kotlin ...\n"; stderr.String() != want {
			t.Errorf("runCLI(%q) stderr = %q; want %q", args, stderr.String(), want)
		}
		if !header.MatchString(stdout.String()) {
			t.Errorf("runCLI(%q) stdout does not start with a header listing skills:\n%s", args, stdout.String())
		}
		body := header.ReplaceAllString(stdout.String(), "")
		if hasLabels := groupLabel.MatchString(body); hasLabels != grouping {
			t.Errorf("runCLI(%q) stdout has group labels: %v; want %v:\n%s", args, hasLabels, grouping, stdout.String())
		}
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

func TestPrintOrgMap(t *testing.T) {
	m := orgMap{org: githubOrg{"owner"}, repoCount: 3, maps: []skillMap{textMap}}
	for _, grouping := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		if code := printOrgMap(m, grouping, &stdout, &stderr); code != 0 {
			t.Errorf("printOrgMap(grouping: %v) = %d; want 0", grouping, code)
		}
		if want := renderOrgMap(m, grouping); stdout.String() != want {
			t.Errorf("printOrgMap(grouping: %v) stdout =\n%s\nwant\n%s", grouping, stdout.String(), want)
		}
		if stderr.Len() != 0 {
			t.Errorf("printOrgMap(grouping: %v) stderr = %q; want nothing", grouping, stderr.String())
		}
	}
}

func TestPrintOrgMapReportsFailedRepositories(t *testing.T) {
	m := orgMap{org: githubOrg{"owner"}, repoCount: 3, maps: []skillMap{textMap}, failures: []error{
		errors.New("owner/a: cannot access https://github.com/owner/a (repository not found or private)\ngit ls-remote failed: fatal"),
		errors.New("owner/b: git fetch failed: fatal"),
	}}
	var stdout, stderr bytes.Buffer
	if code := printOrgMap(m, false, &stdout, &stderr); code != 1 {
		t.Errorf("printOrgMap() = %d; want 1", code)
	}
	if want := renderOrgMap(m, false); stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant the map of the other repositories\n%s", stdout.String(), want)
	}
	want := "error: owner/a: cannot access https://github.com/owner/a (repository not found or private)\ngit ls-remote failed: fatal\n" +
		"error: owner/b: git fetch failed: fatal\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q; want %q", stderr.String(), want)
	}
}

func TestRunCLIMapsGitHubOrganization(t *testing.T) {
	if testing.Short() {
		t.Skip("needs access to github.com")
	}
	// anthropics has a few dozen repositories, some of them with skills, and an empty one.
	var stdout, stderr bytes.Buffer
	args := []string{"https://github.com/anthropics"}
	if code := runCLI(args, &stdout, &stderr); code != 0 {
		t.Fatalf("runCLI(%q) = %d; stderr:\n%s", args, code, stderr.String())
	}
	if want := "Analyzing https://github.com/anthropics ...\n"; stderr.String() != want {
		t.Errorf("stderr = %q; want %q", stderr.String(), want)
	}
	header := regexp.MustCompile(`^anthropics · [1-9]\d* repositories · [1-9]\d* with skills · [1-9]\d* skills\n\n`)
	repo := regexp.MustCompile(`(?m)^anthropics/skills · \S+ @ [0-9a-f]{7} · [1-9]\d* skills?\n\n[├└]── `)
	if !header.MatchString(stdout.String()) || !repo.MatchString(stdout.String()) {
		t.Errorf("stdout does not start with a header and include the map of anthropics/skills:\n%s", stdout.String())
	}
}

func TestRunCLIReportsMissingGitHubOrganization(t *testing.T) {
	if testing.Short() {
		t.Skip("needs access to github.com")
	}
	var stdout, stderr bytes.Buffer
	if code := runCLI([]string{"https://github.com/no-such-organization-for-skill-atlas"}, &stdout, &stderr); code != 1 {
		t.Errorf("runCLI() = %d; want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q; want nothing", stdout.String())
	}
	want := "Analyzing https://github.com/no-such-organization-for-skill-atlas ...\n" +
		"error: cannot access https://github.com/no-such-organization-for-skill-atlas (organization or user not found)\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q; want %q", stderr.String(), want)
	}
}
