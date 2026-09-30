package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// skillMap is the map of a repository's agent skills. The CLI and the web interface present the same map.
type skillMap struct {
	repo   githubRepo
	branch string // empty if the remote HEAD is not a branch
	commit string
	skills []skill // sorted by name
}

// skillGroup is a labeled part of the map as presented. The label is empty if the map is presented without grouping.
type skillGroup struct {
	label  string
	skills []skill
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

	skills, err := loadSkills(checkout)
	if err != nil {
		return skillMap{}, err
	}
	return skillMap{repo: repo, branch: checkout.branch, commit: checkout.commit, skills: skills}, nil
}

// summary describes the map in one line, e.g. "JetBrains/kotlin · master @ c823f9e · 6 skills".
func (m skillMap) summary() string {
	plural := "s"
	if len(m.skills) == 1 {
		plural = ""
	}
	return fmt.Sprintf("%s · %s @ %.7s · %d skill%s", m.repo.slug(), cmp.Or(m.branch, "HEAD"), m.commit, len(m.skills), plural)
}

// groups arranges the skills for presentation. Without grouping, they form a single group with no label.
func (m skillMap) groups(grouping bool) []skillGroup {
	if len(m.skills) == 0 {
		return nil
	}
	if !grouping {
		return []skillGroup{{skills: m.skills}}
	}

	// Skills are grouped by the first word of their names and labeled with the leading words they all share.
	byFirstWord := map[string][]skill{}
	for _, s := range m.skills {
		first, _, _ := strings.Cut(s.name, "-")
		byFirstWord[first] = append(byFirstWord[first], s)
	}
	var groups []skillGroup
	var other []skill
	for _, skills := range byFirstWord {
		if len(skills) == 1 {
			other = append(other, skills[0])
			continue
		}
		groups = append(groups, skillGroup{label: sharedLeadingWords(skills), skills: skills})
	}
	slices.SortFunc(groups, func(a, b skillGroup) int { return cmp.Compare(a.label, b.label) })
	if len(other) > 0 {
		slices.SortFunc(other, compareSkills)
		groups = append(groups, skillGroup{label: "Other", skills: other})
	}
	return groups
}

// sharedLeadingWords returns the leading words, separated by "-", that the names of all skills share.
func sharedLeadingWords(skills []skill) string {
	shared := strings.Split(skills[0].name, "-")
	for _, s := range skills[1:] {
		words := strings.Split(s.name, "-")
		n := 0
		for n < len(shared) && n < len(words) && shared[n] == words[n] {
			n++
		}
		shared = shared[:n]
	}
	return strings.Join(shared, "-")
}

// shownDescription is the skill's description, or a placeholder if it has none.
func (s skill) shownDescription() string {
	if isBlank(s.description) {
		return "(no description)"
	}
	return s.description
}
