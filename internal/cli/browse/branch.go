// Package browse implements `gits browse`, an interactive view over a
// Repository's branches and tags.
package browse

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

const (
	maxBars = 10
	daysAgo = 7
)

var (
	commonReleaseBranches  = []string{"master", "main", "dev", "next"}
	smallNumericCharacters = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}

	// now is a seam so the chart's day axis is deterministic in tests.
	now = time.Now
)

// ExecBranchOverview displays a branch overview.
// Args:
//   - project name
//   - repo name
//   - branch name (optional)
func ExecBranchOverview(args []string, deps types.RuntimeCLI) error {
	repo, err := resolveProjectRepo(args, deps)
	if err != nil {
		return err
	}
	branch := ""
	if len(args) > branchArgIdx {
		branch = args[branchArgIdx]
	}
	return renderBranchOverview(repo, args[1], branch, deps)
}

// renderBranchOverview renders the branch overview for an already-resolved
// repository. An empty branch means the currently checked-out one.
func renderBranchOverview(
	repo domain.Repository, repoName, branch string, deps types.RuntimeCLI,
) error {
	current := branch
	if current == "" {
		var err error
		current, err = deps.Git.CurrentBranch(deps.Ctx, repo.AbsPath)
		if err != nil {
			return fmt.Errorf("unable to get current branch: %w", err)
		}
	}

	// Remote
	remotes, err := deps.Git.Remotes(deps.Ctx, repo.AbsPath)
	if err != nil {
		return fmt.Errorf("unable to get remotes: %w", err)
	}

	width := previewWidth()
	if width == 0 {
		width = 80
	}

	theme := deps.Theme

	panelWidth := width / panelCount

	branchCurrentStyle := theme.BranchCurrent.
		Align(lipgloss.Left).
		Width(panelWidth).
		PaddingLeft(branchNameIndent)

	panelLeftStyle := theme.Normal.
		// Border(lipgloss.NormalBorder()).
		Align(lipgloss.Left).
		Width(panelWidth).
		PaddingLeft(0)

	chartStyle := theme.ChartDates.
		// Border(lipgloss.NormalBorder()).
		Padding(0, chartSidePadding).
		Align(lipgloss.Left).
		Width(panelWidth)

	chartWidth := panelWidth - chartSidePadding - chartSidePadding

	panelLeft := renderBranchDiffList(repo.AbsPath, current, remotes, deps)
	panelLeft = "\n" + branchCurrentStyle.Render(current) + "\n\n" + panelLeft

	// Render commits per day panelRight.
	panelRight, err := renderBranchChart(deps.Ctx, deps.Git, repo, current, chartWidth)
	if err != nil {
		log.Warnf("unable to render chart: %s", err)
	}

	// Document
	doc := strings.Builder{}

	// Header
	headerStyle := previewHeader(theme.PreviewHeader, width)
	doc.WriteString(headerStyle.Render(repoName))
	doc.WriteString("\n")

	// Layout
	doc.WriteString(lipgloss.JoinHorizontal(
		lipgloss.Top,
		panelLeftStyle.Render(panelLeft),
		chartStyle.Render(panelRight),
	))
	doc.WriteString("\n")
	doc.WriteString("Latest commits:")

	docStyle := lipgloss.NewStyle().Padding(0)
	lipgloss.Fprintln(deps.Out, docStyle.Render(doc.String()))

	commitLog, err := deps.Git.Log(deps.Ctx, repo.AbsPath, current)
	if err != nil {
		return err
	}
	fmt.Fprintln(deps.Out, commitLog)
	return nil
}

func renderBranchDiffList(repoPath, subjectBranch string, remotes []string, deps types.RuntimeCLI) string {
	doc := strings.Builder{}
	refs, err := deps.Git.RemoteBranches(deps.Ctx, repoPath)
	if err != nil {
		log.Warnf("unable to list remote branches: %s", err)
	}
	existing := make(map[string]bool, len(refs))
	for _, ref := range refs {
		existing[ref] = true
	}
	branches := map[string]string{}
	for _, remote := range remotes {
		b := append([]string{subjectBranch}, commonReleaseBranches...)
		for _, branchName := range b {
			target := fmt.Sprintf("%s/%s", remote, branchName)
			if existing[target] {
				branches[target] = remote
			}
		}
	}
	theme := deps.Theme

	// Iterate in sorted order so the list doesn't reshuffle on every
	// preview redraw.
	names := make([]string, 0, len(branches))
	for fullName := range branches {
		names = append(names, fullName)
	}
	sort.Strings(names)

	for _, fullName := range names {
		remoteName := branches[fullName]
		ahead, behind, err := deps.Git.Diff(deps.Ctx, repoPath, subjectBranch, fullName)

		state := ""
		if err != nil {
			// Best-effort: one failed comparison marks its own row N/A
			// instead of blanking the whole panel.
			state = deps.Settings.Icons.NA
		} else if ahead == 0 && behind == 0 {
			state = "✓"
		}
		if ahead > 0 {
			state = fmt.Sprintf("▲%d", ahead)
		}
		if behind > 0 {
			if len(state) > 0 {
				state += " "
			}
			state = fmt.Sprintf("%s▼%d", state, behind)
		}
		branchName := strings.TrimPrefix(fullName, remoteName+"/")

		fmt.Fprintf(&doc, "%s %s/%s\n",
			theme.Diff.Width(branchStateWidth).Align(lipgloss.Right).Render(state),
			theme.RemoteName.Render(remoteName),
			theme.BranchName.Render(branchName),
		)
	}
	return doc.String()
}

// renderBranchChart draws a chart of commits per day.
func renderBranchChart(ctx context.Context, gitClient git.Client, repo domain.Repository, branch string, width int) (string, error) {
	commits, err := gitClient.CommitDates(ctx, repo.AbsPath, branch, daysAgo)
	if err != nil {
		return "", fmt.Errorf("unable to get commit dates: %w", err)
	}
	days := make(map[string]int)
	highest := 0
	for _, c := range commits {
		if c == "" {
			continue
		}
		days[c]++
		if days[c] > highest {
			highest = days[c]
		}
	}

	if highest == 0 {
		emptyState := fmt.Sprintf("\n<no commits in the last %d days>", daysAgo)
		return emptyState, nil
	}

	dateLength := 11
	barMaxSize := maxBars
	if width > 0 {
		barMaxSize = width - dateLength
	}

	chart := strings.Builder{}
	if highest > 1 {
		digits := renderDigits(highest)
		chart.WriteString(
			lipgloss.NewStyle().
				Align(lipgloss.Right).
				Width((dateLength + barMaxSize) - 1).
				Padding(0).
				MarginLeft(1).
				Render(digits),
		)
	}

	d := now()
	for i := 0; i <= daysAgo; i++ {
		year, month, day := d.Date()
		d = time.Date(year, month, day-1, 0, 0, 0, 0, time.UTC)
		year, month, day = d.Date()

		bars := 0
		dateStr := fmt.Sprintf("%d-%02d-%02d", year, month, day)
		if count, ok := days[dateStr]; ok {
			bars = int(float64(count) / float64(highest) * float64(barMaxSize))
		}
		chart.WriteString("\n")
		chart.WriteString(dateStr)
		chart.WriteByte(' ')
		chart.WriteString(strings.Repeat("▇", bars))
	}
	return chart.String(), nil
}

// renderDigits returns a string of small numeric characters.
func renderDigits(num int) string {
	var digits strings.Builder
	for _, r := range strconv.Itoa(num) {
		digits.WriteString(smallNumericCharacters[r-'0'])
	}
	return digits.String()
}
