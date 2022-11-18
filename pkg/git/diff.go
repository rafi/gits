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
		return 0, fmt.Errorf("Unable to find modified diff: %w", err)
	}
	pat := regexp.MustCompile(`^\s*(\d+)`)
	matches := pat.FindAllStringSubmatch(string(output), -1)
	if len(matches) > 0 {
		modified, err := strconv.Atoi(matches[0][1])
		if err != nil {
			return 0, fmt.Errorf("Unable to convert string to int: %w", err)
		}
		return modified, nil
	}
	return 0, nil
}

// Diff returns a formatted string of ahead/behind counts
func (g Git) Diff(path string) (string, error) {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	abbrRef, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("Unable to find ref: %w", err)
	}
	branch := strings.TrimSuffix(string(abbrRef), "\n")

	args = []string{"rev-parse", "--abbrev-ref", "@{upstream}"}
	abbrRefUpstream, _ := g.Exec(path, args)
	upstream := strings.TrimSuffix(string(abbrRefUpstream), "\n")
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
