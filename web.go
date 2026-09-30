package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
)

// serveAddress is where `skill-atlas serve` listens. It is a loopback address, so only the local machine can connect.
const serveAddress = "127.0.0.1:8080"

//go:embed web/page.html
var webFiles embed.FS

var pageTemplate = template.Must(template.ParseFS(webFiles, "web/page.html"))

// serve prints the address of listener and serves the web interface on it until the listener fails or is closed.
func serve(listener net.Listener, stdout io.Writer) error {
	fmt.Fprintf(stdout, "Serving Skill Atlas at http://%s\n", listener.Addr())
	return http.Serve(listener, newWebHandler(scanRepository))
}

// newWebHandler serves the start page at / and skill maps at /scan?repo=<github-repository-url>, built by scan.
func newWebHandler(scan func(githubRepo) (skillMap, error)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, http.StatusOK, pageData{})
	})
	mux.HandleFunc("GET /scan", func(w http.ResponseWriter, r *http.Request) {
		input := r.URL.Query().Get("repo")
		repo, ok := parseGitHubRepo(input)
		if !ok {
			message := "Missing the repo parameter."
			if input != "" {
				message = "Not a GitHub repository URL: " + input
			}
			renderPage(w, http.StatusBadRequest, pageData{Error: message})
			return
		}
		m, err := scan(repo)
		if err != nil {
			renderPage(w, http.StatusBadGateway, pageData{Error: err.Error()})
			return
		}
		renderPage(w, http.StatusOK, newMapPage(m))
	})
	return mux
}

// pageData is what the page shows: the skill map if Summary is set, otherwise how to request one, and any Error.
type pageData struct {
	Error   string
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

func newMapPage(m skillMap) pageData {
	page := pageData{Summary: m.summary()}
	for _, group := range m.groups {
		g := pageGroup{Label: group.label()}
		for _, s := range group.skills {
			g.Skills = append(g.Skills, pageSkill{Name: s.name, Description: s.shownDescription()})
		}
		page.Groups = append(page.Groups, g)
	}
	return page
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
