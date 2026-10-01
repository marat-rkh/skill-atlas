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
)

// serveAddress is where `skill-atlas serve` listens. It is a loopback address, so only the local machine can connect.
const serveAddress = "127.0.0.1:8080"

//go:embed web/page.html
var webFiles embed.FS

var pageTemplate = template.Must(template.ParseFS(webFiles, "web/page.html"))

// serve prints the address of listener and serves the web interface on it until the listener fails or is closed.
func serve(listener net.Listener, stdout io.Writer, stars *starStore) error {
	fmt.Fprintf(stdout, "Serving Skill Atlas at http://%s\n", listener.Addr())
	return http.Serve(listener, newWebHandler(scanRepository, stars))
}

// newWebHandler serves the start page at / and skill maps at /scan?repo=<github-repository-url>, built by scan.
// An optional filter parameter limits the map to matching skills, and group=on groups similar skills. Starred skills,
// kept in stars, are shown first; a POST to /star stars or unstars a skill and redirects back to its map. Such requests
// from other websites are rejected.
func newWebHandler(scan func(githubRepo) (skillMap, error), stars *starStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, http.StatusOK, pageData{})
	})
	mux.HandleFunc("GET /scan", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := requestedRepo(w, r.URL.Query().Get("repo"))
		if !ok {
			return
		}
		m, err := scan(repo)
		if err != nil {
			renderPage(w, http.StatusBadGateway, pageData{Error: err.Error()})
			return
		}
		query := r.URL.Query()
		renderPage(w, http.StatusOK, newMapPage(m, query.Get("filter"), query.Get("group") == "on", stars.starred(m.repo)))
	})
	// The map page posts the repository and its filter and grouping, together with star=<path> or unstar=<path>.
	mux.HandleFunc("POST /star", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := requestedRepo(w, r.PostFormValue("repo"))
		if !ok {
			return
		}
		form := r.PostForm
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
		http.Redirect(w, r, scanURL(repo, form.Get("filter"), form.Get("group") == "on"), http.StatusSeeOther)
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

// scanURL is the address of repo's map page with the given filter and grouping.
func scanURL(repo githubRepo, filter string, grouping bool) string {
	address := "/scan?repo=" + url.QueryEscape(repo.webURL())
	if filter = strings.TrimSpace(filter); filter != "" {
		address += "&filter=" + url.QueryEscape(filter)
	}
	if grouping {
		address += "&group=on"
	}
	return address
}

// pageData is what the page shows: the skill map if Summary is set, otherwise how to request one, and any Error.
// Groups holds only the skills that match Filter; Total counts all skills of the map, Matches only the shown ones.
// Grouping tells whether similar skills are grouped.
type pageData struct {
	Error    string
	Summary  string
	Repo     string
	Filter   string
	Grouping bool
	Total    int
	Matches  int
	Groups   []pageGroup
}

type pageGroup struct {
	Label  string
	Skills []pageSkill
}

// pageSkill is a skill as the page shows it. Path identifies the skill when it is starred or unstarred.
type pageSkill struct {
	Name, Description, Path string
	Starred                 bool
}

// newMapPage presents m with the skills that match filter, grouped if grouping is set, and with the skills whose paths
// are in starred shown first.
func newMapPage(m skillMap, filter string, grouping bool, starred map[string]bool) pageData {
	page := pageData{Summary: m.summary(), Repo: m.repo.webURL(), Filter: strings.TrimSpace(filter), Grouping: grouping}
	for _, group := range m.groups(grouping, starred) {
		g := pageGroup{Label: group.label}
		for _, s := range group.skills {
			page.Total++
			if s.matches(page.Filter) {
				g.Skills = append(g.Skills, pageSkill{
					Name: s.name, Description: s.shownDescription(), Path: s.path, Starred: starred[s.path],
				})
			}
		}
		if len(g.Skills) > 0 {
			page.Groups = append(page.Groups, g)
			page.Matches += len(g.Skills)
		}
	}
	return page
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
