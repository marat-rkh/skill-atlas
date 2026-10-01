package main

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestScanRemote(t *testing.T) {
	remote, commit := newRemote(t, map[string]string{
		"skills/pdf/SKILL.md":  "---\nname: pdf\ndescription: Work with PDFs.\n---\n",
		"skills/xlsx/SKILL.md": "---\nname: xlsx\n---\n",
		"README.md":            "# Skills",
	})
	repo := githubRepo{"owner", "repo"}

	got, err := scanRemote(repo, remote)
	if err != nil {
		t.Fatal(err)
	}
	want := skillMap{repo: repo, branch: "main", commit: commit, skills: []skill{
		{name: "pdf", description: "Work with PDFs.", path: "skills/pdf"},
		{name: "xlsx", path: "skills/xlsx"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scanRemote() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestScanRemoteReportsErrors(t *testing.T) {
	_, err := scanRemote(githubRepo{"owner", "repo"}, "file://"+t.TempDir()+"/missing.git")
	if err == nil || !strings.Contains(err.Error(), "cannot access") {
		t.Errorf("scanRemote() error = %v; want a 'cannot access' error", err)
	}
}

func TestScanOrganizationAt(t *testing.T) {
	alpha, alphaCommit := newRemote(t, map[string]string{
		"skills/pdf/SKILL.md": "---\nname: pdf\ndescription: Work with PDFs.\n---\n",
		"SKILL.md":            "---\nname: alpha\n---\n",
	})
	beta, betaCommit := newRemote(t, map[string]string{"skills/xlsx/SKILL.md": "---\nname: xlsx\n---\n"})
	noSkills, _ := newRemote(t, map[string]string{"README.md": "# No skills"})
	remotes := map[string]string{
		"Alpha":     alpha,
		"beta":      beta,
		"no-skills": noSkills,
		"empty":     newEmptyRemote(t),
		"missing":   "file://" + t.TempDir() + "/missing.git",
	}
	// beta is listed twice, as if a repository created while listing shifted it to the next page.
	api := newFakeGitHubAPI(t, "owner",
		`[{"name": "beta"}, {"name": "no-skills"}, {"name": "missing"}, {"name": "forked", "fork": true}]`,
		`[{"name": "beta"}, {"name": "empty"}, {"name": "Alpha"}]`,
	)
	var mu sync.Mutex
	var requested []string
	remote := func(repo githubRepo) string {
		mu.Lock()
		defer mu.Unlock()
		requested = append(requested, repo.slug())
		return remotes[repo.name]
	}

	got, err := scanOrganizationAt(api.URL, githubOrg{"owner"}, remote)
	if err != nil {
		t.Fatal(err)
	}
	want := orgMap{org: githubOrg{"owner"}, repoCount: 5, maps: []skillMap{
		{repo: githubRepo{"owner", "Alpha"}, branch: "main", commit: alphaCommit, skills: []skill{
			{name: "alpha", path: ""},
			{name: "pdf", description: "Work with PDFs.", path: "skills/pdf"},
		}},
		{repo: githubRepo{"owner", "beta"}, branch: "main", commit: betaCommit, skills: []skill{
			{name: "xlsx", path: "skills/xlsx"},
		}},
	}}
	failures := got.failures
	got.failures = nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scanOrganizationAt() =\n%+v\nwant\n%+v", got, want)
	}
	if want := "owner/missing: cannot access " + strings.TrimSuffix(remotes["missing"], ".git") + " (repository not found or private)\n"; len(failures) != 1 || !strings.HasPrefix(failures[0].Error(), want) {
		t.Errorf("failures = %q; want one with prefix %q", failures, want)
	}
	slices.Sort(requested)
	if want := []string{"owner/Alpha", "owner/beta", "owner/empty", "owner/missing", "owner/no-skills"}; !slices.Equal(requested, want) {
		t.Errorf("fetched %q; want %q", requested, want)
	}
}

func TestScanOrganizationAtAnalyzesManyRepositoriesInOrder(t *testing.T) {
	remote, _ := newRemote(t, map[string]string{"SKILL.md": "---\nname: x\n---\n"})
	// More repositories than are analyzed at a time, listed in reverse order.
	var repos, wantNames []string
	for i := 3 * parallelScans; i > 0; i-- {
		repos = append(repos, fmt.Sprintf(`{"name": "repo-%03d"}`, i))
		wantNames = append([]string{fmt.Sprintf("repo-%03d", i)}, wantNames...)
	}
	api := newFakeGitHubAPI(t, "owner", "["+strings.Join(repos, ",")+"]")

	got, err := scanOrganizationAt(api.URL, githubOrg{"owner"}, func(githubRepo) string { return remote })
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range got.maps {
		names = append(names, m.repo.name)
	}
	if !slices.Equal(names, wantNames) || got.repoCount != len(wantNames) || len(got.failures) != 0 {
		t.Errorf("scanOrganizationAt() = %d repositories, maps of %q, failures %q; want %d, maps of %q, no failures",
			got.repoCount, names, got.failures, len(wantNames), wantNames)
	}
}

func TestScanOrganizationAtReportsListingErrors(t *testing.T) {
	api := newFakeGitHubAPI(t, "owner", `[]`)

	_, err := scanOrganizationAt(api.URL, githubOrg{"missing"}, githubRepo.cloneURL)
	if want := "cannot access https://github.com/missing (organization or user not found)"; err == nil || err.Error() != want {
		t.Errorf("scanOrganizationAt() error = %v; want %q", err, want)
	}
}

func TestCompareRepos(t *testing.T) {
	repos := []githubRepo{{"o", "beta"}, {"o", "Alpha"}, {"o", "alpha"}, {"o", "Beta"}, {"o", "_x"}, {"o", "a"}}
	slices.SortFunc(repos, compareRepos)
	want := []githubRepo{{"o", "_x"}, {"o", "a"}, {"o", "Alpha"}, {"o", "alpha"}, {"o", "Beta"}, {"o", "beta"}}
	if !slices.Equal(repos, want) {
		t.Errorf("sorted = %v; want %v", repos, want)
	}
}

func TestOrgMapSummary(t *testing.T) {
	org := githubOrg{"JetBrains"}
	repoWith := func(n int) skillMap { return skillMap{skills: make([]skill, n)} }
	tests := []struct {
		m    orgMap
		want string
	}{
		{orgMap{org: org, repoCount: 683, maps: []skillMap{repoWith(6), repoWith(2)}}, "JetBrains · 683 repositories · 2 with skills · 8 skills"},
		{orgMap{org: org, repoCount: 1, maps: []skillMap{repoWith(1)}}, "JetBrains · 1 repository · 1 with skills · 1 skill"},
		{orgMap{org: org, repoCount: 2}, "JetBrains · 2 repositories · 0 with skills · 0 skills"},
		{orgMap{org: org}, "JetBrains · 0 repositories · 0 with skills · 0 skills"},
	}
	for _, tt := range tests {
		if got := tt.m.summary(); got != tt.want {
			t.Errorf("summary() = %q; want %q", got, tt.want)
		}
	}
}

func TestSkillMapSummary(t *testing.T) {
	repo := githubRepo{"owner", "repo"}
	oneSkill := []skill{{name: "x"}}
	twoSkills := []skill{{name: "x"}, {name: "y"}}
	tests := []struct {
		m    skillMap
		want string
	}{
		{skillMap{repo: repo, branch: "main", commit: "0123456789abcdef", skills: twoSkills}, "owner/repo · main @ 0123456 · 2 skills"},
		{skillMap{repo: repo, branch: "main", commit: "0123456789abcdef", skills: oneSkill}, "owner/repo · main @ 0123456 · 1 skill"},
		{skillMap{repo: repo, branch: "main", commit: "0123456789abcdef"}, "owner/repo · main @ 0123456 · 0 skills"},
		{skillMap{repo: repo, commit: "0123456789abcdef", skills: oneSkill}, "owner/repo · HEAD @ 0123456 · 1 skill"},
	}
	for _, tt := range tests {
		if got := tt.m.summary(); got != tt.want {
			t.Errorf("summary() = %q; want %q", got, tt.want)
		}
	}
}

func TestSkillMapGroups(t *testing.T) {
	m := skillMap{skills: []skill{
		{name: "analysis-api-create-issue"},
		{name: "analysis-api-mark-apis"},
		{name: "build-bump-gradle"},
		{name: "build-tools-bump-api"},
		{name: "docx"},
		{name: "git"},
		{name: "git-commit"},
		{name: "pdf"},
	}}

	if got, want := m.groups(false), []skillGroup{{skills: m.skills}}; !reflect.DeepEqual(got, want) {
		t.Errorf("groups(false) =\n%+v\nwant\n%+v", got, want)
	}
	want := []skillGroup{
		{label: "analysis-api", skills: []skill{{name: "analysis-api-create-issue"}, {name: "analysis-api-mark-apis"}}},
		{label: "build", skills: []skill{{name: "build-bump-gradle"}, {name: "build-tools-bump-api"}}},
		{label: "git", skills: []skill{{name: "git"}, {name: "git-commit"}}},
		{label: "Other", skills: []skill{{name: "docx"}, {name: "pdf"}}},
	}
	if got := m.groups(true); !reflect.DeepEqual(got, want) {
		t.Errorf("groups(true) =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSkillMapGroupsWithoutSkills(t *testing.T) {
	for _, grouping := range []bool{false, true} {
		if got := (skillMap{}).groups(grouping); got != nil {
			t.Errorf("groups(%v) = %+v; want none", grouping, got)
		}
	}
}

func TestSkillMapGroupsWithoutSharedFirstWords(t *testing.T) {
	m := skillMap{skills: []skill{{name: "debug"}, {name: "review-code"}}}
	want := []skillGroup{{label: "Other", skills: m.skills}}
	if got := m.groups(true); !reflect.DeepEqual(got, want) {
		t.Errorf("groups(true) =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSkillShownDescription(t *testing.T) {
	if got := (skill{description: "Does things."}).shownDescription(); got != "Does things." {
		t.Errorf("shownDescription() = %q", got)
	}
	if got := (skill{description: " \n"}).shownDescription(); got != "(no description)" {
		t.Errorf("shownDescription() of a blank description = %q; want (no description)", got)
	}
}
