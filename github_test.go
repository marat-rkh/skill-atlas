package main

import "testing"

func TestParseGitHubRepo(t *testing.T) {
	tests := []struct {
		input string
		want  githubRepo
	}{
		{"https://github.com/JetBrains/kotlin", githubRepo{"JetBrains", "kotlin"}},
		{"https://github.com/JetBrains/kotlin.git", githubRepo{"JetBrains", "kotlin"}},
		{"https://github.com/JetBrains/kotlin/", githubRepo{"JetBrains", "kotlin"}},
		{"https://github.com/JetBrains/kotlin/tree/master/compiler", githubRepo{"JetBrains", "kotlin"}},
		{"https://github.com/owner/repo?tab=readme-ov-file", githubRepo{"owner", "repo"}},
		{"https://github.com/owner/repo#readme", githubRepo{"owner", "repo"}},
		{"http://www.github.com/owner/repo", githubRepo{"owner", "repo"}},
		{"github.com/owner/repo", githubRepo{"owner", "repo"}},
		{"HTTPS://GitHub.com/Owner/Repo", githubRepo{"Owner", "Repo"}},
		{"git@github.com:owner/repo.git", githubRepo{"owner", "repo"}},
		{"  https://github.com/owner/repo \n", githubRepo{"owner", "repo"}},
		{"https://github.com/my-org/my_repo.js", githubRepo{"my-org", "my_repo.js"}},
	}
	for _, tt := range tests {
		got, ok := parseGitHubRepo(tt.input)
		if !ok || got != tt.want {
			t.Errorf("parseGitHubRepo(%q) = %v, %v; want %v, true", tt.input, got, ok, tt.want)
		}
	}
}

func TestParseGitHubRepoRejectsOtherInput(t *testing.T) {
	for _, input := range []string{
		"",
		"not-a-url",
		"owner/repo",
		"https://gitlab.com/owner/repo",
		"https://evilgithub.com/owner/repo",
		"https://github.com.evil.com/owner/repo",
		"https://github.com/owner",
		"https://github.com/",
		"https://github.com/owner/repo name",
	} {
		if got, ok := parseGitHubRepo(input); ok {
			t.Errorf("parseGitHubRepo(%q) = %v, true; want rejection", input, got)
		}
	}
}

func TestGitHubRepoURLs(t *testing.T) {
	repo := githubRepo{"JetBrains", "kotlin"}
	if got := repo.slug(); got != "JetBrains/kotlin" {
		t.Errorf("slug() = %q", got)
	}
	if got := repo.webURL(); got != "https://github.com/JetBrains/kotlin" {
		t.Errorf("webURL() = %q", got)
	}
	if got := repo.cloneURL(); got != "https://github.com/JetBrains/kotlin.git" {
		t.Errorf("cloneURL() = %q", got)
	}
}
