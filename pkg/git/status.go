package git

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// GitModified returns the number of modified files
func (g Git) Modified(path string) (int, error) {
	args := []string{"diff", "--shortstat"}
	output, err := g.Exec(path, args)
	if err != nil {
		return 0, fmt.Errorf("unable to find modified diff: %w", err)
	}
	pat := regexp.MustCompile(`^\s*(\d+)`)
	matches := pat.FindAllStringSubmatch(string(output), -1)
	if len(matches) > 0 {
		modified, err := strconv.Atoi(matches[0][1])
		if err != nil {
			return 0, fmt.Errorf("unable to convert string to int: %w", err)
		}
		return modified, nil
	}
	return 0, nil
}

// Untracked returns the number of untracked files
func (g Git) Untracked(path string) (int, error) {
	args := []string{"ls-files", "--others", "--exclude-standard"}
	output, err := g.Exec(path, args)
	if err != nil {
		return 0, fmt.Errorf("unable to find untracked: %w", err)
	}
	return len(strings.Split(string(output), "\n")) - 1, nil
}

// CurrentPosition returns a short log description of HEAD
func (g Git) CurrentPosition(path string) (string, error) {
	args := []string{"log", "-1", "--color=always", "--format=%C(auto)%D %C(242)(%aN %ar)%Creset"}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("unable to get current rev: %w", err)
	}
	return cleanOutput(output), nil
}

// Describe generates a version description based on tags and hash
func (g Git) Describe(path string) (string, error) {
	args := []string{"describe", "--tags", "--always"}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("unable to describe rev: %w", err)
	}
	return cleanOutput(output), nil
}

// Diff returns a formatted string of ahead/behind counts
func (g Git) Diff(path string) (string, error) {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	abbrRef, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("unable to find ref: %w", err)
	}
	branch := cleanOutput(abbrRef)

	args = []string{"rev-parse", "--abbrev-ref", "@{upstream}"}
	abbrRefUpstream, _ := g.Exec(path, args)
	upstream := cleanOutput(abbrRefUpstream)
	if upstream == "" {
		upstream = fmt.Sprintf("origin/%v", branch)
	}

	args = []string{"rev-list", "--left-right", branch + "..." + upstream}
	output, _ := g.Exec(path, args)

	result := ""
	if len(output) == 0 {
		result = "✓"
		return result, nil
	}

	behind := 0
	ahead := 0
	for _, rev := range strings.Split(string(output), "\n") {
		if rev == "" {
			continue
		}
		rev = string(rev[0])
		if rev == ">" {
			behind++
		}
		if rev == "<" {
			ahead++
		}
	}

	if ahead > 0 {
		result = fmt.Sprintf("▲%d", ahead)
	}
	if behind > 0 {
		result = fmt.Sprintf("%v▼%d", result, behind)
	}

	return result, nil
}
