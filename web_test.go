package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

var kotlin = githubRepo{"JetBrains", "kotlin"}

var exampleMap = skillMap{
	repo:   kotlin,
	branch: "master",
	commit: "c823f9e0123456789",
	groups: []skillGroup{
		{directory: "", skills: []skill{{name: "root-skill", description: "At the root."}}},
		{directory: ".claude/skills", skills: []skill{
			{name: "alpha", description: "Does alpha things.", path: ".claude/skills/alpha"},
			{name: "beta", path: ".claude/skills/beta"},
		}},
	},
}

// fakeScanner returns a fixed result and records which repositories were requested.
type fakeScanner struct {
	m         skillMap
	err       error
	requested []githubRepo
}

func (f *fakeScanner) scan(repo githubRepo) (skillMap, error) {
	f.requested = append(f.requested, repo)
	return f.m, f.err
}

// get sends a GET request for target to handler and returns the response.
func get(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

// assertInOrder checks that body contains every part, in the given order.
func assertInOrder(t *testing.T, body string, parts ...string) {
	t.Helper()
	rest := body
	for _, part := range parts {
		i := strings.Index(rest, part)
		if i < 0 {
			t.Errorf("page does not contain %q (after the previous parts):\n%s", part, body)
			return
		}
		rest = rest[i+len(part):]
	}
}

const usageHint = "Open <code>/scan?repo=&lt;github-repository-url&gt;</code>"

func TestStartPage(t *testing.T) {
	response := get(t, newWebHandler((&fakeScanner{}).scan), "/")

	if response.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	assertInOrder(t, response.Body.String(),
		"<title>Skill Atlas</title>",
		usageHint,
		`<a href="/scan?repo=https://github.com/JetBrains/kotlin">`,
	)
}

func TestUnknownPageIsNotFound(t *testing.T) {
	if response := get(t, newWebHandler((&fakeScanner{}).scan), "/missing"); response.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", response.Code)
	}
}

func TestScanPageShowsMap(t *testing.T) {
	scanner := &fakeScanner{m: exampleMap}
	response := get(t, newWebHandler(scanner.scan), "/scan?repo=https://github.com/JetBrains/kotlin")

	if response.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", response.Code)
	}
	if !slices.Equal(scanner.requested, []githubRepo{kotlin}) {
		t.Errorf("scanned %v; want [%v]", scanner.requested, kotlin)
	}
	body := response.Body.String()
	assertInOrder(t, body,
		"<title>JetBrains/kotlin · master @ c823f9e · 3 skills · Skill Atlas</title>",
		`<p class="summary">JetBrains/kotlin · master @ c823f9e · 3 skills</p>`,
		"<h2>./</h2>",
		"<li><strong>root-skill</strong><p>At the root.</p></li>",
		"<h2>.claude/skills/</h2>",
		"<li><strong>alpha</strong><p>Does alpha things.</p></li>",
		"<li><strong>beta</strong><p>(no description)</p></li>",
	)
	for _, unexpected := range []string{"No SKILL.md files found.", usageHint, `class="error"`} {
		if strings.Contains(body, unexpected) {
			t.Errorf("page contains %q:\n%s", unexpected, body)
		}
	}
}

func TestScanPageAcceptsRepositoryURLForms(t *testing.T) {
	for _, target := range []string{
		"/scan?repo=https://github.com/JetBrains/kotlin",
		"/scan?repo=https%3A%2F%2Fgithub.com%2FJetBrains%2Fkotlin",
		"/scan?repo=https://github.com/JetBrains/kotlin.git",
		"/scan?repo=https://github.com/JetBrains/kotlin/tree/master/compiler",
		"/scan?repo=https://github.com/JetBrains/kotlin%3Ftab%3Dreadme-ov-file",
		"/scan?repo=github.com/JetBrains/kotlin",
		"/scan?repo=git@github.com:JetBrains/kotlin.git",
		"/scan?other=1&repo=https://github.com/JetBrains/kotlin",
	} {
		scanner := &fakeScanner{m: exampleMap}
		response := get(t, newWebHandler(scanner.scan), target)
		if response.Code != http.StatusOK || !slices.Equal(scanner.requested, []githubRepo{kotlin}) {
			t.Errorf("GET %s: status %d, scanned %v; want 200 and [%v]", target, response.Code, scanner.requested, kotlin)
		}
	}
}

func TestScanPageWithoutSkills(t *testing.T) {
	scanner := &fakeScanner{m: skillMap{repo: kotlin, branch: "master", commit: "c823f9e0123456789"}}
	response := get(t, newWebHandler(scanner.scan), "/scan?repo=https://github.com/JetBrains/kotlin")

	if response.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", response.Code)
	}
	assertInOrder(t, response.Body.String(),
		`<p class="summary">JetBrains/kotlin · master @ c823f9e · 0 skills</p>`,
		"<p>No SKILL.md files found.</p>",
	)
}

func TestScanPageEscapesRepositoryContent(t *testing.T) {
	scanner := &fakeScanner{m: skillMap{repo: kotlin, commit: "c823f9e", groups: []skillGroup{
		{directory: "<i>dir</i>", skills: []skill{{name: "<b>name</b>", description: "<script>alert(1)</script>"}}},
	}}}
	body := get(t, newWebHandler(scanner.scan), "/scan?repo=github.com/JetBrains/kotlin").Body.String()

	assertInOrder(t, body,
		"<h2>&lt;i&gt;dir&lt;/i&gt;/</h2>",
		"<strong>&lt;b&gt;name&lt;/b&gt;</strong>",
		"<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>",
	)
	if strings.Contains(body, "<script>") {
		t.Errorf("page contains an unescaped script tag:\n%s", body)
	}
}

func TestScanPageRejectsInvalidRepository(t *testing.T) {
	tests := []struct{ target, message string }{
		{"/scan", "Missing the repo parameter."},
		{"/scan?repo=", "Missing the repo parameter."},
		{"/scan?repo=https://gitlab.com/owner/repo", "Not a GitHub repository URL: https://gitlab.com/owner/repo"},
		{"/scan?repo=%3Cb%3E", "Not a GitHub repository URL: &lt;b&gt;"},
	}
	for _, tt := range tests {
		scanner := &fakeScanner{m: exampleMap}
		response := get(t, newWebHandler(scanner.scan), tt.target)
		if response.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d; want 400", tt.target, response.Code)
		}
		if len(scanner.requested) != 0 {
			t.Errorf("GET %s: scanned %v; want no scan", tt.target, scanner.requested)
		}
		assertInOrder(t, response.Body.String(), `<p class="error">`+tt.message+"</p>", usageHint)
	}
}

func TestScanPageReportsScanErrors(t *testing.T) {
	scanner := &fakeScanner{err: errors.New("cannot access https://github.com/JetBrains/kotlin\ngit ls-remote failed: <fatal>")}
	response := get(t, newWebHandler(scanner.scan), "/scan?repo=https://github.com/JetBrains/kotlin")

	if response.Code != http.StatusBadGateway {
		t.Errorf("status = %d; want 502", response.Code)
	}
	assertInOrder(t, response.Body.String(),
		"<p class=\"error\">cannot access https://github.com/JetBrains/kotlin\ngit ls-remote failed: &lt;fatal&gt;</p>",
		usageHint,
	)
}

func TestScanPageRequiresGet(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/scan?repo=https://github.com/JetBrains/kotlin", nil)
	newWebHandler((&fakeScanner{m: exampleMap}).scan).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d; want 405", recorder.Code)
	}
}

func TestScanPageOfLocalRepository(t *testing.T) {
	remote, commit := newRemote(t, map[string]string{
		".claude/skills/review/SKILL.md": "---\nname: review\ndescription: Reviews a pull request.\n---\n",
		".claude/skills/debug/SKILL.md":  "---\nname: debug\n---\n",
	})
	scan := func(repo githubRepo) (skillMap, error) { return scanRemote(repo, remote) }
	response := get(t, newWebHandler(scan), "/scan?repo=https://github.com/owner/repo")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200:\n%s", response.Code, response.Body.String())
	}
	assertInOrder(t, response.Body.String(),
		`<p class="summary">owner/repo · main @ `+commit[:7]+` · 2 skills</p>`,
		"<h2>.claude/skills/</h2>",
		"<li><strong>debug</strong><p>(no description)</p></li>",
		"<li><strong>review</strong><p>Reviews a pull request.</p></li>",
	)
}

func TestServe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	served := make(chan error)
	go func() { served <- serve(listener, &stdout) }()

	response, err := http.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), usageHint) {
		t.Errorf("GET / = %d:\n%s\nwant 200 and the start page", response.StatusCode, body)
	}

	listener.Close()
	<-served
	if want := "Serving Skill Atlas at http://" + listener.Addr().String() + "\n"; stdout.String() != want {
		t.Errorf("stdout = %q; want %q", stdout.String(), want)
	}
}

func TestServeAddressIsLocalOnly(t *testing.T) {
	host, port, err := net.SplitHostPort(serveAddress)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Errorf("serveAddress host %q is not a loopback address", host)
	}
	if port != "8080" {
		t.Errorf("serveAddress port = %q; want 8080", port)
	}
}

func TestScanPageOfGitHubRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("needs access to github.com")
	}
	server := httptest.NewServer(newWebHandler(scanRepository))
	defer server.Close()

	response, err := http.Get(server.URL + "/scan?repo=https://github.com/JetBrains/kotlin")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.StatusCode, body)
	}
	assertInOrder(t, string(body), `<p class="summary">JetBrains/kotlin · `, "<h2>.claude/skills/</h2>", "<li><strong>")
}
