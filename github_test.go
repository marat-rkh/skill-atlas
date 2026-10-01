package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseGitHubRepo(t *testing.T) {
	tests := []struct {
		input string
		want  githubRepo
	}{
		{"https://github.com/JetBrains/kotlin", githubRepo{"JetBrains", "kotlin"}},
		{"https://github.com/JetBrains/kotlin.git", githubRepo{"JetBrains", "kotlin"}},
		{"https://github.com/JetBrains/kotlin/", githubRepo{"JetBrains", "kotlin"}},
		{"https://github.com/JetBrains/kotlin/tree/master/compiler", githubRepo{"JetBrains", "kotlin"}},
		{"https://github.com/owner/repo?tab=readme-ov-file", githubRepo{"owner", "repo"}},
		{"https://github.com/owner/repo#readme", githubRepo{"owner", "repo"}},
		{"http://www.github.com/owner/repo", githubRepo{"owner", "repo"}},
		{"github.com/owner/repo", githubRepo{"owner", "repo"}},
		{"HTTPS://GitHub.com/Owner/Repo", githubRepo{"Owner", "Repo"}},
		{"git@github.com:owner/repo.git", githubRepo{"owner", "repo"}},
		{"  https://github.com/owner/repo \n", githubRepo{"owner", "repo"}},
		{"https://github.com/my-org/my_repo.js", githubRepo{"my-org", "my_repo.js"}},
	}
	for _, tt := range tests {
		got, ok := parseGitHubRepo(tt.input)
		if !ok || got != tt.want {
			t.Errorf("parseGitHubRepo(%q) = %v, %v; want %v, true", tt.input, got, ok, tt.want)
		}
	}
}

func TestParseGitHubRepoRejectsOtherInput(t *testing.T) {
	for _, input := range []string{
		"",
		"not-a-url",
		"owner/repo",
		"https://gitlab.com/owner/repo",
		"https://evilgithub.com/owner/repo",
		"https://github.com.evil.com/owner/repo",
		"https://github.com/owner",
		"https://github.com/",
		"https://github.com/owner/repo name",
		"https://github.com/orgs/JetBrains",
		"https://github.com/orgs/JetBrains/repositories",
	} {
		if got, ok := parseGitHubRepo(input); ok {
			t.Errorf("parseGitHubRepo(%q) = %v, true; want rejection", input, got)
		}
	}
}

func TestGitHubRepoURLs(t *testing.T) {
	repo := githubRepo{"JetBrains", "kotlin"}
	if got := repo.slug(); got != "JetBrains/kotlin" {
		t.Errorf("slug() = %q", got)
	}
	if got := repo.webURL(); got != "https://github.com/JetBrains/kotlin" {
		t.Errorf("webURL() = %q", got)
	}
	if got := repo.cloneURL(); got != "https://github.com/JetBrains/kotlin.git" {
		t.Errorf("cloneURL() = %q", got)
	}
}

func TestParseGitHubOrg(t *testing.T) {
	tests := []struct {
		input string
		want  githubOrg
	}{
		{"https://github.com/JetBrains", githubOrg{"JetBrains"}},
		{"https://github.com/JetBrains/", githubOrg{"JetBrains"}},
		{"https://github.com/JetBrains?tab=repositories", githubOrg{"JetBrains"}},
		{"https://github.com/JetBrains/?tab=repositories", githubOrg{"JetBrains"}},
		{"https://github.com/JetBrains#readme", githubOrg{"JetBrains"}},
		{"https://github.com/orgs/JetBrains", githubOrg{"JetBrains"}},
		{"https://github.com/orgs/JetBrains/repositories", githubOrg{"JetBrains"}},
		{"https://github.com/orgs/JetBrains/repositories?type=source", githubOrg{"JetBrains"}},
		{"http://www.github.com/JetBrains", githubOrg{"JetBrains"}},
		{"github.com/JetBrains", githubOrg{"JetBrains"}},
		{"HTTPS://GitHub.com/JetBrains", githubOrg{"JetBrains"}},
		{"https://github.com/jetbrains", githubOrg{"jetbrains"}},
		{"  https://github.com/JetBrains \n", githubOrg{"JetBrains"}},
		{"https://github.com/my-org_2", githubOrg{"my-org_2"}},
	}
	for _, tt := range tests {
		got, ok := parseGitHubOrg(tt.input)
		if !ok || got != tt.want {
			t.Errorf("parseGitHubOrg(%q) = %v, %v; want %v, true", tt.input, got, ok, tt.want)
		}
	}
}

func TestParseGitHubOrgRejectsOtherInput(t *testing.T) {
	for _, input := range []string{
		"",
		"JetBrains",
		"https://github.com",
		"https://github.com/",
		"https://github.com/JetBrains/kotlin",
		"https://github.com/JetBrains/kotlin/",
		"https://github.com/orgs",
		"https://github.com/orgs/",
		"https://gitlab.com/JetBrains",
		"https://evilgithub.com/JetBrains",
		"https://github.com.evil.com/JetBrains",
		"git@github.com:JetBrains",
		"https://github.com/..",
		"https://github.com/Jet.Brains",
		"https://github.com/Jet Brains",
	} {
		if got, ok := parseGitHubOrg(input); ok {
			t.Errorf("parseGitHubOrg(%q) = %v, true; want rejection", input, got)
		}
	}
}

func TestGitHubOrgURL(t *testing.T) {
	if got := (githubOrg{"JetBrains"}).webURL(); got != "https://github.com/JetBrains" {
		t.Errorf("webURL() = %q", got)
	}
}

// fakeGitHubAPI serves the repository list of an organization in pages, as the GitHub API does: the first page at
// /users/<org>/repos?per_page=100, and the next ones at the URLs in the Link header. It records the requests.
type fakeGitHubAPI struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
}

func newFakeGitHubAPI(t *testing.T, org string, pages ...string) *fakeGitHubAPI {
	t.Helper()
	api := &fakeGitHubAPI{}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		api.requests = append(api.requests, r.Clone(r.Context()))
		api.mu.Unlock()

		page := 1
		if r.URL.Path != "/users/"+org+"/repos" || r.URL.RawQuery != "per_page=100" {
			// Like GitHub, link to the next pages by the organization's ID rather than its name.
			page, _ = strconv.Atoi(r.URL.Query().Get("page"))
			if r.URL.Path != "/user/1/repos" || page < 2 || page > len(pages) {
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, `{"message": "Not Found", "status": "404"}`)
				return
			}
		}
		pageURL := func(n int) string { return fmt.Sprintf("%s/user/1/repos?per_page=100&page=%d", api.URL, n) }
		if page < len(pages) {
			w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next", <%s>; rel="last"`, pageURL(page+1), pageURL(len(pages))))
		} else if page > 1 {
			w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="prev", <%s>; rel="first"`, pageURL(page-1), pageURL(1)))
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		io.WriteString(w, pages[page-1])
	}))
	t.Cleanup(api.Close)
	return api
}

// received returns the requests the API received.
func (api *fakeGitHubAPI) received() []*http.Request {
	api.mu.Lock()
	defer api.mu.Unlock()
	return slices.Clone(api.requests)
}

// requestedURLs returns the paths and queries of the requests the API received.
func (api *fakeGitHubAPI) requestedURLs() []string {
	var urls []string
	for _, r := range api.received() {
		urls = append(urls, r.URL.String())
	}
	return urls
}

func TestListRepos(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	api := newFakeGitHubAPI(t, "JetBrains",
		`[{"name": "kotlin", "fork": false, "archived": false}, {"name": "fork", "fork": true}, {"name": "old", "archived": true}]`,
		`[{"name": "taken-down", "disabled": true}, {"name": "Exposed"}]`,
		`[]`,
	)

	got, err := listRepos(api.URL, githubOrg{"JetBrains"})
	if err != nil {
		t.Fatal(err)
	}
	want := []githubRepo{{"JetBrains", "kotlin"}, {"JetBrains", "old"}, {"JetBrains", "Exposed"}}
	if !slices.Equal(got, want) {
		t.Errorf("listRepos() = %v; want %v", got, want)
	}
	wantURLs := []string{"/users/JetBrains/repos?per_page=100", "/user/1/repos?per_page=100&page=2", "/user/1/repos?per_page=100&page=3"}
	if got := api.requestedURLs(); !slices.Equal(got, wantURLs) {
		t.Errorf("requested %q; want %q", got, wantURLs)
	}
	for _, r := range api.received() {
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("X-GitHub-Api-Version = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q without GITHUB_TOKEN; want none", got)
		}
	}
}

func TestListReposAuthenticatesWithGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	api := newFakeGitHubAPI(t, "JetBrains", `[{"name": "kotlin"}]`, `[{"name": "Exposed"}]`)

	if _, err := listRepos(api.URL, githubOrg{"JetBrains"}); err != nil {
		t.Fatal(err)
	}
	requests := api.received()
	if len(requests) != 2 {
		t.Fatalf("requested %q; want both pages", api.requestedURLs())
	}
	for _, r := range requests {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Errorf("Authorization = %q; want the token", got)
		}
	}
}

func TestListReposReportsMissingOrganization(t *testing.T) {
	api := newFakeGitHubAPI(t, "JetBrains", `[]`)

	_, err := listRepos(api.URL, githubOrg{"no-such-org"})
	if want := "cannot access https://github.com/no-such-org (organization or user not found)"; err == nil || err.Error() != want {
		t.Errorf("listRepos() error = %v; want %q", err, want)
	}
}

func TestListReposReportsAPIErrors(t *testing.T) {
	reset := time.Now().Add(20 * time.Minute)
	tests := []struct {
		name, token string
		status      int
		header      map[string]string
		body        string
		want        string
	}{
		{"rate limit", "", http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(reset.Unix(), 10)},
			`{"message": "API rate limit exceeded for 127.0.0.1."}`,
			"cannot list the repositories of https://github.com/JetBrains: GitHub API rate limit exceeded, try again after " + reset.Format("15:04") + "\n" +
				"Set the GITHUB_TOKEN environment variable to a GitHub token to raise the rate limit."},
		{"rate limit with a token", "secret-token", http.StatusTooManyRequests, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(reset.Unix(), 10)},
			`{"message": "API rate limit exceeded for user ID 1."}`,
			"cannot list the repositories of https://github.com/JetBrains: GitHub API rate limit exceeded, try again after " + reset.Format("15:04")},
		{"rate limit without a reset time", "", http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "0"}, `{}`,
			"cannot list the repositories of https://github.com/JetBrains: GitHub API rate limit exceeded\n" +
				"Set the GITHUB_TOKEN environment variable to a GitHub token to raise the rate limit."},
		{"bad credentials", "wrong-token", http.StatusUnauthorized, nil, `{"message": "Bad credentials"}`,
			"cannot list the repositories of https://github.com/JetBrains: GitHub API responded 401 Unauthorized: Bad credentials"},
		{"no message", "", http.StatusInternalServerError, nil, "oops",
			"cannot list the repositories of https://github.com/JetBrains: GitHub API responded 500 Internal Server Error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", tt.token)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, value := range tt.header {
					w.Header().Set(key, value)
				}
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer server.Close()

			_, err := listRepos(server.URL, githubOrg{"JetBrains"})
			if err == nil || err.Error() != tt.want {
				t.Errorf("listRepos() error = %v; want %q", err, tt.want)
			}
		})
	}
}

func TestListReposReportsUnexpectedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"repos": []}`)
	}))
	defer server.Close()

	_, err := listRepos(server.URL, githubOrg{"JetBrains"})
	if want := "cannot list the repositories of https://github.com/JetBrains: unexpected GitHub API response: "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("listRepos() error = %v; want prefix %q", err, want)
	}
}

func TestListReposReportsUnreachableAPI(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	_, err := listRepos(server.URL, githubOrg{"JetBrains"})
	if want := "cannot list the repositories of https://github.com/JetBrains\n"; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("listRepos() error = %v; want prefix %q", err, want)
	}
}

func TestListReposFollowsOnlyLinksToTheAPI(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	elsewhere := newFakeGitHubAPI(t, "JetBrains", `[]`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<`+elsewhere.URL+`/user/1/repos?per_page=100&page=2>; rel="next"`)
		io.WriteString(w, `[{"name": "kotlin"}]`)
	}))
	defer server.Close()

	_, err := listRepos(server.URL, githubOrg{"JetBrains"})
	if want := "cannot list the repositories of https://github.com/JetBrains: unexpected next page " + elsewhere.URL; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("listRepos() error = %v; want prefix %q", err, want)
	}
	if got := elsewhere.requestedURLs(); len(got) != 0 {
		t.Errorf("requested %q from another server; want nothing", got)
	}
}

func TestNextPageURL(t *testing.T) {
	tests := []struct{ link, want string }{
		{"", ""},
		{`<https://api.github.com/user/1/repos?page=2>; rel="next", <https://api.github.com/user/1/repos?page=9>; rel="last"`,
			"https://api.github.com/user/1/repos?page=2"},
		{`<https://api.github.com/user/1/repos?page=1>; rel="prev", <https://api.github.com/user/1/repos?page=3>; rel="next", ` +
			`<https://api.github.com/user/1/repos?page=9>; rel="last", <https://api.github.com/user/1/repos?page=1>; rel="first"`,
			"https://api.github.com/user/1/repos?page=3"},
		{`<https://api.github.com/user/1/repos?page=8>; rel="prev", <https://api.github.com/user/1/repos?page=1>; rel="first"`, ""},
	}
	for _, tt := range tests {
		if got := nextPageURL(tt.link); got != tt.want {
			t.Errorf("nextPageURL(%q) = %q; want %q", tt.link, got, tt.want)
		}
	}
}
