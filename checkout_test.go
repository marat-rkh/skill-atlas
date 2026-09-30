package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCheckoutSkillFiles(t *testing.T) {
	remote, commit := newRemote(t, map[string]string{
		"README.md":                    "# Repo",
		"skills/pdf/SKILL.md":          "---\nname: pdf\n---\n",
		"skills/pdf/reference.md":      "Details",
		".claude/skills/lint/SKILL.md": "---\nname: lint\n---\n",
		"SKILL.md":                     "---\nname: root\n---\n",
		"skills/we*ird [x]/SKILL.md":   "---\nname: weird\n---\n",
		"docs/skill.md":                "not a skill: wrong case",
		"docs/NOT_SKILL.md":            "not a skill: different name",
		"linked/SKILL.md":              "->../skills/pdf/SKILL.md",
	})

	checkout, err := checkoutSkillFiles(remote)
	if err != nil {
		t.Fatal(err)
	}
	defer checkout.close()

	if checkout.branch != "main" {
		t.Errorf("branch = %q; want main", checkout.branch)
	}
	if checkout.commit != commit {
		t.Errorf("commit = %q; want %q", checkout.commit, commit)
	}
	wantFiles := []string{".claude/skills/lint/SKILL.md", "SKILL.md", "skills/pdf/SKILL.md", "skills/we*ird [x]/SKILL.md"}
	if got := slices.Sorted(slices.Values(checkout.skillFiles)); !slices.Equal(got, wantFiles) {
		t.Errorf("skillFiles = %q; want %q", got, wantFiles)
	}
	if got := checkedOutFiles(t, checkout.dir); !slices.Equal(got, wantFiles) {
		t.Errorf("files in the checkout = %q; want only the skill files %q", got, wantFiles)
	}
	for path, want := range map[string]string{
		"skills/pdf/SKILL.md":        "---\nname: pdf\n---\n",
		"skills/we*ird [x]/SKILL.md": "---\nname: weird\n---\n",
	} {
		if got, err := checkout.read(path); err != nil || got != want {
			t.Errorf("read(%q) = %q, %v; want %q", path, got, err, want)
		}
	}

	checkout.close()
	if _, err := os.Stat(checkout.dir); !os.IsNotExist(err) {
		t.Errorf("checkout directory still exists after close: %v", err)
	}
}

func TestCheckoutSkillFilesWithoutSkills(t *testing.T) {
	remote, _ := newRemote(t, map[string]string{"README.md": "# Nothing here"})

	checkout, err := checkoutSkillFiles(remote)
	if err != nil {
		t.Fatal(err)
	}
	defer checkout.close()

	if len(checkout.skillFiles) != 0 {
		t.Errorf("skillFiles = %q; want none", checkout.skillFiles)
	}
}

func TestCheckoutSkillFilesReportsInaccessibleRepository(t *testing.T) {
	remote := "file://" + filepath.ToSlash(filepath.Join(t.TempDir(), "missing.git"))

	checkout, err := checkoutSkillFiles(remote)
	if err == nil {
		checkout.close()
		t.Fatal("checkoutSkillFiles() succeeded for a missing repository")
	}
	want := "cannot access " + strings.TrimSuffix(remote, ".git") + " (repository not found or private)"
	if !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q; want prefix %q", err, want)
	}
}

func TestSparsePattern(t *testing.T) {
	tests := []struct{ path, want string }{
		{"skills/pdf/SKILL.md", "/skills/pdf/SKILL.md"},
		{`a*b?c[d]\e/SKILL.md`, `/a\*b\?c\[d]\\e/SKILL.md`},
	}
	for _, tt := range tests {
		if got := sparsePattern(tt.path); got != tt.want {
			t.Errorf("sparsePattern(%q) = %q; want %q", tt.path, got, tt.want)
		}
	}
}

// newRemote commits files to a new local repository on branch main and returns its URL and the commit hash.
// A file whose content starts with "->" becomes a symlink to the rest of the content.
func newRemote(t *testing.T, files map[string]string) (url, commit string) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	git("init", "-q", "-b", "main")
	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if target, ok := strings.CutPrefix(content, "->"); ok {
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, full); err != nil {
				t.Fatal(err)
			}
		} else {
			writeFile(t, full, content)
		}
	}
	git("add", "-A")
	git("commit", "-q", "-m", "test")
	// Serve partial and by-object fetches, as GitHub does.
	git("config", "uploadpack.allowFilter", "true")
	git("config", "uploadpack.allowAnySHA1InWant", "true")
	return "file://" + filepath.ToSlash(dir), git("rev-parse", "HEAD")
}

// checkedOutFiles lists the files in a working tree, excluding .git, as sorted slash-separated relative paths.
func checkedOutFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	return files
}
