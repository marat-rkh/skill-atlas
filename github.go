package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// githubRepo is a GitHub repository identified by its owner and name.
type githubRepo struct {
	owner, name string
}

func (r githubRepo) slug() string     { return r.owner + "/" + r.name }
func (r githubRepo) webURL() string   { return "https://github.com/" + r.slug() }
func (r githubRepo) cloneURL() string { return r.webURL() + ".git" }

// githubOrg is a GitHub organization identified by its name. A user account owns repositories the same way, so it can
// be a githubOrg too.
type githubOrg struct {
	name string
}

func (o githubOrg) webURL() string { return "https://github.com/" + o.name }

// Accepts https://github.com/o/r, github.com/o/r, git@github.com:o/r.git; anything after o/r is ignored.
var githubURLPattern = regexp.MustCompile(
	`(?i)^(?:(?:https?://)?(?:www\.)?github\.com/|git@github\.com:)([\w.-]+)/([\w.-]+?)(?:\.git)?(?:[/?#].*)?$`,
)

func parseGitHubRepo(input string) (githubRepo, bool) {
	match := githubURLPattern.FindStringSubmatch(strings.TrimSpace(input))
	// github.com/orgs/<org> is an organization's page, not a repository.
	if match == nil || strings.EqualFold(match[1], "orgs") {
		return githubRepo{}, false
	}
	return githubRepo{owner: match[1], name: match[2]}, true
}

// Accepts https://github.com/o, github.com/o/, https://github.com/orgs/o/repositories; a query or fragment is ignored.
var githubOrgURLPattern = regexp.MustCompile(
	`(?i)^(?:https?://)?(?:www\.)?github\.com/(?:orgs/([\w-]+)(?:[/?#].*)?|([\w-]+)/?(?:[?#].*)?)$`,
)

func parseGitHubOrg(input string) (githubOrg, bool) {
	match := githubOrgURLPattern.FindStringSubmatch(strings.TrimSpace(input))
	// github.com/orgs itself is not an organization.
	if match == nil || strings.EqualFold(match[2], "orgs") {
		return githubOrg{}, false
	}
	return githubOrg{name: match[1] + match[2]}, true
}

// githubAPI is the base URL of the GitHub REST API.
const githubAPI = "https://api.github.com"

var apiClient = &http.Client{Timeout: time.Minute}

// listRepos lists the public repositories of org with the GitHub REST API at api, except forks and disabled
// repositories. If the GITHUB_TOKEN environment variable is set, the requests are authenticated with it, which raises
// the API's rate limit.
func listRepos(api string, org githubOrg) ([]githubRepo, error) {
	token := os.Getenv("GITHUB_TOKEN")
	var repos []githubRepo
	next := api + "/users/" + url.PathEscape(org.name) + "/repos?per_page=100"
	for next != "" {
		request, err := http.NewRequest(http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := apiClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("cannot list the repositories of %s\n%w", org.webURL(), err)
		}
		var page []struct {
			Name           string
			Fork, Disabled bool
		}
		err = decodeAPIResponse(response, &page)
		switch {
		case response.StatusCode == http.StatusNotFound:
			return nil, fmt.Errorf("cannot access %s (organization or user not found)", org.webURL())
		case err != nil && response.Header.Get("X-RateLimit-Remaining") == "0":
			message := "cannot list the repositories of " + org.webURL() + ": GitHub API rate limit exceeded"
			if reset, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				message += ", try again after " + time.Unix(reset, 0).Format("15:04")
			}
			if token == "" {
				message += "\nSet the GITHUB_TOKEN environment variable to a GitHub token to raise the rate limit."
			}
			return nil, errors.New(message)
		case err != nil:
			return nil, fmt.Errorf("cannot list the repositories of %s: %w", org.webURL(), err)
		}
		for _, repo := range page {
			if !repo.Fork && !repo.Disabled {
				repos = append(repos, githubRepo{owner: org.name, name: repo.Name})
			}
		}

		next = nextPageURL(response.Header.Get("Link"))
		// The token must not be sent anywhere else.
		if next != "" && !strings.HasPrefix(next, api+"/") {
			return nil, fmt.Errorf("cannot list the repositories of %s: unexpected next page %s", org.webURL(), next)
		}
	}
	return repos, nil
}

// decodeAPIResponse decodes the JSON body of a successful response into v, closing the body. For any other status, it
// returns an error with the status and the API's message.
func decodeAPIResponse(response *http.Response, v any) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body struct{ Message string }
		json.NewDecoder(response.Body).Decode(&body)
		if body.Message == "" {
			return fmt.Errorf("GitHub API responded %s", response.Status)
		}
		return fmt.Errorf("GitHub API responded %s: %s", response.Status, body.Message)
	}
	if err := json.NewDecoder(response.Body).Decode(v); err != nil {
		return fmt.Errorf("unexpected GitHub API response: %w", err)
	}
	return nil
}

var nextLinkPattern = regexp.MustCompile(`<([^>]*)>\s*;\s*rel="next"`)

// nextPageURL returns the URL of the next page from the Link header of a GitHub API response, or "" on the last page.
func nextPageURL(link string) string {
	if match := nextLinkPattern.FindStringSubmatch(link); match != nil {
		return match[1]
	}
	return ""
}
