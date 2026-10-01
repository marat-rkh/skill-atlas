package main

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// skillMap is the map of a repository's agent skills. The CLI and the web interface present the same map.
type skillMap struct {
	repo   githubRepo
	branch string // empty if the remote HEAD is not a branch
	commit string
	skills []skill // sorted by name
}

// orgMap is the map of a GitHub organization's agent skills: the maps of its repositories that have skills.
type orgMap struct {
	org       githubOrg
	repoCount int        // how many repositories were analyzed, including those without skills and the failed ones
	maps      []skillMap // sorted by repository name, ignoring case
	failures  []error    // one for each repository that could not be analyzed
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

// parallelScans is how many repositories of an organization are analyzed at a time. Analyzing a repository mostly
// waits for the network, so this is more than the number of processors.
const parallelScans = 32

// scanOrganization builds the map of a GitHub organization from its public repositories, except forks.
func scanOrganization(org githubOrg) (orgMap, error) {
	return scanOrganizationAt(githubAPI, org, githubRepo.cloneURL)
}

// scanOrganizationAt builds the map of org, listing its repositories with the GitHub API at api and fetching each
// repository from remote(repo). Empty repositories have no skills. A repository that cannot be analyzed doesn't stop
// the others; it is reported in the map's failures.
func scanOrganizationAt(api string, org githubOrg, remote func(githubRepo) string) (orgMap, error) {
	repos, err := listRepos(api, org)
	if err != nil {
		return orgMap{}, err
	}
	slices.SortFunc(repos, compareRepos)
	// Pages of the list can overlap if repositories are created while it is listed.
	repos = slices.Compact(repos)

	maps := make([]skillMap, len(repos))
	errs := make([]error, len(repos))
	slots := make(chan struct{}, parallelScans)
	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			maps[i], errs[i] = scanRemote(repo, remote(repo))
		})
	}
	wg.Wait()

	m := orgMap{org: org, repoCount: len(repos)}
	for i, repo := range repos {
		switch {
		case errors.Is(errs[i], errEmptyRepository):
		case errs[i] != nil:
			m.failures = append(m.failures, fmt.Errorf("%s: %w", repo.slug(), errs[i]))
		case len(maps[i].skills) > 0:
			m.maps = append(m.maps, maps[i])
		}
	}
	return m, nil
}

// compareRepos orders repositories by name, ignoring case, and then by the exact name.
func compareRepos(a, b githubRepo) int {
	return cmp.Or(cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name)), cmp.Compare(a.name, b.name))
}

// summary describes the map in one line, e.g. "JetBrains/kotlin · master @ c823f9e · 6 skills".
func (m skillMap) summary() string {
	skills := count(len(m.skills), "skill", "skills")
	return fmt.Sprintf("%s · %s @ %.7s · %s", m.repo.slug(), cmp.Or(m.branch, "HEAD"), m.commit, skills)
}

// summary describes the map in one line, e.g. "JetBrains · 683 repositories · 26 with skills · 412 skills".
func (m orgMap) summary() string {
	skills := 0
	for _, repo := range m.maps {
		skills += len(repo.skills)
	}
	return fmt.Sprintf("%s · %s · %d with skills · %s",
		m.org.name, count(m.repoCount, "repository", "repositories"), len(m.maps), count(skills, "skill", "skills"))
}

// count describes n things in words, e.g. "1 skill" or "2 skills".
func count(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
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
