package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// openTestStars opens the stars saved at path.
func openTestStars(t *testing.T, path string) *starStore {
	t.Helper()
	stars, err := openStarStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return stars
}

// setStarred stars or unstars the skill at path in repo.
func setStarred(t *testing.T, stars *starStore, repo githubRepo, path string, starred bool) {
	t.Helper()
	if err := stars.setStarred(repo, path, starred); err != nil {
		t.Fatal(err)
	}
}

func assertStarred(t *testing.T, stars *starStore, repo githubRepo, want map[string]bool) {
	t.Helper()
	if got := stars.starred(repo); !reflect.DeepEqual(got, want) {
		t.Errorf("starred(%v) = %v; want %v", repo, got, want)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("%s =\n%s\nwant\n%s", path, data, want)
	}
}

func TestStarStoreSavesStars(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill-atlas", "stars.json")
	stars := openTestStars(t, path)
	assertStarred(t, stars, kotlin, map[string]bool{})
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("opening the stars created %s", path)
	}

	other := githubRepo{"Owner", "Repo"}
	setStarred(t, stars, kotlin, "skills/b", true)
	setStarred(t, stars, kotlin, "skills/a", true)
	setStarred(t, stars, kotlin, "", true)
	setStarred(t, stars, other, "skills/a", true)
	assertStarred(t, stars, kotlin, map[string]bool{"": true, "skills/a": true, "skills/b": true})
	assertStarred(t, stars, other, map[string]bool{"skills/a": true})
	assertFileContent(t, path, `{
  "jetbrains/kotlin": [
    "",
    "skills/a",
    "skills/b"
  ],
  "owner/repo": [
    "skills/a"
  ]
}
`)

	reopened := openTestStars(t, path)
	assertStarred(t, reopened, kotlin, map[string]bool{"": true, "skills/a": true, "skills/b": true})
	assertStarred(t, reopened, other, map[string]bool{"skills/a": true})

	setStarred(t, reopened, kotlin, "skills/a", false)
	setStarred(t, reopened, other, "skills/a", false)
	assertStarred(t, reopened, kotlin, map[string]bool{"": true, "skills/b": true})
	assertStarred(t, reopened, other, map[string]bool{})
	assertFileContent(t, path, `{
  "jetbrains/kotlin": [
    "",
    "skills/b"
  ]
}
`)
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("the directory of the stars has %d entries; want only stars.json", len(entries))
	}
}

func TestStarStoreStarsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stars.json")
	stars := openTestStars(t, path)
	setStarred(t, stars, kotlin, "skills/a", true)
	setStarred(t, stars, kotlin, "skills/a", true)
	setStarred(t, stars, kotlin, "skills/b", false)
	assertFileContent(t, path, "{\n  \"jetbrains/kotlin\": [\n    \"skills/a\"\n  ]\n}\n")

	setStarred(t, stars, kotlin, "skills/a", false)
	setStarred(t, stars, kotlin, "skills/a", false)
	assertFileContent(t, path, "{}\n")
}

func TestStarStoreIgnoresRepositoryCase(t *testing.T) {
	stars := openTestStars(t, filepath.Join(t.TempDir(), "stars.json"))
	setStarred(t, stars, githubRepo{"jetbrains", "KOTLIN"}, "skills/a", true)
	assertStarred(t, stars, kotlin, map[string]bool{"skills/a": true})
	setStarred(t, stars, kotlin, "skills/a", false)
	assertStarred(t, stars, githubRepo{"jetbrains", "KOTLIN"}, map[string]bool{})
}

func TestStarStoreReadsFile(t *testing.T) {
	tests := []struct {
		content string
		want    map[string]bool
	}{
		{`{"jetbrains/kotlin": ["skills/a", "skills/b"], "owner/repo": ["skills/c"]}`, map[string]bool{"skills/a": true, "skills/b": true}},
		{`{"jetbrains/kotlin": null}`, map[string]bool{}},
		{`{}`, map[string]bool{}},
		{`null`, map[string]bool{}},
	}
	for _, tt := range tests {
		path := filepath.Join(t.TempDir(), "stars.json")
		if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
			t.Fatal(err)
		}
		stars := openTestStars(t, path)
		assertStarred(t, stars, kotlin, tt.want)

		// The stars read from the file can be changed and saved again.
		setStarred(t, stars, kotlin, "skills/new", true)
		tt.want["skills/new"] = true
		assertStarred(t, openTestStars(t, path), kotlin, tt.want)
	}
}

func TestStarStoreReportsInvalidFile(t *testing.T) {
	for _, content := range []string{"", "{", `["skills/a"]`, `{"jetbrains/kotlin": "skills/a"}`} {
		path := filepath.Join(t.TempDir(), "stars.json")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := openStarStore(path)
		if want := "cannot read stars from " + path + ": "; err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("openStarStore() of %q: error = %v; want one starting with %q", content, err, want)
		}
	}
}

func TestStarStoreReportsUnreadableFile(t *testing.T) {
	path := t.TempDir() // a directory cannot be read as a file
	_, err := openStarStore(path)
	if err == nil || !strings.HasPrefix(err.Error(), "cannot read stars: ") || !strings.Contains(err.Error(), path) {
		t.Errorf("openStarStore() error = %v; want a 'cannot read stars' error naming %s", err, path)
	}
}

func TestStarStoreKeepsStarsThatCannotBeSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stars.json")
	stars := openTestStars(t, path)
	setStarred(t, stars, kotlin, "skills/a", true)
	// A directory in place of the file makes saving fail.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, change := range []struct {
		path    string
		starred bool
	}{{"skills/b", true}, {"skills/a", false}} {
		err := stars.setStarred(kotlin, change.path, change.starred)
		if err == nil || !strings.HasPrefix(err.Error(), "cannot save stars: ") {
			t.Errorf("setStarred(%q, %v) error = %v; want a 'cannot save stars' error", change.path, change.starred, err)
		}
		assertStarred(t, stars, kotlin, map[string]bool{"skills/a": true})
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("failed saves left %d entries in the directory of the stars; want only stars.json", len(entries))
	}
}

func TestStarStoreSavesConcurrentStars(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stars.json")
	stars := openTestStars(t, path)
	want := map[string]bool{}
	var wg sync.WaitGroup
	for i := range 20 {
		skillPath := fmt.Sprintf("skills/%02d", i)
		want[skillPath] = true
		wg.Go(func() {
			if err := stars.setStarred(kotlin, skillPath, true); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	assertStarred(t, stars, kotlin, want)
	assertStarred(t, openTestStars(t, path), kotlin, want)
}

// useTempConfigDir points the user's configuration directory, on every platform, into a new temporary directory and
// returns it.
func useTempConfigDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return configDir
}

func TestOpenUserStarStore(t *testing.T) {
	configDir := useTempConfigDir(t)
	stars, err := openUserStarStore()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(configDir, "skill-atlas", "stars.json"); stars.path != want {
		t.Errorf("stars are saved in %s; want %s", stars.path, want)
	}
	setStarred(t, stars, kotlin, "skills/a", true)
	if _, err := os.Stat(filepath.Join(configDir, "skill-atlas", "stars.json")); err != nil {
		t.Error(err)
	}
}

func TestOpenUserStarStoreWithoutConfigurationDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	t.Setenv("home", "") // Plan 9
	_, err := openUserStarStore()
	if err == nil || !strings.HasPrefix(err.Error(), "cannot find where to save stars: ") {
		t.Errorf("openUserStarStore() error = %v; want a 'cannot find where to save stars' error", err)
	}
}
