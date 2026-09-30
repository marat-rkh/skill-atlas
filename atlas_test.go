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
	want := skillMap{repo: repo, branch: "main", commit: commit, groups: []skillGroup{
		{directory: "skills", skills: []skill{
			{name: "pdf", description: "Work with PDFs.", path: "skills/pdf"},
			{name: "xlsx", path: "skills/xlsx"},
		}},
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
	oneSkill := []skillGroup{{directory: "skills", skills: []skill{{name: "x"}}}}
	twoSkills := []skillGroup{{skills: []skill{{name: "x"}}}, {directory: "skills", skills: []skill{{name: "y"}}}}
	tests := []struct {
		m    skillMap
		want string
	}{
		{skillMap{repo: repo, branch: "main", commit: "0123456789abcdef", groups: twoSkills}, "owner/repo · main @ 0123456 · 2 skills"},
		{skillMap{repo: repo, branch: "main", commit: "0123456789abcdef", groups: oneSkill}, "owner/repo · main @ 0123456 · 1 skill"},
		{skillMap{repo: repo, branch: "main", commit: "0123456789abcdef"}, "owner/repo · main @ 0123456 · 0 skills"},
		{skillMap{repo: repo, commit: "0123456789abcdef", groups: oneSkill}, "owner/repo · HEAD @ 0123456 · 1 skill"},
	}
	for _, tt := range tests {
		if got := tt.m.summary(); got != tt.want {
			t.Errorf("summary() = %q; want %q", got, tt.want)
		}
	}
}

func TestSkillGroupLabel(t *testing.T) {
	if got := (skillGroup{directory: ""}).label(); got != "./" {
		t.Errorf("label() of the root group = %q; want ./", got)
	}
	if got := (skillGroup{directory: ".claude/skills"}).label(); got != ".claude/skills/" {
		t.Errorf("label() = %q; want .claude/skills/", got)
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
