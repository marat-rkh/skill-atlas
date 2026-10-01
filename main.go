package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

const usage = `Usage:
  skill-atlas <github-repository-url>           print the map of the repository's agent skills
  skill-atlas --group <github-repository-url>   print the map with similar skills grouped
  skill-atlas serve                             start the web interface at http://` + serveAddress

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

// runCLI runs the tool with the given arguments and returns the process exit code.
func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "serve" {
		fmt.Fprintf(stderr, "error: %v\n", runServer(stdout))
		return 1
	}

	grouping := len(args) == 2 && args[0] == "--group"
	if grouping {
		args = args[1:]
	}
	repo, ok := githubRepo{}, false
	if len(args) == 1 {
		repo, ok = parseGitHubRepo(args[0])
	}
	if !ok {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	fmt.Fprintf(stderr, "Analyzing %s ...\n", repo.webURL())
	m, err := scanRepository(repo)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, renderMap(m, grouping))
	return 0
}

// runServer serves the web interface at serveAddress, with the stars saved in the user's configuration directory, until
// it fails.
func runServer(stdout io.Writer) error {
	listener, err := net.Listen("tcp", serveAddress)
	if err != nil {
		return err
	}
	defer listener.Close()
	stars, err := openUserStarStore()
	if err != nil {
		return err
	}
	return serve(listener, stdout, stars)
}
