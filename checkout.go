package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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

// errEmptyRepository is reported for a repository without commits, which has no skills.
var errEmptyRepository = errors.New("repository is empty")

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
	switch {
	case errors.Is(err, errGitTimeout):
		return nil, fmt.Errorf("cannot access %s\n%w", strings.TrimSuffix(remote, ".git"), err)
	case err != nil:
		return nil, fmt.Errorf("cannot access %s (repository not found or private)\n%w", strings.TrimSuffix(remote, ".git"), err)
	}
	// The output is "ref: refs/heads/<branch>\tHEAD" if HEAD is a branch, and then "<commit>\tHEAD" unless the
	// repository is empty.
	branch, hasCommit := "", false
	for _, line := range strings.Split(head, "\n") {
		if ref, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
			branch, _, _ = strings.Cut(ref, "\t")
		} else if strings.HasSuffix(line, "\tHEAD") && !strings.HasPrefix(line, "ref: ") {
			hasCommit = true
		}
	}
	if !hasCommit {
		return nil, fmt.Errorf("cannot analyze %s (%w)", strings.TrimSuffix(remote, ".git"), errEmptyRepository)
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

// gitTimeout is how long a git command may run, so that a stalled connection to the remote, which git would wait for
// indefinitely, becomes an error. The largest trees, like that of JetBrains/intellij-community, are about 15 MB; this
// leaves time to fetch them over a slow connection shared by parallel scans. Tests shorten it.
var gitTimeout = 5 * time.Minute

// errGitTimeout is reported for a git command that did not finish within gitTimeout.
var errGitTimeout = errors.New("timed out")

func runGit(dir string, stdin io.Reader, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	// When git is stopped, a helper it started, like git-remote-https, can live on and keep its output open.
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		switch {
		case ctx.Err() != nil:
			return "", fmt.Errorf("git %s %w after %v", args[0], errGitTimeout, gitTimeout)
		case !errors.As(err, &exitErr):
			return "", fmt.Errorf("git is required but could not be started: %w", err)
		}
		return "", fmt.Errorf("git %s failed: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
