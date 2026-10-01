package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
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
func serve(listener net.Listener, stdout io.Writer) error {
	fmt.Fprintf(stdout, "Serving Skill Atlas at http://%s\n", listener.Addr())
	return http.Serve(listener, newCachingWebHandler(scanRepository, scanOrganization))
}

// newCachingWebHandler is newWebHandler that keeps the maps built by scanRepo and scanOrg for mapLifetime. The map of
// an organization with repositories that could not be analyzed is not kept, so that opening it again retries them.
func newCachingWebHandler(scanRepo func(githubRepo) (skillMap, error), scanOrg func(githubOrg) (orgMap, error)) http.Handler {
	return newWebHandler(
		cached(scanRepo, func(skillMap) bool { return true }, mapLifetime, time.Now),
		cached(scanOrg, func(m orgMap) bool { return len(m.failures) == 0 }, mapLifetime, time.Now),
	)
}

// newWebHandler serves the start page at /, repository maps at /scan?repo=<github-repository-url>, built by scanRepo,
// and organization maps at /scan?org=<github-organization-url>, built by scanOrg. An optional filter parameter limits
// the map to matching skills, and group=on groups similar skills.
func newWebHandler(scanRepo func(githubRepo) (skillMap, error), scanOrg func(githubOrg) (orgMap, error)) http.Handler {
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
			org, ok := parseGitHubOrg(orgInput)
			if !ok {
				renderPage(w, http.StatusBadRequest, pageData{Error: "Not a GitHub organization URL: " + orgInput})
				return
			}
			m, err := scanOrg(org)
			if err != nil {
				renderPage(w, http.StatusBadGateway, pageData{Error: err.Error()})
				return
			}
			renderPage(w, http.StatusOK, newOrgPage(m, filter, grouping))
		case repoInput != "":
			repo, ok := parseGitHubRepo(repoInput)
			if !ok {
				renderPage(w, http.StatusBadRequest, pageData{Error: "Not a GitHub repository URL: " + repoInput})
				return
			}
			m, err := scanRepo(repo)
			if err != nil {
				renderPage(w, http.StatusBadGateway, pageData{Error: err.Error()})
				return
			}
			renderPage(w, http.StatusOK, newMapPage(m, filter, grouping))
		default:
			renderPage(w, http.StatusBadRequest, pageData{Error: "Missing the repo or org parameter."})
		}
	})
	return mux
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

// pageRepo is a repository in the map of an organization.
type pageRepo struct {
	Summary string
	Groups  []pageGroup
}

type pageGroup struct {
	Label  string
	Skills []pageSkill
}

type pageSkill struct {
	Name, Description string
}

func newMapPage(m skillMap, filter string, grouping bool) pageData {
	page := pageData{Summary: m.summary(), Repo: m.repo.webURL(), Filter: strings.TrimSpace(filter), Grouping: grouping}
	page.Groups = page.matchingGroups(m)
	return page
}

func newOrgPage(m orgMap, filter string, grouping bool) pageData {
	page := pageData{Summary: m.summary(), Org: m.org.webURL(), Filter: strings.TrimSpace(filter), Grouping: grouping}
	for _, failure := range m.failures {
		page.Failures = append(page.Failures, failure.Error())
	}
	for _, repo := range m.maps {
		if groups := page.matchingGroups(repo); len(groups) > 0 {
			page.Repos = append(page.Repos, pageRepo{Summary: repo.summary(), Groups: groups})
		}
	}
	return page
}

// matchingGroups arranges the map's skills that match the page's filter in groups, leaving out groups without them,
// and adds the map's skills to the page's counts.
func (p *pageData) matchingGroups(m skillMap) []pageGroup {
	var groups []pageGroup
	for _, group := range m.groups(p.Grouping) {
		g := pageGroup{Label: group.label}
		for _, s := range group.skills {
			p.Total++
			if s.matches(p.Filter) {
				g.Skills = append(g.Skills, pageSkill{Name: s.name, Description: s.shownDescription()})
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
