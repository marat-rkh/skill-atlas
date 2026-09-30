package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// repoCheckout is a temporary shallow checkout of a repository that contains only its SKILL.md files.
type repoCheckout struct {
	dir        string
	branch     string // empty if the remote HEAD is not a branch
	commit     string
	skillFiles []string
}

func (c *repoCheckout) read(path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(c.dir, filepath.FromSlash(path)))
	return string(data), err
}

func (c *repoCheckout) close() {
	os.RemoveAll(c.dir)
}

// checkoutSkillFiles fetches the latest commit of the remote's default branch without file contents, finds every
// SKILL.md in its tree, and then downloads just those files. This keeps huge repositories cheap to analyze.
func checkoutSkillFiles(remote string) (_ *repoCheckout, err error) {
	dir, err := os.MkdirTemp("", "skill-atlas-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()
	git := func(args ...string) (string, error) { return runGit(dir, nil, args...) }

	if _, err = git("init", "-q"); err != nil {
		return nil, err
	}
	if _, err = git("remote", "add", "origin", remote); err != nil {
		return nil, err
	}
	head, err := git("ls-remote", "--symref", "origin", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("cannot access %s (repository not found or private)\n%w", strings.TrimSuffix(remote, ".git"), err)
	}
	branch := ""
	for _, line := range strings.Split(head, "\n") {
		if ref, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
			branch, _, _ = strings.Cut(ref, "\t")
			break
		}
	}

	if _, err = git("fetch", "-q", "--depth", "1", "--filter=blob:none", "origin", "HEAD"); err != nil {
		return nil, err
	}
	commit, err := git("rev-parse", "FETCH_HEAD")
	if err != nil {
		return nil, err
	}

	tree, err := git("ls-tree", "-r", "-z", "FETCH_HEAD")
	if err != nil {
		return nil, err
	}
	var skillFiles []string
	// Entries look like "<mode> <type> <object>\t<path>"; mode 100xxx is a regular file.
	for _, entry := range strings.Split(tree, "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		if ok && strings.HasPrefix(meta, "100") && lastSegment(path) == "SKILL.md" {
			skillFiles = append(skillFiles, path)
		}
	}

	if len(skillFiles) > 0 {
		// Checking out through sparse patterns downloads all the needed blobs in a single batch.
		patterns := make([]string, len(skillFiles))
		for i, file := range skillFiles {
			patterns[i] = sparsePattern(file)
		}
		if _, err = runGit(dir, strings.NewReader(strings.Join(patterns, "\n")), "sparse-checkout", "set", "--no-cone", "--stdin"); err != nil {
			return nil, err
		}
		if _, err = git("checkout", "-q", "FETCH_HEAD"); err != nil {
			return nil, err
		}
	}
	return &repoCheckout{dir: dir, branch: branch, commit: strings.TrimSpace(commit), skillFiles: skillFiles}, nil
}

var sparseSpecialChars = regexp.MustCompile(`[\\*?\[]`)

func sparsePattern(path string) string {
	return "/" + sparseSpecialChars.ReplaceAllString(path, `\${0}`)
}

func runGit(dir string, stdin io.Reader, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return "", fmt.Errorf("git is required but could not be started: %w", err)
		}
		return "", fmt.Errorf("git %s failed: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
