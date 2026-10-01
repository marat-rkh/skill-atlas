package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

var kotlin = githubRepo{"JetBrains", "kotlin"}

var exampleMap = skillMap{
	repo:   kotlin,
	branch: "master",
	commit: "c823f9e0123456789",
	skills: []skill{
		{name: "alpha-one", description: "Does alpha things.", path: ".claude/skills/alpha-one"},
		{name: "alpha-two", path: ".claude/skills/alpha-two"},
		{name: "root-skill", description: "At the root."},
	},
}

var jetbrains = githubOrg{"JetBrains"}

var exampleOrgMap = orgMap{
	org:       jetbrains,
	repoCount: 5,
	maps: []skillMap{
		{repo: githubRepo{"JetBrains", "Exposed"}, branch: "main", commit: "abcdef0123456789", skills: []skill{
			{name: "alpha-sql", description: "Writes SQL."},
			{name: "dao", description: "Works with DAOs."},
		}},
		exampleMap,
	},
}

// fakeScanner returns fixed results and records which repositories and organizations were requested.
type fakeScanner struct {
	m             skillMap
	org           orgMap
	err           error
	requested     []githubRepo
	requestedOrgs []githubOrg
}

func (f *fakeScanner) scan(repo githubRepo) (skillMap, error) {
	f.requested = append(f.requested, repo)
	return f.m, f.err
}

func (f *fakeScanner) scanOrg(org githubOrg) (orgMap, error) {
	f.requestedOrgs = append(f.requestedOrgs, org)
	return f.org, f.err
}

// handler returns the web handler with the scanner's results.
func (f *fakeScanner) handler() http.Handler {
	return newWebHandler(f.scan, f.scanOrg)
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

const orgUsageHint = "Open <code>/scan?org=&lt;github-organization-url&gt;</code>"

const filterForm = `<form action="/scan" method="get" role="search">`

// groupingCheckbox is the start of the grouping checkbox, which is in the filter form.
const groupingCheckbox = `<label><input type="checkbox" name="group"`

func TestStartPage(t *testing.T) {
	response := get(t, (&fakeScanner{}).handler(), "/")

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
		orgUsageHint,
		`<a href="/scan?org=https://github.com/JetBrains">`,
	)
}

func TestUnknownPageIsNotFound(t *testing.T) {
	if response := get(t, (&fakeScanner{}).handler(), "/missing"); response.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", response.Code)
	}
}

func TestScanPageShowsMap(t *testing.T) {
	scanner := &fakeScanner{m: exampleMap}
	response := get(t, scanner.handler(), "/scan?repo=https://github.com/JetBrains/kotlin")

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
		filterForm,
		`<input type="hidden" name="repo" value="https://github.com/JetBrains/kotlin">`,
		`<input type="search" name="filter" value="">`,
		groupingCheckbox+` onchange="this.form.submit()"> Group similar skills</label>`,
		"</form>",
		"<section>\n  <ul>",
		"<li><strong>alpha-one</strong><p>Does alpha things.</p></li>",
		"<li><strong>alpha-two</strong><p>(no description)</p></li>",
		"<li><strong>root-skill</strong><p>At the root.</p></li>",
		"</ul>\n</section>",
	)
	for _, unexpected := range []string{"<h2>", "No SKILL.md files found.", "No skills match", `class="matches"`, usageHint, `class="error"`} {
		if strings.Contains(body, unexpected) {
			t.Errorf("page contains %q:\n%s", unexpected, body)
		}
	}
}

func TestScanPageGroupsSimilarSkills(t *testing.T) {
	scanner := &fakeScanner{m: exampleMap}
	response := get(t, scanner.handler(), "/scan?repo=https://github.com/JetBrains/kotlin&group=on")

	if response.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", response.Code)
	}
	if !slices.Equal(scanner.requested, []githubRepo{kotlin}) {
		t.Errorf("scanned %v; want [%v]", scanner.requested, kotlin)
	}
	assertInOrder(t, response.Body.String(),
		`<p class="summary">JetBrains/kotlin · master @ c823f9e · 3 skills</p>`,
		groupingCheckbox+` checked onchange="this.form.submit()"> Group similar skills</label>`,
		"<h2>alpha</h2>",
		"<li><strong>alpha-one</strong><p>Does alpha things.</p></li>",
		"<li><strong>alpha-two</strong><p>(no description)</p></li>",
		"<h2>Other</h2>",
		"<li><strong>root-skill</strong><p>At the root.</p></li>",
	)
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
		response := get(t, scanner.handler(), target)
		if response.Code != http.StatusOK || !slices.Equal(scanner.requested, []githubRepo{kotlin}) {
			t.Errorf("GET %s: status %d, scanned %v; want 200 and [%v]", target, response.Code, scanner.requested, kotlin)
		}
	}
}

func TestScanPageWithoutSkills(t *testing.T) {
	scanner := &fakeScanner{m: skillMap{repo: kotlin, branch: "master", commit: "c823f9e0123456789"}}
	response := get(t, scanner.handler(), "/scan?repo=https://github.com/JetBrains/kotlin")

	if response.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", response.Code)
	}
	assertInOrder(t, response.Body.String(),
		`<p class="summary">JetBrains/kotlin · master @ c823f9e · 0 skills</p>`,
		"<p>No SKILL.md files found.</p>",
	)
}

func TestScanPageFiltersSkills(t *testing.T) {
	tests := []struct {
		name, filter, group, matchCount string
		shown, hidden                   []string
	}{
		// Groups are formed from the whole map, so alpha-one stays in the alpha group without alpha-two.
		{"by name", "ONE", "&group=on", `1 of 3 skills matches "ONE"`,
			[]string{"<h2>alpha</h2>", "<strong>alpha-one</strong>"},
			[]string{"<h2>Other</h2>", "<strong>root-skill</strong>", "<strong>alpha-two</strong>"}},
		{"by name without grouping", "ONE", "", `1 of 3 skills matches "ONE"`,
			[]string{"<strong>alpha-one</strong>"},
			[]string{"<h2>", "<strong>root-skill</strong>", "<strong>alpha-two</strong>"}},
		{"by description", "the ROOT", "&group=on", `1 of 3 skills matches "the ROOT"`,
			[]string{"<h2>Other</h2>", "<strong>root-skill</strong>"},
			[]string{"<h2>alpha</h2>", "<strong>alpha-one</strong>", "<strong>alpha-two</strong>"}},
		{"ignoring surrounding whitespace", "  a  ", "", `3 of 3 skills match "a"`,
			[]string{"<strong>alpha-one</strong>", "<strong>alpha-two</strong>", "<strong>root-skill</strong>"}, nil},
		{"not by the placeholder of a missing description", "no description", "&group=on", `0 of 3 skills match "no description"`,
			[]string{"<p>No skills match the filter.</p>"},
			[]string{"<h2>", "<strong>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := "/scan?repo=https://github.com/JetBrains/kotlin&filter=" + url.QueryEscape(tt.filter) + tt.group
			response := get(t, (&fakeScanner{m: exampleMap}).handler(), target)

			if response.Code != http.StatusOK {
				t.Errorf("status = %d; want 200", response.Code)
			}
			body := response.Body.String()
			assertInOrder(t, body,
				"<title>JetBrains/kotlin · master @ c823f9e · 3 skills · Skill Atlas</title>",
				`<p class="summary">JetBrains/kotlin · master @ c823f9e · 3 skills</p>`,
				filterForm,
				`<input type="search" name="filter" value="`+strings.TrimSpace(tt.filter)+`">`,
				`<p class="matches">`+strings.ReplaceAll(tt.matchCount, `"`, "&#34;")+"</p>",
			)
			assertInOrder(t, body, tt.shown...)
			for _, hidden := range tt.hidden {
				if strings.Contains(body, hidden) {
					t.Errorf("page contains %q:\n%s", hidden, body)
				}
			}
			if tt.matchCount[0] != '0' && strings.Contains(body, "No skills match") {
				t.Errorf("page says no skills match:\n%s", body)
			}
		})
	}
}

func TestScanPageWithEmptyFilterShowsWholeMap(t *testing.T) {
	for _, filter := range []string{"", "%20%20"} {
		target := "/scan?repo=https://github.com/JetBrains/kotlin&filter=" + filter
		body := get(t, (&fakeScanner{m: exampleMap}).handler(), target).Body.String()

		assertInOrder(t, body, `<input type="search" name="filter" value="">`,
			"<strong>alpha-one</strong>", "<strong>alpha-two</strong>", "<strong>root-skill</strong>")
		for _, unexpected := range []string{`class="matches"`, "No skills match"} {
			if strings.Contains(body, unexpected) {
				t.Errorf("GET %s: page contains %q:\n%s", target, unexpected, body)
			}
		}
	}
}

func TestScanPageFilterWithoutSkills(t *testing.T) {
	scanner := &fakeScanner{m: skillMap{repo: kotlin, branch: "master", commit: "c823f9e0123456789"}}
	body := get(t, scanner.handler(), "/scan?repo=https://github.com/JetBrains/kotlin&filter=x").Body.String()

	assertInOrder(t, body, filterForm, `<p class="matches">0 of 0 skills match &#34;x&#34;</p>`, "<p>No SKILL.md files found.</p>")
	if strings.Contains(body, "No skills match") {
		t.Errorf("page says no skills match:\n%s", body)
	}
}

func TestScanPageFilterFormKeepsRepository(t *testing.T) {
	// The form must submit the scanned repository, even if it was requested in another URL form.
	for _, repo := range []string{"git@github.com:JetBrains/kotlin.git", "https://github.com/JetBrains/kotlin/tree/master/compiler"} {
		target := "/scan?repo=" + url.QueryEscape(repo) + "&filter=alpha"
		body := get(t, (&fakeScanner{m: exampleMap}).handler(), target).Body.String()
		assertInOrder(t, body, filterForm,
			`<input type="hidden" name="repo" value="https://github.com/JetBrains/kotlin">`,
			`<input type="search" name="filter" value="alpha">`,
			`<button type="submit">Filter</button>`,
			"</form>",
		)
	}
}

func TestScanPageFormKeepsFilterAndGrouping(t *testing.T) {
	body := get(t, (&fakeScanner{m: exampleMap}).handler(),
		"/scan?repo=https://github.com/JetBrains/kotlin&filter=alpha&group=on").Body.String()
	assertInOrder(t, body, filterForm,
		`<input type="search" name="filter" value="alpha">`,
		groupingCheckbox+` checked onchange="this.form.submit()"> Group similar skills</label>`,
		"</form>",
		"<h2>alpha</h2>",
	)
}

func TestScanPageEscapesFilter(t *testing.T) {
	target := "/scan?repo=https://github.com/JetBrains/kotlin&filter=" + url.QueryEscape(`"><script>alert(1)</script>`)
	body := get(t, (&fakeScanner{m: exampleMap}).handler(), target).Body.String()

	assertInOrder(t, body,
		`<input type="search" name="filter" value="&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;">`,
		`<p class="matches">0 of 3 skills match &#34;&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;&#34;</p>`,
	)
	if strings.Contains(body, "<script>") {
		t.Errorf("page contains an unescaped script tag:\n%s", body)
	}
}

func TestMatchCount(t *testing.T) {
	tests := []struct {
		matches, total int
		want           string
	}{
		{0, 0, `0 of 0 skills match "x"`},
		{0, 1, `0 of 1 skill match "x"`},
		{1, 1, `1 of 1 skill matches "x"`},
		{1, 2, `1 of 2 skills matches "x"`},
		{2, 6, `2 of 6 skills match "x"`},
	}
	for _, tt := range tests {
		if got := (pageData{Filter: "x", Matches: tt.matches, Total: tt.total}).MatchCount(); got != tt.want {
			t.Errorf("MatchCount() with %d of %d = %q; want %q", tt.matches, tt.total, got, tt.want)
		}
	}
}

func TestScanPageEscapesRepositoryContent(t *testing.T) {
	scanner := &fakeScanner{m: skillMap{repo: kotlin, commit: "c823f9e", skills: []skill{
		{name: "<b>name</b>-1", description: "<script>alert(1)</script>"},
		{name: "<b>name</b>-2"},
	}}}
	body := get(t, scanner.handler(), "/scan?repo=github.com/JetBrains/kotlin&group=on").Body.String()

	assertInOrder(t, body,
		"<h2>&lt;b&gt;name&lt;/b&gt;</h2>",
		"<strong>&lt;b&gt;name&lt;/b&gt;-1</strong>",
		"<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>",
	)
	if strings.Contains(body, "<script>") {
		t.Errorf("page contains an unescaped script tag:\n%s", body)
	}
}

func TestScanPageRejectsInvalidRepository(t *testing.T) {
	tests := []struct{ target, message string }{
		{"/scan", "Missing the repo or org parameter."},
		{"/scan?repo=", "Missing the repo or org parameter."},
		{"/scan?repo=https://gitlab.com/owner/repo", "Not a GitHub repository URL: https://gitlab.com/owner/repo"},
		{"/scan?repo=%3Cb%3E", "Not a GitHub repository URL: &lt;b&gt;"},
	}
	for _, tt := range tests {
		scanner := &fakeScanner{m: exampleMap}
		response := get(t, scanner.handler(), tt.target)
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
	response := get(t, scanner.handler(), "/scan?repo=https://github.com/JetBrains/kotlin")

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
	(&fakeScanner{m: exampleMap}).handler().ServeHTTP(recorder, request)
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
	response := get(t, newWebHandler(scan, (&fakeScanner{}).scanOrg), "/scan?repo=https://github.com/owner/repo")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200:\n%s", response.Code, response.Body.String())
	}
	assertInOrder(t, response.Body.String(),
		`<p class="summary">owner/repo · main @ `+commit[:7]+` · 2 skills</p>`,
		`<input type="hidden" name="repo" value="https://github.com/owner/repo">`,
		"<li><strong>debug</strong><p>(no description)</p></li>",
		"<li><strong>review</strong><p>Reviews a pull request.</p></li>",
	)

	response = get(t, newWebHandler(scan, (&fakeScanner{}).scanOrg), "/scan?repo=https://github.com/owner/repo&filter=pull")
	body := response.Body.String()
	assertInOrder(t, body, `<p class="matches">1 of 2 skills matches &#34;pull&#34;</p>`,
		"<li><strong>review</strong><p>Reviews a pull request.</p></li>")
	if strings.Contains(body, "<strong>debug</strong>") {
		t.Errorf("filtered page contains the debug skill:\n%s", body)
	}
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
	server := httptest.NewServer(newWebHandler(scanRepository, scanOrganization))
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
	assertInOrder(t, string(body), `<p class="summary">JetBrains/kotlin · `, filterForm, groupingCheckbox, "<li><strong>")
}

func TestScanPageShowsOrganizationMap(t *testing.T) {
	scanner := &fakeScanner{org: exampleOrgMap}
	response := get(t, scanner.handler(), "/scan?org=https://github.com/JetBrains")

	if response.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", response.Code)
	}
	if !slices.Equal(scanner.requestedOrgs, []githubOrg{jetbrains}) || len(scanner.requested) != 0 {
		t.Errorf("scanned %v and %v; want only [%v]", scanner.requestedOrgs, scanner.requested, jetbrains)
	}
	body := response.Body.String()
	assertInOrder(t, body,
		"<title>JetBrains · 5 repositories · 2 with skills · 5 skills · Skill Atlas</title>",
		`<p class="summary">JetBrains · 5 repositories · 2 with skills · 5 skills</p>`,
		filterForm,
		`<input type="hidden" name="org" value="https://github.com/JetBrains">`,
		`<input type="search" name="filter" value="">`,
		groupingCheckbox+` onchange="this.form.submit()"> Group similar skills</label>`,
		"</form>",
		`<section class="repo">`+"\n  <h2>JetBrains/Exposed · main @ abcdef0 · 2 skills</h2>\n  <section>\n    <ul>",
		"<li><strong>alpha-sql</strong><p>Writes SQL.</p></li>",
		"<li><strong>dao</strong><p>Works with DAOs.</p></li>",
		"</ul>\n  </section>\n</section>",
		`<section class="repo">`+"\n  <h2>JetBrains/kotlin · master @ c823f9e · 3 skills</h2>",
		"<li><strong>alpha-one</strong><p>Does alpha things.</p></li>",
		"<li><strong>alpha-two</strong><p>(no description)</p></li>",
		"<li><strong>root-skill</strong><p>At the root.</p></li>",
		"</section>\n</body>",
	)
	for _, unexpected := range []string{`name="repo"`, "<h3>", "No SKILL.md files found.", "No skills match", `class="matches"`, orgUsageHint, `class="error"`} {
		if strings.Contains(body, unexpected) {
			t.Errorf("page contains %q:\n%s", unexpected, body)
		}
	}
}

func TestScanPageGroupsOrganizationSkills(t *testing.T) {
	response := get(t, (&fakeScanner{org: exampleOrgMap}).handler(), "/scan?org=https://github.com/JetBrains&group=on")

	if response.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", response.Code)
	}
	// Skills are grouped within each repository.
	assertInOrder(t, response.Body.String(),
		groupingCheckbox+` checked onchange="this.form.submit()"> Group similar skills</label>`,
		"<h2>JetBrains/Exposed · main @ abcdef0 · 2 skills</h2>",
		"<h3>Other</h3>",
		"<li><strong>alpha-sql</strong><p>Writes SQL.</p></li>",
		"<li><strong>dao</strong><p>Works with DAOs.</p></li>",
		"<h2>JetBrains/kotlin · master @ c823f9e · 3 skills</h2>",
		"<h3>alpha</h3>",
		"<li><strong>alpha-one</strong><p>Does alpha things.</p></li>",
		"<li><strong>alpha-two</strong><p>(no description)</p></li>",
		"<h3>Other</h3>",
		"<li><strong>root-skill</strong><p>At the root.</p></li>",
	)
}

func TestScanPageFiltersOrganizationSkills(t *testing.T) {
	tests := []struct {
		name, filter, group, matchCount string
		shown, hidden                   []string
	}{
		{"in several repositories", "ALPHA", "", `3 of 5 skills match "ALPHA"`,
			[]string{"<h2>JetBrains/Exposed", "<strong>alpha-sql</strong>", "<h2>JetBrains/kotlin", "<strong>alpha-one</strong>", "<strong>alpha-two</strong>"},
			[]string{"<strong>dao</strong>", "<strong>root-skill</strong>"}},
		{"hiding repositories without matches", "dao", "&group=on", `1 of 5 skills matches "dao"`,
			[]string{"<h2>JetBrains/Exposed", "<h3>Other</h3>", "<strong>dao</strong>"},
			[]string{"<h2>JetBrains/kotlin", "<strong>alpha-sql</strong>", "<strong>alpha-one</strong>"}},
		{"with no matches", "nothing", "", `0 of 5 skills match "nothing"`,
			[]string{"<p>No skills match the filter.</p>"},
			[]string{"<h2>", "<strong>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := "/scan?org=https://github.com/JetBrains&filter=" + url.QueryEscape(tt.filter) + tt.group
			body := get(t, (&fakeScanner{org: exampleOrgMap}).handler(), target).Body.String()

			assertInOrder(t, body,
				`<p class="summary">JetBrains · 5 repositories · 2 with skills · 5 skills</p>`,
				`<input type="hidden" name="org" value="https://github.com/JetBrains">`,
				`<input type="search" name="filter" value="`+tt.filter+`">`,
				`<p class="matches">`+strings.ReplaceAll(tt.matchCount, `"`, "&#34;")+"</p>",
			)
			assertInOrder(t, body, tt.shown...)
			for _, hidden := range tt.hidden {
				if strings.Contains(body, hidden) {
					t.Errorf("page contains %q:\n%s", hidden, body)
				}
			}
			if tt.matchCount[0] != '0' && strings.Contains(body, "No skills match") {
				t.Errorf("page says no skills match:\n%s", body)
			}
		})
	}
}

func TestScanPageOfOrganizationWithoutSkills(t *testing.T) {
	scanner := &fakeScanner{org: orgMap{org: jetbrains, repoCount: 3}}
	body := get(t, scanner.handler(), "/scan?org=https://github.com/JetBrains&filter=x").Body.String()

	assertInOrder(t, body,
		`<p class="summary">JetBrains · 3 repositories · 0 with skills · 0 skills</p>`,
		`<p class="matches">0 of 0 skills match &#34;x&#34;</p>`,
		"<p>No SKILL.md files found.</p>",
	)
	if strings.Contains(body, "No skills match") {
		t.Errorf("page says no skills match:\n%s", body)
	}
}

func TestScanPageAcceptsOrganizationURLForms(t *testing.T) {
	for _, target := range []string{
		"/scan?org=https://github.com/JetBrains",
		"/scan?org=https%3A%2F%2Fgithub.com%2FJetBrains",
		"/scan?org=https://github.com/JetBrains/",
		"/scan?org=https://github.com/JetBrains%3Ftab%3Drepositories",
		"/scan?org=https://github.com/orgs/JetBrains/repositories",
		"/scan?org=github.com/JetBrains",
		"/scan?repo=&org=https://github.com/JetBrains",
	} {
		scanner := &fakeScanner{org: exampleOrgMap}
		response := get(t, scanner.handler(), target)
		if response.Code != http.StatusOK || !slices.Equal(scanner.requestedOrgs, []githubOrg{jetbrains}) {
			t.Errorf("GET %s: status %d, scanned %v; want 200 and [%v]", target, response.Code, scanner.requestedOrgs, jetbrains)
		}
	}
}

func TestScanPageFormKeepsOrganization(t *testing.T) {
	// The form must submit the scanned organization, even if it was requested in another URL form.
	target := "/scan?org=" + url.QueryEscape("https://github.com/orgs/JetBrains/repositories") + "&filter=alpha&group=on"
	body := get(t, (&fakeScanner{org: exampleOrgMap}).handler(), target).Body.String()
	assertInOrder(t, body, filterForm,
		`<input type="hidden" name="org" value="https://github.com/JetBrains">`,
		`<input type="search" name="filter" value="alpha">`,
		groupingCheckbox+` checked onchange="this.form.submit()"> Group similar skills</label>`,
		"</form>",
	)
}

func TestScanPageShowsOrganizationFailures(t *testing.T) {
	m := exampleOrgMap
	m.failures = []error{
		errors.New("JetBrains/a: cannot access https://github.com/JetBrains/a (repository not found or private)\ngit ls-remote failed: <fatal>"),
		errors.New("JetBrains/b: git fetch failed: fatal"),
	}
	response := get(t, (&fakeScanner{org: m}).handler(), "/scan?org=https://github.com/JetBrains")

	// The map of the other repositories is still shown.
	if response.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", response.Code)
	}
	assertInOrder(t, response.Body.String(),
		"<p class=\"error\">JetBrains/a: cannot access https://github.com/JetBrains/a (repository not found or private)\ngit ls-remote failed: &lt;fatal&gt;</p>",
		`<p class="error">JetBrains/b: git fetch failed: fatal</p>`,
		`<p class="summary">JetBrains · 5 repositories · 2 with skills · 5 skills</p>`,
		"<h2>JetBrains/Exposed · main @ abcdef0 · 2 skills</h2>",
		"<h2>JetBrains/kotlin · master @ c823f9e · 3 skills</h2>",
	)
}

func TestCachingWebHandlerKeepsMaps(t *testing.T) {
	partial := exampleOrgMap
	partial.failures = []error{errors.New("JetBrains/a: git fetch failed: fatal")}
	tests := []struct {
		name     string
		org      orgMap
		orgScans int
	}{
		{"complete organization", exampleOrgMap, 1},
		// Opening the map of an organization with failed repositories again retries them.
		{"organization with failed repositories", partial, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := &fakeScanner{m: exampleMap, org: tt.org}
			handler := newCachingWebHandler(scanner.scan, scanner.scanOrg)
			for _, target := range []string{"/scan?repo=https://github.com/JetBrains/kotlin", "/scan?org=https://github.com/JetBrains"} {
				for _, options := range []string{"", "&filter=alpha", "&group=on"} {
					if response := get(t, handler, target+options); response.Code != http.StatusOK {
						t.Errorf("GET %s = %d; want 200", target+options, response.Code)
					}
				}
			}
			if len(scanner.requested) != 1 {
				t.Errorf("scanned the repository %d times; want 1", len(scanner.requested))
			}
			if len(scanner.requestedOrgs) != tt.orgScans {
				t.Errorf("scanned the organization %d times; want %d", len(scanner.requestedOrgs), tt.orgScans)
			}
		})
	}
}

func TestScanPageRejectsInvalidOrganization(t *testing.T) {
	tests := []struct{ target, message string }{
		{"/scan?org=https://github.com/JetBrains/kotlin", "Not a GitHub organization URL: https://github.com/JetBrains/kotlin"},
		{"/scan?org=https://gitlab.com/JetBrains", "Not a GitHub organization URL: https://gitlab.com/JetBrains"},
		{"/scan?org=%3Cb%3E", "Not a GitHub organization URL: &lt;b&gt;"},
		{"/scan?org=https://github.com/JetBrains&repo=https://github.com/JetBrains/kotlin", "Pass either the repo or the org parameter, not both."},
		{"/scan?repo=https://github.com/JetBrains", "Not a GitHub repository URL: https://github.com/JetBrains"},
	}
	for _, tt := range tests {
		scanner := &fakeScanner{m: exampleMap, org: exampleOrgMap}
		response := get(t, scanner.handler(), tt.target)
		if response.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d; want 400", tt.target, response.Code)
		}
		if len(scanner.requested) != 0 || len(scanner.requestedOrgs) != 0 {
			t.Errorf("GET %s: scanned %v and %v; want no scan", tt.target, scanner.requested, scanner.requestedOrgs)
		}
		assertInOrder(t, response.Body.String(), `<p class="error">`+tt.message+"</p>", usageHint, orgUsageHint)
	}
}

func TestScanPageReportsOrganizationScanErrors(t *testing.T) {
	scanner := &fakeScanner{err: errors.New("cannot access https://github.com/JetBrains (organization or user not found)")}
	response := get(t, scanner.handler(), "/scan?org=https://github.com/JetBrains")

	if response.Code != http.StatusBadGateway {
		t.Errorf("status = %d; want 502", response.Code)
	}
	assertInOrder(t, response.Body.String(),
		`<p class="error">cannot access https://github.com/JetBrains (organization or user not found)</p>`,
		usageHint,
	)
}

func TestScanPageOfLocalOrganization(t *testing.T) {
	review, _ := newRemote(t, map[string]string{".claude/skills/review/SKILL.md": "---\nname: review\ndescription: Reviews a pull request.\n---\n"})
	tools, commit := newRemote(t, map[string]string{"debug/SKILL.md": "---\nname: debug\n---\n"})
	remotes := map[string]string{"review": review, "tools": tools}
	api := newFakeGitHubAPI(t, "owner", `[{"name": "tools"}, {"name": "review"}, {"name": "fork", "fork": true}]`)
	scanOrg := func(org githubOrg) (orgMap, error) {
		return scanOrganizationAt(api.URL, org, func(repo githubRepo) string { return remotes[repo.name] })
	}
	handler := newWebHandler((&fakeScanner{}).scan, scanOrg)

	response := get(t, handler, "/scan?org=https://github.com/owner")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200:\n%s", response.Code, response.Body.String())
	}
	assertInOrder(t, response.Body.String(),
		`<p class="summary">owner · 2 repositories · 2 with skills · 2 skills</p>`,
		`<input type="hidden" name="org" value="https://github.com/owner">`,
		"<h2>owner/review · main @ ",
		"<li><strong>review</strong><p>Reviews a pull request.</p></li>",
		"<h2>owner/tools · main @ "+commit[:7]+" · 1 skill</h2>",
		"<li><strong>debug</strong><p>(no description)</p></li>",
	)

	body := get(t, handler, "/scan?org=https://github.com/owner&filter=pull").Body.String()
	assertInOrder(t, body, `<p class="matches">1 of 2 skills matches &#34;pull&#34;</p>`,
		"<li><strong>review</strong><p>Reviews a pull request.</p></li>")
	if strings.Contains(body, "owner/tools") {
		t.Errorf("filtered page contains the tools repository:\n%s", body)
	}
}

func TestScanPageOfGitHubOrganization(t *testing.T) {
	if testing.Short() {
		t.Skip("needs access to github.com")
	}
	server := httptest.NewServer(newWebHandler(scanRepository, scanOrganization))
	defer server.Close()

	response, err := http.Get(server.URL + "/scan?org=https://github.com/anthropics")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.StatusCode, body)
	}
	assertInOrder(t, string(body), `<p class="summary">anthropics · `, filterForm, groupingCheckbox,
		`<section class="repo">`+"\n  <h2>anthropics/", "<li><strong>")
	if strings.Contains(string(body), `class="error"`) {
		t.Errorf("page reports errors:\n%s", body)
	}
}
