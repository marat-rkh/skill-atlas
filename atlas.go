package main

import (
	"cmp"
	"fmt"
)

// skillMap is the map of a repository's agent skills. The CLI and the web interface present the same map.
type skillMap struct {
	repo   githubRepo
	branch string // empty if the remote HEAD is not a branch
	commit string
	groups []skillGroup
}

// scanRepository builds the skill map of a GitHub repository.
func scanRepository(repo githubRepo) (skillMap, error) {
	return scanRemote(repo, repo.cloneURL())
}

// scanRemote builds the skill map of repo, fetching its contents from remote.
func scanRemote(repo githubRepo, remote string) (skillMap, error) {
	checkout, err := checkoutSkillFiles(remote)
	if err != nil {
		return skillMap{}, err
	}
	defer checkout.close()

	groups, err := loadSkillGroups(checkout)
	if err != nil {
		return skillMap{}, err
	}
	return skillMap{repo: repo, branch: checkout.branch, commit: checkout.commit, groups: groups}, nil
}

// summary describes the map in one line, e.g. "JetBrains/kotlin · master @ c823f9e · 6 skills".
func (m skillMap) summary() string {
	count := 0
	for _, group := range m.groups {
		count += len(group.skills)
	}
	plural := "s"
	if count == 1 {
		plural = ""
	}
	return fmt.Sprintf("%s · %s @ %.7s · %d skill%s", m.repo.slug(), cmp.Or(m.branch, "HEAD"), m.commit, count, plural)
}

// label is how the group's directory is shown: "./" for the repository root, otherwise the directory and a slash.
func (g skillGroup) label() string {
	if g.directory == "" {
		return "./"
	}
	return g.directory + "/"
}

// shownDescription is the skill's description, or a placeholder if it has none.
func (s skill) shownDescription() string {
	if isBlank(s.description) {
		return "(no description)"
	}
	return s.description
}
