package main

import (
	"reflect"
	"strings"
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

	if got, want := m.groups(false, nil), []skillGroup{{skills: m.skills}}; !reflect.DeepEqual(got, want) {
		t.Errorf("groups(false, nil) =\n%+v\nwant\n%+v", got, want)
	}
	want := []skillGroup{
		{label: "analysis-api", skills: []skill{{name: "analysis-api-create-issue"}, {name: "analysis-api-mark-apis"}}},
		{label: "build", skills: []skill{{name: "build-bump-gradle"}, {name: "build-tools-bump-api"}}},
		{label: "git", skills: []skill{{name: "git"}, {name: "git-commit"}}},
		{label: "Other", skills: []skill{{name: "docx"}, {name: "pdf"}}},
	}
	if got := m.groups(true, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("groups(true, nil) =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSkillMapGroupsWithoutSkills(t *testing.T) {
	for _, grouping := range []bool{false, true} {
		if got := (skillMap{}).groups(grouping, nil); got != nil {
			t.Errorf("groups(%v, nil) = %+v; want none", grouping, got)
		}
	}
}

func TestSkillMapGroupsWithoutSharedFirstWords(t *testing.T) {
	m := skillMap{skills: []skill{{name: "debug"}, {name: "review-code"}}}
	want := []skillGroup{{label: "Other", skills: m.skills}}
	if got := m.groups(true, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("groups(true, nil) =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSkillMapGroupsWithStarredSkills(t *testing.T) {
	createIssue := skill{name: "analysis-api-create-issue", path: "skills/create-issue"}
	markAPIs := skill{name: "analysis-api-mark-apis", path: "skills/mark-apis"}
	bumpGradle := skill{name: "build-bump-gradle", path: "skills/bump-gradle"}
	bumpAPI := skill{name: "build-tools-bump-api", path: "skills/bump-api"}
	docx := skill{name: "docx", path: "skills/docx"}
	git := skill{name: "git", path: "skills/git"}
	gitCommit := skill{name: "git-commit", path: "skills/git-commit"}
	pdfA := skill{name: "pdf", path: "a/pdf"}
	pdfB := skill{name: "pdf", path: "b/pdf"}
	xlsx := skill{name: "xlsx", path: "skills/xlsx"}
	m := skillMap{skills: []skill{createIssue, markAPIs, bumpGradle, bumpAPI, docx, git, gitCommit, pdfA, pdfB, xlsx}}
	// Stars belong to paths, so only one of the pdf skills is starred. The star of a skill that is gone is ignored.
	starred := map[string]bool{markAPIs.path: true, docx.path: true, git.path: true, gitCommit.path: true, pdfB.path: true,
		"skills/removed": true}

	want := []skillGroup{{skills: []skill{markAPIs, docx, git, gitCommit, pdfB, createIssue, bumpGradle, bumpAPI, pdfA, xlsx}}}
	if got := m.groups(false, starred); !reflect.DeepEqual(got, want) {
		t.Errorf("groups(false, starred) =\n%+v\nwant\n%+v", got, want)
	}
	// The other groups keep the labels they have without stars, and the git group, whose skills are all starred, is gone.
	want = []skillGroup{
		{label: "Starred", skills: []skill{markAPIs, docx, git, gitCommit, pdfB}},
		{label: "analysis-api", skills: []skill{createIssue}},
		{label: "build", skills: []skill{bumpGradle, bumpAPI}},
		{label: "pdf", skills: []skill{pdfA}},
		{label: "Other", skills: []skill{xlsx}},
	}
	if got := m.groups(true, starred); !reflect.DeepEqual(got, want) {
		t.Errorf("groups(true, starred) =\n%+v\nwant\n%+v", got, want)
	}
	if !reflect.DeepEqual(m.skills, []skill{createIssue, markAPIs, bumpGradle, bumpAPI, docx, git, gitCommit, pdfA, pdfB, xlsx}) {
		t.Errorf("groups() changed the map's skills: %+v", m.skills)
	}
}

func TestSkillMapGroupsWithAllSkillsStarred(t *testing.T) {
	m := skillMap{skills: []skill{{name: "git", path: "git"}, {name: "git-commit", path: "git-commit"}, {name: "pdf", path: "pdf"}}}
	starred := map[string]bool{"git": true, "git-commit": true, "pdf": true}

	if got, want := m.groups(false, starred), []skillGroup{{skills: m.skills}}; !reflect.DeepEqual(got, want) {
		t.Errorf("groups(false, starred) =\n%+v\nwant\n%+v", got, want)
	}
	if got, want := m.groups(true, starred), []skillGroup{{label: "Starred", skills: m.skills}}; !reflect.DeepEqual(got, want) {
		t.Errorf("groups(true, starred) =\n%+v\nwant\n%+v", got, want)
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
