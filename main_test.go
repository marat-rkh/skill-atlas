package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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
			"  skill-atlas --group <github-repository-url>   print the map with similar skills grouped\n" +
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

func TestRunCLIServeReportsUnreadableStars(t *testing.T) {
	path := filepath.Join(useTempConfigDir(t), "skill-atlas", "stars.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	// `serve` reads the stars once it listens, so the address must be free.
	listener, err := net.Listen("tcp", serveAddress)
	if err != nil {
		t.Skipf("%s is in use: %v", serveAddress, err)
	}
	listener.Close()

	var stdout, stderr bytes.Buffer
	exited := make(chan int)
	go func() { exited <- runCLI([]string{"serve"}, &stdout, &stderr) }()
	select {
	case code := <-exited:
		if code != 1 {
			t.Errorf("runCLI(serve) = %d; want 1", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runCLI(serve) is still serving")
	}
	if want := "error: cannot read stars from " + path + ": "; !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("stderr = %q; want prefix %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q; want nothing", stdout.String())
	}
	// The address is free again.
	listener, err = net.Listen("tcp", serveAddress)
	if err != nil {
		t.Errorf("serve did not stop listening: %v", err)
	} else {
		listener.Close()
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
