package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// starStore keeps the skills starred in the web interface in a JSON file, so that they stay starred when the server
// restarts. The file maps each repository, e.g. "jetbrains/kotlin", to the paths of its starred skills.
type starStore struct {
	path  string
	mu    sync.Mutex
	stars map[string][]string // by starKey, sorted
}

// openUserStarStore opens the stars saved in skill-atlas/stars.json in the user's configuration directory.
func openUserStarStore() (*starStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("cannot find where to save stars: %w", err)
	}
	return openStarStore(filepath.Join(dir, "skill-atlas", "stars.json"))
}

// openStarStore opens the stars saved in the file at path. If there is no such file, no skill is starred yet.
func openStarStore(path string) (*starStore, error) {
	stars := map[string][]string{}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &starStore{path: path, stars: stars}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read stars: %w", err)
	}
	if err := json.Unmarshal(data, &stars); err != nil {
		return nil, fmt.Errorf("cannot read stars from %s: %w", path, err)
	}
	if stars == nil { // the file says null
		stars = map[string][]string{}
	}
	return &starStore{path: path, stars: stars}, nil
}

// starKey identifies repo in the file. GitHub ignores the case of repository owners and names, and so do stars.
func starKey(repo githubRepo) string {
	return strings.ToLower(repo.slug())
}

// starred returns the paths of repo's starred skills.
func (s *starStore) starred(repo githubRepo) map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	starred := map[string]bool{}
	for _, path := range s.stars[starKey(repo)] {
		starred[path] = true
	}
	return starred
}

// setStarred stars or unstars the skill at path in repo and saves the stars. If they cannot be saved, nothing changes.
func (s *starStore) setStarred(repo githubRepo, path string, starred bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := starKey(repo)
	paths := slices.DeleteFunc(slices.Clone(s.stars[key]), func(p string) bool { return p == path })
	if starred {
		paths = append(paths, path)
		slices.Sort(paths)
	}
	stars := maps.Clone(s.stars)
	if len(paths) > 0 {
		stars[key] = paths
	} else {
		delete(stars, key)
	}
	if err := writeStars(s.path, stars); err != nil {
		return fmt.Errorf("cannot save stars: %w", err)
	}
	s.stars = stars
	return nil
}

// writeStars writes stars to the file at path. The file is replaced at once, so it is never left half-written.
func writeStars(path string, stars map[string][]string) error {
	data, err := json.MarshalIndent(stars, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".stars-*.json")
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), path)
	}
	if err != nil {
		os.Remove(file.Name())
	}
	return err
}
