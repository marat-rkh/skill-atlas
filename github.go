package main

import (
	"regexp"
	"strings"
)

// githubRepo is a GitHub repository identified by its owner and name.
type githubRepo struct {
	owner, name string
}

func (r githubRepo) slug() string     { return r.owner + "/" + r.name }
func (r githubRepo) webURL() string   { return "https://github.com/" + r.slug() }
func (r githubRepo) cloneURL() string { return r.webURL() + ".git" }

// Accepts https://github.com/o/r, github.com/o/r, git@github.com:o/r.git; anything after o/r is ignored.
var githubURLPattern = regexp.MustCompile(
	`(?i)^(?:(?:https?://)?(?:www\.)?github\.com/|git@github\.com:)([\w.-]+)/([\w.-]+?)(?:\.git)?(?:[/?#].*)?$`,
)

func parseGitHubRepo(input string) (githubRepo, bool) {
	match := githubURLPattern.FindStringSubmatch(strings.TrimSpace(input))
	if match == nil {
		return githubRepo{}, false
	}
	return githubRepo{owner: match[1], name: match[2]}, true
}
