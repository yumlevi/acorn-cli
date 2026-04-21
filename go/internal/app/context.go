package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// GatherContext produces the first-message context block acorn/context.py
// injects before the user's initial prompt. Not a verbatim port — just the
// useful fields (OS, Go/Node/Python versions if installed, git branch,
// relevant top-level files).
func GatherContext(cwd string) string {
	var parts []string
	parts = append(parts, "[Environment: "+envDescription()+"]")
	parts = append(parts, "[CWD: "+cwd+"]")
	if root := findGitRoot(cwd); root != "" {
		branch := gitBranch(cwd)
		if branch != "" {
			parts = append(parts, fmt.Sprintf("[Git: %s (branch %s)]", root, branch))
		} else {
			parts = append(parts, "[Git: "+root+"]")
		}
	}
	if files := topLevelClues(cwd); files != "" {
		parts = append(parts, "[Files: "+files+"]")
	}
	if tools := detectTools(); tools != "" {
		parts = append(parts, "[Tools: "+tools+"]")
	}
	return strings.Join(parts, "\n")
}

func envDescription() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func topLevelClues(cwd string) string {
	// Pick up a handful of "what kind of project is this" markers.
	clues := []string{
		"package.json", "go.mod", "Cargo.toml", "pyproject.toml", "requirements.txt",
		"pom.xml", "build.gradle", "Gemfile", "composer.json",
		"Makefile", "README.md", ".git", "tsconfig.json", "vite.config.ts",
	}
	var found []string
	for _, c := range clues {
		if _, err := os.Stat(filepath.Join(cwd, c)); err == nil {
			found = append(found, c)
		}
	}
	return strings.Join(found, ", ")
}

func detectTools() string {
	tools := []string{"node", "python3", "go", "rustc", "cargo", "bun", "deno", "docker", "git"}
	var present []string
	for _, t := range tools {
		if _, err := exec.LookPath(t); err == nil {
			present = append(present, t)
		}
	}
	return strings.Join(present, ", ")
}
