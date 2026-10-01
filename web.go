package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// serveAddress is where `skill-atlas serve` listens. It is a loopback address, so only the local machine can connect.
const serveAddress = "127.0.0.1:8080"

// mapLifetime is how long the web interface keeps a map it has built, so that changing the filter or grouping shows
// the map again without analyzing the repository or organization again.
const mapLifetime = 5 * time.Minute

//go:embed web/page.html
var webFiles embed.FS

var pageTemplate = template.Must(template.ParseFS(webFiles, "web/page.html"))

// serve prints the address of listener and serves the web interface on it until the listener fails or is closed.
func serve(listener net.Listener, stdout io.Writer, stars *starStore) error {
	fmt.Fprintf(stdout, "Serving Skill Atlas at http://%s\n", listener.Addr())
	return http.Serve(listener, newCachingWebHandler(scanRepository, scanOrganization, stars))
}

// newCachingWebHandler is newWebHandler that keeps the maps built by scanRepo and scanOrg for mapLifetime. The map of
// an organization with repositories that could not be analyzed is not kept, so that opening it again retries them.
func newCachingWebHandler(
	scanRepo func(githubRepo) (skillMap, error), scanOrg func(githubOrg) (orgMap, error), stars *starStore,
) http.Handler {
	return newWebHandler(
		cached(scanRepo, func(skillMap) bool { return true }, mapLifetime, time.Now),
		cached(scanOrg, func(m orgMap) bool { return len(m.failures) == 0 }, mapLifetime, time.Now),
		stars,
	)
}

// newWebHandler serves the start page at /, repository maps at /scan?repo=<github-repository-url>, built by scanRepo,
// and organization maps at /scan?org=<github-organization-url>, built by scanOrg. An optional filter parameter limits
// the map to matching skills, and group=on groups similar skills. Starred skills, kept in stars, are shown first; a
// POST to /star stars or unstars a skill and redirects back to its map. Such requests from other websites are rejected.
func newWebHandler(
	scanRepo func(githubRepo) (skillMap, error), scanOrg func(githubOrg) (orgMap, error), stars *starStore,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, http.StatusOK, pageData{})
	})
	mux.HandleFunc("GET /scan", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		repoInput, orgInput := query.Get("repo"), query.Get("org")
		filter, grouping := query.Get("filter"), query.Get("group") == "on"
		switch {
		case repoInput != "" && orgInput != "":
			renderPage(w, http.StatusBadRequest, pageData{Error: "Pass either the repo or the org parameter, not both."})
		case orgInput != "":
			org, ok := requestedOrg(w, orgInput)
			if !ok {
				return
			}
			m, err := scanOrg(org)
			if err != nil {
				renderPage(w, http.StatusBadGateway, pageData{Error: err.Error()})
				return
			}
			renderPage(w, http.StatusOK, newOrgPage(m, filter, grouping, stars.starred))
		case repoInput != "":
			repo, ok := requestedRepo(w, repoInput)
			if !ok {
				return
			}
			m, err := scanRepo(repo)
			if err != nil {
				renderPage(w, http.StatusBadGateway, pageData{Error: err.Error()})
				return
			}
			renderPage(w, http.StatusOK, newMapPage(m, filter, grouping, stars.starred(m.repo)))
		default:
			renderPage(w, http.StatusBadRequest, pageData{Error: "Missing the repo or org parameter."})
		}
	})
	// The map page posts the repository and its filter and grouping, together with star=<path> or unstar=<path>. The
	// map of an organization also posts the organization, and the request then redirects back to it.
	mux.HandleFunc("POST /star", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := requestedRepo(w, r.PostFormValue("repo"))
		if !ok {
			return
		}
		form := r.PostForm
		back := scanURL("repo", repo.webURL(), form.Get("filter"), form.Get("group") == "on")
		if form.Has("org") {
			org, ok := requestedOrg(w, form.Get("org"))
			if !ok {
				return
			}
			back = scanURL("org", org.webURL(), form.Get("filter"), form.Get("group") == "on")
		}
		var path string
		var starred bool
		switch {
		case form.Has("star"):
			path, starred = form.Get("star"), true
		case form.Has("unstar"):
			path = form.Get("unstar")
		default:
			renderPage(w, http.StatusBadRequest, pageData{Error: "Missing the skill to star or unstar."})
			return
		}
		if err := stars.setStarred(repo, path, starred); err != nil {
			renderPage(w, http.StatusInternalServerError, pageData{Error: err.Error()})
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	})
	return http.NewCrossOriginProtection().Handler(mux)
}

// requestedRepo returns the repository that the repo parameter, given as input, names. If there is none, it responds
// with an error page.
func requestedRepo(w http.ResponseWriter, input string) (githubRepo, bool) {
	repo, ok := parseGitHubRepo(input)
	if !ok {
		message := "Missing the repo parameter."
		if input != "" {
			message = "Not a GitHub repository URL: " + input
		}
		renderPage(w, http.StatusBadRequest, pageData{Error: message})
	}
	return repo, ok
}

// requestedOrg returns the organization that the org parameter, given as input, names. If there is none, it responds
// with an error page.
func requestedOrg(w http.ResponseWriter, input string) (githubOrg, bool) {
	org, ok := parseGitHubOrg(input)
	if !ok {
		renderPage(w, http.StatusBadRequest, pageData{Error: "Not a GitHub organization URL: " + input})
	}
	return org, ok
}

// scanURL is the address of a map page, where param is "repo" or "org" and target the URL of the repository or
// organization, with the given filter and grouping.
func scanURL(param, target, filter string, grouping bool) string {
	address := "/scan?" + param + "=" + url.QueryEscape(target)
	if filter = strings.TrimSpace(filter); filter != "" {
		address += "&filter=" + url.QueryEscape(filter)
	}
	if grouping {
		address += "&group=on"
	}
	return address
}

// pageData is what the page shows: the skill map if Summary is set, otherwise how to request one, and any Error.
// The map of a repository is in Groups, and the map of an organization in Repos, one for each repository with skills
// that match Filter, with Failures for the repositories that could not be analyzed; Repo or Org is the URL of what was
// analyzed. Only the skills that match Filter are shown; Total counts all skills of the map, Matches only the shown
// ones. Grouping tells whether similar skills are grouped.
type pageData struct {
	Error    string
	Failures []string
	Summary  string
	Repo     string
	Org      string
	Filter   string
	Grouping bool
	Total    int
	Matches  int
	Groups   []pageGroup
	Repos    []pageRepo
}

// pageRepo is a repository in the map of an organization. Repo is its URL, and Form the id of the form that stars and
// unstars its skills.
type pageRepo struct {
	Summary, Repo, Form string
	Groups              []pageGroup
}

type pageGroup struct {
	Label  string
	Skills []pageSkill
}

// pageSkill is a skill as the page shows it. Path identifies the skill when it is starred or unstarred with the form
// whose id is Form.
type pageSkill struct {
	Name, Description, Path, Form string
	Starred                       bool
}

// newMapPage presents m with the skills that match filter, grouped if grouping is set, and with the skills whose paths
// are in starred shown first.
func newMapPage(m skillMap, filter string, grouping bool, starred map[string]bool) pageData {
	page := pageData{Summary: m.summary(), Repo: m.repo.webURL(), Filter: strings.TrimSpace(filter), Grouping: grouping}
	page.Groups = page.matchingGroups(m, starred, "stars")
	return page
}

// newOrgPage presents m like newMapPage presents the map of each of its repositories, with the paths of the skills
// starred in a repository given by starred.
func newOrgPage(m orgMap, filter string, grouping bool, starred func(githubRepo) map[string]bool) pageData {
	page := pageData{Summary: m.summary(), Org: m.org.webURL(), Filter: strings.TrimSpace(filter), Grouping: grouping}
	for _, failure := range m.failures {
		page.Failures = append(page.Failures, failure.Error())
	}
	for _, repo := range m.maps {
		form := fmt.Sprintf("stars-%d", len(page.Repos)+1)
		if groups := page.matchingGroups(repo, starred(repo.repo), form); len(groups) > 0 {
			page.Repos = append(page.Repos, pageRepo{Summary: repo.summary(), Repo: repo.repo.webURL(), Form: form, Groups: groups})
		}
	}
	return page
}

// matchingGroups arranges the map's skills that match the page's filter in groups, with the skills whose paths are in
// starred first, leaving out groups without matches, and adds the map's skills to the page's counts. Form is the id of
// the form that stars and unstars the map's skills.
func (p *pageData) matchingGroups(m skillMap, starred map[string]bool, form string) []pageGroup {
	var groups []pageGroup
	for _, group := range m.groups(p.Grouping, starred) {
		g := pageGroup{Label: group.label}
		for _, s := range group.skills {
			p.Total++
			if s.matches(p.Filter) {
				g.Skills = append(g.Skills, pageSkill{
					Name: s.name, Description: s.shownDescription(), Path: s.path, Starred: starred[s.path], Form: form,
				})
			}
		}
		if len(g.Skills) > 0 {
			groups = append(groups, g)
			p.Matches += len(g.Skills)
		}
	}
	return groups
}

// matches reports whether the skill's name or description contains filter, ignoring case. Every skill matches an
// empty filter.
func (s skill) matches(filter string) bool {
	filter = strings.ToLower(filter)
	return strings.Contains(strings.ToLower(s.name), filter) || strings.Contains(strings.ToLower(s.description), filter)
}

// MatchCount describes how many skills match the filter, e.g. `2 of 6 skills match "test"`.
func (p pageData) MatchCount() string {
	plural, verb := "s", "match"
	if p.Total == 1 {
		plural = ""
	}
	if p.Matches == 1 {
		verb = "matches"
	}
	return fmt.Sprintf("%d of %d skill%s %s \"%s\"", p.Matches, p.Total, plural, verb, p.Filter)
}

func renderPage(w http.ResponseWriter, status int, page pageData) {
	var b bytes.Buffer
	if err := pageTemplate.Execute(&b, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	b.WriteTo(w)
}
