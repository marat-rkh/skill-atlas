package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

// runCLI runs the tool with the given arguments and returns the process exit code.
func runCLI(args []string, stdout, stderr io.Writer) int {
	repo, ok := githubRepo{}, false
	if len(args) == 1 {
		repo, ok = parseGitHubRepo(args[0])
	}
	if !ok {
		fmt.Fprintln(stderr, "Usage: skill-atlas <github-repository-url>")
		return 2
	}

	fmt.Fprintf(stderr, "Analyzing %s ...\n", repo.webURL())
	m, err := scanRepository(repo)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, renderMap(m))
	return 0
}
