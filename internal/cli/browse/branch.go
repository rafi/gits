package browse

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/loader"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/fzf"
	"github.com/rafi/gits/pkg/git"
)

const (
	maxBars = 10
	daysAgo = 7
)

var (
	commonReleaseBranches  = []string{"master", "main", "dev", "next"}
	smallNumericCharacters = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
)

// ExecBrowseOverview displays a branch overview.
// Args:
//   - project name
//   - repo name
//   - branch name (optional)
func ExecBranchOverview(args []string, deps types.RuntimeCLI) error {
	// Project
	if len(args) < 1 {
		return fmt.Errorf("missing project name")
	}
	project, err := loader.GetProject(args[0], deps.Runtime)
	if err != nil {
		return fmt.Errorf("unable to load project %q: %w", args[0], err)
	}

	// Repository
	if len(args) < 2 {
		return fmt.Errorf("missing repo name")
	}
	repoName := args[1]
	foundRepo, found := project.GetRepo(repoName, "")
	if !found {
		return fmt.Errorf("repo %s/%s not found", args[0], repoName)
	}

	repo, err := deps.Git.Open(foundRepo.AbsPath)
	if err != nil {
		return fmt.Errorf("unable to open repo: %w", err)
	}

	// Branch
	current := ""
	if len(args) > 2 {
		current = args[2]
	} else {
		current, err = repo.CurrentBranch()
		if err != nil {
			return fmt.Errorf("unable to get current branch: %w", err)
		}
	}

	// Remote
	remotes, err := repo.Remotes()
	if err != nil {
		return fmt.Errorf("unable to get remotes: %w", err)
	}

	// Fzf sets environment variables to detect width/height, see man fzf.
	width, _, err := fzf.GetPreviewSize()
	if err != nil {
		log.Warnf("unable to parse FZF_PREVIEW_COLUMNS: %s", err)
	}

	if width == 0 {
		// TODO: improve
		width = 80
	}

	theme := deps.Theme

	chartWidth := 0
	chartSidePadding := 2

	branchCurrentStyle := theme.BranchCurrent.Copy().
		Align(lipgloss.Left)

	panelLeftStyle := theme.Normal.Copy().
		// Border(lipgloss.NormalBorder()).
		Align(lipgloss.Left)

	chartStyle := theme.ChartDates.Copy().
		// Border(lipgloss.NormalBorder()).
		Padding(0, chartSidePadding).
		Align(lipgloss.Left)

	if width > 0 {
		panelWidth := width / 2

		branchCurrentStyle = branchCurrentStyle.
			Width(panelWidth).
			PaddingLeft(10)

		panelLeftStyle = panelLeftStyle.
			Width(panelWidth).
			PaddingLeft(0)

		chartStyle = chartStyle.Width(panelWidth)
		chartWidth = panelWidth - 2*chartSidePadding
	}

	panelLeft, err := renderBranchDiffList(repo, foundRepo.AbsPath, current, remotes, deps)
	if err != nil {
		log.Warnf("unable to render branch diff list: %s", err)
	}
	panelLeft = "\n" + branchCurrentStyle.Render(current) + "\n\n" + panelLeft

	// Render commits per day panelRight.
	panelRight, err := renderBranchChart(deps.Git, foundRepo, current, chartWidth)
	if err != nil {
		log.Warnf("unable to render chart: %s", err)
	}

	// Document
	doc := strings.Builder{}

	// Header
	headerStyle := theme.PreviewHeader.Copy()
	if width > 0 {
		headerStyle = headerStyle.Align(lipgloss.Center).Width(width - 2)
	}
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
	fmt.Println(docStyle.Render(doc.String()))

	log, err := deps.Git.Log(foundRepo.AbsPath, current)
	if err != nil {
		return err
	}
	fmt.Println(log)
	return nil
}

func renderBranchDiffList(repo git.Repository, repoPath, subjectBranch string, remotes []string, deps types.RuntimeCLI) (string, error) {
	doc := strings.Builder{}
	branches := map[string]string{}
	for _, remote := range remotes {
		b := append([]string{subjectBranch}, commonReleaseBranches...)
		for _, branchName := range b {
			target := fmt.Sprintf("%s/%s", remote, branchName)
			if repo.IsRemoteBranch(remote, branchName) {
				branches[target] = remote
				continue
			}
		}
	}
	theme := deps.Theme

	for fullName, remoteName := range branches {
		ahead, behind, err := deps.Git.Diff(repoPath, subjectBranch, fullName)
		if err != nil {
			return "", err
		}

		state := ""
		if ahead == 0 && behind == 0 {
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

		doc.WriteString(
			fmt.Sprintf("%s %s/%s\n",
				theme.Diff.Copy().Width(20).Align(lipgloss.Right).Render(state),
				theme.RemoteName.Render(remoteName),
				theme.BranchName.Render(branchName),
			))
	}
	return doc.String(), nil
}

// renderBranchChart draws a chart of commits per day.
func renderBranchChart(gitClient git.Git, repo domain.Repository, branch string, width int) (string, error) {
	commits, err := gitClient.CommitDates(repo.AbsPath, branch, daysAgo)
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

	d := time.Now()
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
		chart.WriteString(dateStr + " ")
		chart.WriteString(strings.Repeat("▇", bars))
	}
	return chart.String(), nil
}

// renderDigits returns a string of small numeric characters.
func renderDigits(num int) string {
	digits := ""
	for _, rune := range strconv.Itoa(num) {
		char := fmt.Sprintf("%c", rune)
		i, err := strconv.Atoi(char)
		if err != nil {
			log.Warnf("unable to convert %q to int: %s", char, err)
			break
		}
		digits += smallNumericCharacters[i]
	}
	return digits
}
