package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

// noStars returns a store of stars, saved in a temporary directory, in which no skill is starred yet.
func noStars(t *testing.T) *starStore {
	t.Helper()
	stars, err := openStarStore(filepath.Join(t.TempDir(), "stars.json"))
	if err != nil {
		t.Fatal(err)
	}
	return stars
}

// starring returns a store of stars, saved in a temporary directory, in which the skills at paths in repo are starred.
func starring(t *testing.T, repo githubRepo, paths ...string) *starStore {
	t.Helper()
	stars := noStars(t)
	for _, path := range paths {
		if err := stars.setStarred(repo, path, true); err != nil {
			t.Fatal(err)
		}
	}
	return stars
}

// postForm sends form to handler in a POST request for target, as the browser does when the map page submits it.
func postForm(t *testing.T, handler http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// listedSkill is how the map page lists a skill: a button that stars or unstars it, its name and its description.
func listedSkill(path, name, description string, starred bool) string {
	button := `<button class="star" form="stars" name="star" value="` + path + `" title="Star" aria-label="Star ` + name + `">☆</button>`
	if starred {
		button = `<button class="star" form="stars" name="unstar" value="` + path + `" title="Unstar" aria-label="Unstar ` + name + `">★</button>`
	}
	return "<li>" + button + " <strong>" + name + "</strong><p>" + description + "</p></li>"
}

var (
	alphaOne  = listedSkill(".claude/skills/alpha-one", "alpha-one", "Does alpha things.", false)
	alphaTwo  = listedSkill(".claude/skills/alpha-two", "alpha-two", "(no description)", false)
	rootSkill = listedSkill("", "root-skill", "At the root.", false)
)

const usageHint = "Open <code>/scan?repo=&lt;github-repository-url&gt;</code>"

const filterForm = `<form action="/scan" method="get" role="search">`

// groupingCheckbox is the start of the grouping checkbox, which is in the filter form.
const groupingCheckbox = `<label><input type="checkbox" name="group"`

const starForm = `<form id="stars" action="/star" method="post">`

func TestStartPage(t *testing.T) {
	response := get(t, newWebHandler((&fakeScanner{}).scan, noStars(t)), "/")

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
	if response := get(t, newWebHandler((&fakeScanner{}).scan, noStars(t)), "/missing"); response.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", response.Code)
	}
}

func TestScanPageShowsMap(t *testing.T) {
	scanner := &fakeScanner{m: exampleMap}
	response := get(t, newWebHandler(scanner.scan, noStars(t)), "/scan?repo=https://github.com/JetBrains/kotlin")

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
		starForm,
		`<input type="hidden" name="repo" value="https://github.com/JetBrains/kotlin">`,
		`<input type="hidden" name="filter" value="">`+"\n</form>",
		"<section>\n  <ul>",
		alphaOne,
		alphaTwo,
		rootSkill,
		"</ul>\n</section>",
	)
	for _, unexpected := range []string{"<h2>", "★", "No SKILL.md files found.", "No skills match", `class="matches"`, usageHint, `class="error"`} {
		if strings.Contains(body, unexpected) {
			t.Errorf("page contains %q:\n%s", unexpected, body)
		}
	}
}

func TestScanPageGroupsSimilarSkills(t *testing.T) {
	scanner := &fakeScanner{m: exampleMap}
	response := get(t, newWebHandler(scanner.scan, noStars(t)), "/scan?repo=https://github.com/JetBrains/kotlin&group=on")

	if response.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", response.Code)
	}
	if !slices.Equal(scanner.requested, []githubRepo{kotlin}) {
		t.Errorf("scanned %v; want [%v]", scanner.requested, kotlin)
	}
	assertInOrder(t, response.Body.String(),
		`<p class="summary">JetBrains/kotlin · master @ c823f9e · 3 skills</p>`,
		groupingCheckbox+` checked onchange="this.form.submit()"> Group similar skills</label>`,
		starForm,
		`<input type="hidden" name="group" value="on">`+"\n</form>",
		"<h2>alpha</h2>",
		alphaOne,
		alphaTwo,
		"<h2>Other</h2>",
		rootSkill,
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
		response := get(t, newWebHandler(scanner.scan, noStars(t)), target)
		if response.Code != http.StatusOK || !slices.Equal(scanner.requested, []githubRepo{kotlin}) {
			t.Errorf("GET %s: status %d, scanned %v; want 200 and [%v]", target, response.Code, scanner.requested, kotlin)
		}
	}
}

func TestScanPageWithoutSkills(t *testing.T) {
	scanner := &fakeScanner{m: skillMap{repo: kotlin, branch: "master", commit: "c823f9e0123456789"}}
	response := get(t, newWebHandler(scanner.scan, noStars(t)), "/scan?repo=https://github.com/JetBrains/kotlin")

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
			response := get(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, noStars(t)), target)

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
		body := get(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, noStars(t)), target).Body.String()

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
	body := get(t, newWebHandler(scanner.scan, noStars(t)), "/scan?repo=https://github.com/JetBrains/kotlin&filter=x").Body.String()

	assertInOrder(t, body, filterForm, `<p class="matches">0 of 0 skills match &#34;x&#34;</p>`, "<p>No SKILL.md files found.</p>")
	if strings.Contains(body, "No skills match") {
		t.Errorf("page says no skills match:\n%s", body)
	}
}

func TestScanPageFilterFormKeepsRepository(t *testing.T) {
	// The form must submit the scanned repository, even if it was requested in another URL form.
	for _, repo := range []string{"git@github.com:JetBrains/kotlin.git", "https://github.com/JetBrains/kotlin/tree/master/compiler"} {
		target := "/scan?repo=" + url.QueryEscape(repo) + "&filter=alpha"
		body := get(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, noStars(t)), target).Body.String()
		assertInOrder(t, body, filterForm,
			`<input type="hidden" name="repo" value="https://github.com/JetBrains/kotlin">`,
			`<input type="search" name="filter" value="alpha">`,
			`<button type="submit">Filter</button>`,
			"</form>",
		)
	}
}

func TestScanPageFormKeepsFilterAndGrouping(t *testing.T) {
	body := get(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, noStars(t)),
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
	body := get(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, noStars(t)), target).Body.String()

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
		{name: "<b>name</b>-1", description: "<script>alert(1)</script>", path: `"><script>alert(2)</script>`},
		{name: "<b>name</b>-2"},
	}}}
	body := get(t, newWebHandler(scanner.scan, noStars(t)), "/scan?repo=github.com/JetBrains/kotlin&group=on").Body.String()

	assertInOrder(t, body,
		"<h2>&lt;b&gt;name&lt;/b&gt;</h2>",
		`<button class="star" form="stars" name="star" value="&#34;&gt;&lt;script&gt;alert(2)&lt;/script&gt;" title="Star" aria-label="Star &lt;b&gt;name&lt;/b&gt;-1">☆</button>`,
		"<strong>&lt;b&gt;name&lt;/b&gt;-1</strong>",
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
		response := get(t, newWebHandler(scanner.scan, noStars(t)), tt.target)
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
	response := get(t, newWebHandler(scanner.scan, noStars(t)), "/scan?repo=https://github.com/JetBrains/kotlin")

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
	newWebHandler((&fakeScanner{m: exampleMap}).scan, noStars(t)).ServeHTTP(recorder, request)
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
	response := get(t, newWebHandler(scan, noStars(t)), "/scan?repo=https://github.com/owner/repo")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200:\n%s", response.Code, response.Body.String())
	}
	assertInOrder(t, response.Body.String(),
		`<p class="summary">owner/repo · main @ `+commit[:7]+` · 2 skills</p>`,
		`<input type="hidden" name="repo" value="https://github.com/owner/repo">`,
		listedSkill(".claude/skills/debug", "debug", "(no description)", false),
		listedSkill(".claude/skills/review", "review", "Reviews a pull request.", false),
	)

	response = get(t, newWebHandler(scan, noStars(t)), "/scan?repo=https://github.com/owner/repo&filter=pull")
	body := response.Body.String()
	assertInOrder(t, body, `<p class="matches">1 of 2 skills matches &#34;pull&#34;</p>`,
		listedSkill(".claude/skills/review", "review", "Reviews a pull request.", false))
	if strings.Contains(body, "<strong>debug</strong>") {
		t.Errorf("filtered page contains the debug skill:\n%s", body)
	}
}

func TestStarSkillOfLocalRepository(t *testing.T) {
	remote, _ := newRemote(t, map[string]string{
		".claude/skills/review/SKILL.md": "---\nname: review\ndescription: Reviews a pull request.\n---\n",
		".claude/skills/debug/SKILL.md":  "---\nname: debug\n---\n",
	})
	scan := func(repo githubRepo) (skillMap, error) { return scanRemote(repo, remote) }
	server := httptest.NewServer(newWebHandler(scan, noStars(t)))
	defer server.Close()

	// The browser submits the star form of the map page and follows the redirect back to the map.
	form := url.Values{"repo": {"https://github.com/owner/repo"}, "filter": {"e"}, "group": {"on"}, "star": {".claude/skills/review"}}
	response, err := http.PostForm(server.URL+"/star", form)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.StatusCode, body)
	}
	if got, want := response.Request.URL.RequestURI(), "/scan?repo=https%3A%2F%2Fgithub.com%2Fowner%2Frepo&filter=e&group=on"; got != want {
		t.Errorf("redirected to %s; want %s", got, want)
	}
	assertInOrder(t, string(body),
		`<input type="search" name="filter" value="e">`,
		groupingCheckbox+` checked onchange="this.form.submit()"> Group similar skills</label>`,
		`<p class="matches">2 of 2 skills match &#34;e&#34;</p>`,
		"<h2>Starred</h2>",
		listedSkill(".claude/skills/review", "review", "Reviews a pull request.", true),
		"<h2>Other</h2>",
		listedSkill(".claude/skills/debug", "debug", "(no description)", false),
	)
}

func TestScanPageShowsStarredSkillsFirst(t *testing.T) {
	stars := starring(t, kotlin, "", ".claude/skills/alpha-two")
	handler := newWebHandler((&fakeScanner{m: exampleMap}).scan, stars)
	starredRoot := listedSkill("", "root-skill", "At the root.", true)
	starredAlphaTwo := listedSkill(".claude/skills/alpha-two", "alpha-two", "(no description)", true)

	body := get(t, handler, "/scan?repo=https://github.com/JetBrains/kotlin").Body.String()
	assertInOrder(t, body, `<p class="summary">JetBrains/kotlin · master @ c823f9e · 3 skills</p>`,
		"<section>\n  <ul>", starredAlphaTwo, starredRoot, alphaOne, "</ul>\n</section>")
	if strings.Contains(body, "<h2>") || strings.Count(body, "<section>") != 1 {
		t.Errorf("page without grouping does not show a single list without a heading:\n%s", body)
	}

	// Groups are formed from all skills, so alpha-one stays in the alpha group, and the Other group has no skills left.
	body = get(t, handler, "/scan?repo=https://github.com/JetBrains/kotlin&group=on").Body.String()
	assertInOrder(t, body, "<h2>Starred</h2>", starredAlphaTwo, starredRoot, "<h2>alpha</h2>", alphaOne)
	if strings.Contains(body, "<h2>Other</h2>") {
		t.Errorf("page contains the Other group without skills:\n%s", body)
	}
}

func TestScanPageFiltersStarredSkills(t *testing.T) {
	tests := []struct {
		name, filter, group, matchCount string
		shown, hidden                   []string
	}{
		{"starred skill matches", "alpha", "&group=on", `2 of 3 skills match "alpha"`,
			[]string{"<h2>Starred</h2>", listedSkill(".claude/skills/alpha-one", "alpha-one", "Does alpha things.", true),
				"<h2>alpha</h2>", alphaTwo},
			[]string{"<h2>Other</h2>", "root-skill"}},
		{"starred skill does not match", "root", "&group=on", `1 of 3 skills matches "root"`,
			[]string{"<h2>Other</h2>", rootSkill},
			[]string{"<h2>Starred</h2>", "<h2>alpha</h2>", "★"}},
		{"without grouping", "s", "", `2 of 3 skills match "s"`,
			[]string{listedSkill(".claude/skills/alpha-one", "alpha-one", "Does alpha things.", true), rootSkill},
			[]string{"<h2>", "alpha-two"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newWebHandler((&fakeScanner{m: exampleMap}).scan, starring(t, kotlin, ".claude/skills/alpha-one"))
			target := "/scan?repo=https://github.com/JetBrains/kotlin&filter=" + url.QueryEscape(tt.filter) + tt.group
			body := get(t, handler, target).Body.String()

			assertInOrder(t, body, `<p class="matches">`+strings.ReplaceAll(tt.matchCount, `"`, "&#34;")+"</p>")
			assertInOrder(t, body, tt.shown...)
			for _, hidden := range tt.hidden {
				if strings.Contains(body, hidden) {
					t.Errorf("page contains %q:\n%s", hidden, body)
				}
			}
		})
	}
}

func TestScanPageStarFormKeepsFilterAndGrouping(t *testing.T) {
	body := get(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, noStars(t)),
		"/scan?repo=git@github.com:JetBrains/kotlin.git&filter=%20alpha%20&group=on").Body.String()
	assertInOrder(t, body, starForm,
		`<input type="hidden" name="repo" value="https://github.com/JetBrains/kotlin">`,
		`<input type="hidden" name="filter" value="alpha">`,
		`<input type="hidden" name="group" value="on">`,
		"</form>",
	)
}

func TestStarSkill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stars.json")
	stars, err := openStarStore(path)
	if err != nil {
		t.Fatal(err)
	}
	handler := newWebHandler((&fakeScanner{m: exampleMap}).scan, stars)
	submit := func(action, skillPath string) {
		t.Helper()
		form := url.Values{"repo": {"https://github.com/JetBrains/kotlin"}, "filter": {""}, action: {skillPath}}
		response := postForm(t, handler, "/star", form)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("%s %q: status = %d; want 303:\n%s", action, skillPath, response.Code, response.Body.String())
		}
		if got, want := response.Header().Get("Location"), "/scan?repo=https%3A%2F%2Fgithub.com%2FJetBrains%2Fkotlin"; got != want {
			t.Errorf("%s %q: Location = %q; want %q", action, skillPath, got, want)
		}
	}
	page := func() string { return get(t, handler, "/scan?repo=https://github.com/JetBrains/kotlin").Body.String() }

	submit("star", ".claude/skills/alpha-two")
	assertInOrder(t, page(), listedSkill(".claude/skills/alpha-two", "alpha-two", "(no description)", true), alphaOne, rootSkill)

	// The skill at the root of the repository has an empty path.
	submit("star", "")
	assertInOrder(t, page(), listedSkill(".claude/skills/alpha-two", "alpha-two", "(no description)", true),
		listedSkill("", "root-skill", "At the root.", true), alphaOne)

	submit("unstar", ".claude/skills/alpha-two")
	assertInOrder(t, page(), listedSkill("", "root-skill", "At the root.", true), alphaOne, alphaTwo)

	// The stars are saved, so a restarted server shows them too.
	reopened, err := openStarStore(path)
	if err != nil {
		t.Fatal(err)
	}
	body := get(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, reopened), "/scan?repo=https://github.com/JetBrains/kotlin").Body.String()
	assertInOrder(t, body, listedSkill("", "root-skill", "At the root.", true), alphaOne, alphaTwo)
}

func TestStarRedirectsToMapWithFilterAndGrouping(t *testing.T) {
	const kotlinMap = "/scan?repo=https%3A%2F%2Fgithub.com%2FJetBrains%2Fkotlin"
	tests := []struct {
		form url.Values
		want string
	}{
		{url.Values{"repo": {"https://github.com/JetBrains/kotlin"}, "star": {"x"}}, kotlinMap},
		{url.Values{"repo": {"https://github.com/JetBrains/kotlin"}, "unstar": {"x"}, "filter": {"  "}, "group": {""}}, kotlinMap},
		{url.Values{"repo": {"https://github.com/JetBrains/kotlin"}, "star": {"x"}, "group": {"on"}}, kotlinMap + "&group=on"},
		{url.Values{"repo": {"git@github.com:JetBrains/kotlin.git"}, "star": {"x"}, "filter": {" alpha & one "}, "group": {"on"}},
			kotlinMap + "&filter=alpha+%26+one&group=on"},
	}
	for _, tt := range tests {
		response := postForm(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, noStars(t)), "/star", tt.form)
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != tt.want {
			t.Errorf("POST /star %v: status %d, Location %q; want 303 and %q", tt.form, response.Code, response.Header().Get("Location"), tt.want)
		}
	}
}

func TestStarRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		form    url.Values
		message string
	}{
		{url.Values{"star": {"x"}}, "Missing the repo parameter."},
		{url.Values{"repo": {"https://gitlab.com/owner/repo"}, "star": {"x"}}, "Not a GitHub repository URL: https://gitlab.com/owner/repo"},
		{url.Values{"repo": {"https://github.com/JetBrains/kotlin"}, "filter": {"x"}}, "Missing the skill to star or unstar."},
	}
	for _, tt := range tests {
		stars := noStars(t)
		response := postForm(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, stars), "/star", tt.form)
		if response.Code != http.StatusBadRequest {
			t.Errorf("POST /star %v: status = %d; want 400", tt.form, response.Code)
		}
		assertInOrder(t, response.Body.String(), `<p class="error">`+tt.message+"</p>", usageHint)
		if _, err := os.Stat(stars.path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("POST /star %v saved stars", tt.form)
		}
	}
}

func TestStarRequiresPost(t *testing.T) {
	stars := noStars(t)
	response := get(t, newWebHandler((&fakeScanner{m: exampleMap}).scan, stars), "/star?repo=https://github.com/JetBrains/kotlin&star=x")
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d; want 405", response.Code)
	}
	if got := stars.starred(kotlin); len(got) != 0 {
		t.Errorf("GET starred %v", got)
	}
}

func TestStarRejectsRequestsFromOtherWebsites(t *testing.T) {
	tests := []struct {
		headers map[string]string
		allowed bool
	}{
		{map[string]string{"Sec-Fetch-Site": "same-origin"}, true},
		{map[string]string{"Origin": "http://127.0.0.1:8080"}, true}, // a browser that doesn't send Sec-Fetch-Site
		{nil, true}, // not a browser, e.g. curl
		{map[string]string{"Sec-Fetch-Site": "cross-site"}, false},
		{map[string]string{"Sec-Fetch-Site": "same-site"}, false}, // e.g. a page that another port of 127.0.0.1 serves
		{map[string]string{"Origin": "https://example.com"}, false},
		{map[string]string{"Origin": "http://127.0.0.1:3000"}, false},
	}
	for _, tt := range tests {
		stars := noStars(t)
		form := url.Values{"repo": {"https://github.com/JetBrains/kotlin"}, "star": {".claude/skills/alpha-one"}}
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/star", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for name, value := range tt.headers {
			request.Header.Set(name, value)
		}
		recorder := httptest.NewRecorder()
		newWebHandler((&fakeScanner{m: exampleMap}).scan, stars).ServeHTTP(recorder, request)

		wantCode := http.StatusForbidden
		if tt.allowed {
			wantCode = http.StatusSeeOther
		}
		if recorder.Code != wantCode {
			t.Errorf("POST with headers %v: status = %d; want %d", tt.headers, recorder.Code, wantCode)
		}
		if starred := stars.starred(kotlin)[".claude/skills/alpha-one"]; starred != tt.allowed {
			t.Errorf("POST with headers %v: starred = %v; want %v", tt.headers, starred, tt.allowed)
		}
	}
}

func TestStarReportsSaveErrors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skill-atlas")
	stars, err := openStarStore(filepath.Join(dir, "stars.json"))
	if err != nil {
		t.Fatal(err)
	}
	// A file where the directory of the stars should be makes saving them fail.
	if err := os.WriteFile(dir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newWebHandler((&fakeScanner{m: exampleMap}).scan, stars)
	response := postForm(t, handler, "/star", url.Values{"repo": {"https://github.com/JetBrains/kotlin"}, "star": {".claude/skills/alpha-one"}})

	if response.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500", response.Code)
	}
	assertInOrder(t, response.Body.String(), `<p class="error">cannot save stars: `, "</p>", usageHint)
	assertInOrder(t, get(t, handler, "/scan?repo=https://github.com/JetBrains/kotlin").Body.String(), alphaOne, alphaTwo, rootSkill)
}

func TestStarsBelongToRepositoryIgnoringCase(t *testing.T) {
	stars := starring(t, githubRepo{"other", "repo"}, ".claude/skills/alpha-one")
	handler := newWebHandler((&fakeScanner{m: exampleMap}).scan, stars)
	if body := get(t, handler, "/scan?repo=https://github.com/JetBrains/kotlin").Body.String(); strings.Contains(body, "★") {
		t.Errorf("page shows the stars of another repository:\n%s", body)
	}

	postForm(t, handler, "/star", url.Values{"repo": {"https://github.com/jetbrains/KOTLIN"}, "star": {".claude/skills/alpha-two"}})
	assertInOrder(t, get(t, handler, "/scan?repo=https://github.com/JetBrains/kotlin").Body.String(),
		listedSkill(".claude/skills/alpha-two", "alpha-two", "(no description)", true), alphaOne, rootSkill)
}

func TestStarsBelongToSkillDirectories(t *testing.T) {
	scanner := &fakeScanner{m: skillMap{repo: kotlin, commit: "c823f9e", skills: []skill{
		{name: "docs", description: "First.", path: "a/docs"},
		{name: "docs", description: "Second.", path: "b/docs"},
	}}}
	body := get(t, newWebHandler(scanner.scan, starring(t, kotlin, "b/docs")), "/scan?repo=https://github.com/JetBrains/kotlin").Body.String()
	assertInOrder(t, body, listedSkill("b/docs", "docs", "Second.", true), listedSkill("a/docs", "docs", "First.", false))
}

func TestServe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	served := make(chan error)
	go func() { served <- serve(listener, &stdout, noStars(t)) }()

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
	server := httptest.NewServer(newWebHandler(scanRepository, noStars(t)))
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
	assertInOrder(t, string(body), `<p class="summary">JetBrains/kotlin · `, filterForm, groupingCheckbox, starForm, `<li><button class="star"`)
}
