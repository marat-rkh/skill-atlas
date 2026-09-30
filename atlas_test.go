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
