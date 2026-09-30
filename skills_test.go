package main

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{
			name:    "plain scalars",
			content: "---\nname: pdf\ndescription: Work with PDF files.\n---\n# PDF\n",
			want:    map[string]string{"name": "pdf", "description": "Work with PDF files."},
		},
		{
			name:    "value containing a colon",
			content: "---\ndescription: Use when: the user asks\n---\n",
			want:    map[string]string{"description": "Use when: the user asks"},
		},
		{
			name:    "double-quoted with escapes",
			content: "---\ndescription: \"Say \\\"hi\\\"\\tthen\\\\stop: now\"\n---\n",
			want:    map[string]string{"description": "Say \"hi\"\tthen\\stop: now"},
		},
		{
			name:    "single-quoted",
			content: "---\ndescription: 'It''s here: really'\n---\n",
			want:    map[string]string{"description": "It's here: really"},
		},
		{
			name:    "plain value continued on indented lines",
			content: "---\ndescription: First line\n  second line\n\n  third line\nname: x\n---\n",
			want:    map[string]string{"description": "First line second line third line", "name": "x"},
		},
		{
			name:    "folded block scalar",
			content: "---\ndescription: >\n  Folded into\n  one line.\n\nname: x\n---\n",
			want:    map[string]string{"description": "Folded into one line.", "name": "x"},
		},
		{
			name:    "literal block scalar with chomping indicator",
			content: "---\ndescription: |-\n  Line one.\n  Line two.\n---\n",
			want:    map[string]string{"description": "Line one.\nLine two."},
		},
		{
			name:    "nested maps are not interpreted",
			content: "---\nname: x\nmetadata:\n  short-description: Short\n---\n",
			want:    map[string]string{"name": "x", "metadata": "short-description: Short"},
		},
		{
			name:    "comments are skipped",
			content: "---\n# a comment\nname: x\n---\n",
			want:    map[string]string{"name": "x"},
		},
		{
			name:    "CRLF line endings and byte order mark",
			content: "\uFEFF---\r\nname: x\r\ndescription: y\r\n---\r\n",
			want:    map[string]string{"name": "x", "description": "y"},
		},
		{
			name:    "delimiters with trailing spaces",
			content: "---  \nname: x\n--- \n",
			want:    map[string]string{"name": "x"},
		},
		{
			name:    "body delimiters are not frontmatter",
			content: "---\nname: x\n---\n\n---\nname: y\n---\n",
			want:    map[string]string{"name": "x"},
		},
		{
			name:    "no frontmatter",
			content: "# Title\n\nname: x\n",
			want:    map[string]string{},
		},
		{
			name:    "unterminated frontmatter",
			content: "---\nname: x\n",
			want:    map[string]string{},
		},
		{
			name:    "empty file",
			content: "",
			want:    map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseFrontmatter(tt.content); !maps.Equal(got, tt.want) {
				t.Errorf("parseFrontmatter() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestParseSkill(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		want    skill
	}{
		{
			name:    "name and description from frontmatter",
			path:    "skills/pdf-tools",
			content: "---\nname: pdf\ndescription: Work with PDFs.\n---\n",
			want:    skill{name: "pdf", description: "Work with PDFs.", path: "skills/pdf-tools"},
		},
		{
			name:    "missing name falls back to the directory name",
			path:    "skills/pdf",
			content: "---\ndescription: Work with PDFs.\n---\n",
			want:    skill{name: "pdf", description: "Work with PDFs.", path: "skills/pdf"},
		},
		{
			name:    "blank name falls back to the directory name",
			path:    "pdf",
			content: "---\nname: \"  \"\n---\n",
			want:    skill{name: "pdf", path: "pdf"},
		},
		{
			name:    "skill at the repository root without a name",
			path:    "",
			content: "# Just instructions\n",
			want:    skill{name: "(unnamed)", path: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseSkill(tt.path, tt.content); got != tt.want {
				t.Errorf("parseSkill() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadSkills(t *testing.T) {
	files := map[string]string{
		"SKILL.md":                  "---\nname: root\n---\n",
		"skills/zeta/SKILL.md":      "---\nname: zeta\n---\n",
		"skills/alpha/SKILL.md":     "---\nname: alpha\ndescription: First.\n---\n",
		"skills/b/SKILL.md":         "---\nname: b\n---\n",
		".claude/skills/b/SKILL.md": "---\nname: b\n---\n",
		"top/SKILL.md":              "---\nname: top\n---\n",
	}
	dir := t.TempDir()
	var skillFiles []string
	for path, content := range files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(path)), content)
		skillFiles = append(skillFiles, path)
	}

	skills, err := loadSkills(&repoCheckout{dir: dir, skillFiles: skillFiles})
	if err != nil {
		t.Fatal(err)
	}
	want := []skill{
		{name: "alpha", description: "First.", path: "skills/alpha"},
		{name: "b", path: ".claude/skills/b"},
		{name: "b", path: "skills/b"},
		{name: "root", path: ""},
		{name: "top", path: "top"},
		{name: "zeta", path: "skills/zeta"},
	}
	if !reflect.DeepEqual(skills, want) {
		t.Errorf("loadSkills() =\n%+v\nwant\n%+v", skills, want)
	}
}

func TestLoadSkillsReportsUnreadableFiles(t *testing.T) {
	_, err := loadSkills(&repoCheckout{dir: t.TempDir(), skillFiles: []string{"missing/SKILL.md"}})
	if err == nil {
		t.Fatal("loadSkills() succeeded for a missing file")
	}
}

func TestPathHelpers(t *testing.T) {
	tests := []struct{ path, parent, last string }{
		{"a/b/c", "a/b", "c"},
		{"a", "", "a"},
		{"", "", ""},
		{".claude/skills/x", ".claude/skills", "x"},
	}
	for _, tt := range tests {
		if got := parentDir(tt.path); got != tt.parent {
			t.Errorf("parentDir(%q) = %q; want %q", tt.path, got, tt.parent)
		}
		if got := lastSegment(tt.path); got != tt.last {
			t.Errorf("lastSegment(%q) = %q; want %q", tt.path, got, tt.last)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
