package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

const usage = `Usage:
  skill-atlas <github-repository-url>           print the map of the repository's agent skills
  skill-atlas <github-organization-url>         print the map of the agent skills in the organization's repositories
  skill-atlas --group <github-url>              print the map with similar skills grouped
  skill-atlas serve                             start the web interface at http://` + serveAddress

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

// runCLI runs the tool with the given arguments and returns the process exit code.
func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "serve" {
		listener, err := net.Listen("tcp", serveAddress)
		if err == nil {
			err = serve(listener, stdout)
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	grouping := len(args) == 2 && args[0] == "--group"
	if grouping {
		args = args[1:]
	}
	if len(args) != 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if org, ok := parseGitHubOrg(args[0]); ok {
		fmt.Fprintf(stderr, "Analyzing %s ...\n", org.webURL())
		m, err := scanOrganization(org)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		return printOrgMap(m, grouping, stdout, stderr)
	}
	repo, ok := parseGitHubRepo(args[0])
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

// printOrgMap prints the organization's map to stdout, and then an error to stderr for each repository that could not
// be analyzed. It returns the exit code: 1 if some repositories could not be analyzed, 0 otherwise.
func printOrgMap(m orgMap, grouping bool, stdout, stderr io.Writer) int {
	fmt.Fprint(stdout, renderOrgMap(m, grouping))
	for _, failure := range m.failures {
		fmt.Fprintf(stderr, "error: %v\n", failure)
	}
	if len(m.failures) > 0 {
		return 1
	}
	return 0
}
