// Package pick is the interactive half of resolving what a command runs on:
// an fzf-backed [target.Selector], plus the branch picker `browse` uses.
package pick

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/infra/fzf"
	"github.com/rafi/gits/internal/runtime/command"
	"github.com/rafi/gits/internal/runtime/target"
)

// branchLineFields is how many tab-separated fields a branch selection line
// carries: the kind indicator and the ref name.
const branchLineFields = 2

// FZF asks the user, through the finder subprocess.
type FZF struct {
	deps app.RuntimeCLI
}

// NewFZF returns the interactive selector for a CLI command.
func NewFZF(deps app.RuntimeCLI) FZF { return FZF{deps: deps} }

var _ target.Selector = FZF{}

// isCancelled reports whether an interactive selection ended without a
// choice — the user pressed Esc/Ctrl-C or there was nothing to match.
func isCancelled(err error) bool {
	return errors.Is(err, fzf.ErrAborted) || errors.Is(err, fzf.ErrNoMatch)
}

// ParseArgs parses the arguments and returns the project and repo, prompting
// for whatever they left out.
func ParseArgs(args []string, skipRepoSelect bool, deps app.RuntimeCLI) (
	domain.Project, *domain.Repository, error,
) {
	return ParseArgsWithTags(args, domain.TagSet{}, skipRepoSelect, deps)
}

// ParseArgsWithTags is ParseArgs narrowed to repositories carrying any of
// tags.
func ParseArgsWithTags(
	args []string, tags domain.TagSet, skipRepoSelect bool, deps app.RuntimeCLI,
) (domain.Project, *domain.Repository, error) {
	return target.Resolve(
		target.Parse(args, !skipRepoSelect).WithTags(tags),
		NewFZF(deps), deps.Runtime)
}

// Project returns an interactively selected project name.
func (f FZF) Project(ctx context.Context) (string, error) {
	deps := f.deps

	// Collect project names in a stable order so the picker does not reshuffle
	// between runs.
	buffer := bytes.Buffer{}
	for _, name := range deps.Projects.SortedNames() {
		project := deps.Projects[name]
		project.Name = name
		buffer.WriteString(style.ProjectTitle(project, deps.Theme))
		buffer.WriteByte('\n')
	}

	// Run fzf with the sub-command 'list' as preview.
	finder := fzf.New(deps.Err, "--nth=1").WithFinder(deps.Settings.Finder)
	finder.WithPrompt("project> ")

	previewCmd := previewCommandf(deps.ConfigPath, "list", "{1}", "-o", "tree")
	finder.WithPreview(previewCmd, "")

	projName, err := finder.Run(ctx, buffer)
	if err != nil {
		if isCancelled(err) {
			return "", nil
		}
		return "", err
	}
	return strings.Split(projName, " ")[0], nil
}

// Repo returns an interactively selected repository name.
func (f FZF) Repo(
	ctx context.Context, project domain.Project, rootProject string,
) (string, error) {
	deps := f.deps

	repoStyle := deps.Theme.RepoTitle
	buffer := bytes.Buffer{}
	for _, repo := range project.ListReposWithNamespace() {
		buffer.WriteString(repoStyle.Render(repo))
		buffer.WriteByte('\n')
	}

	// rootProject is empty when a root project is provided.
	prefix := project.Name
	if rootProject == "" {
		prefix = ""
		rootProject = project.Name
	}

	// Run fzf with the hidden sub-command 'repo-overview' as preview.
	finder := fzf.New(deps.Err).WithFinder(deps.Settings.Finder)
	finder.WithPrompt(fmt.Sprintf("[%s] repo> ", project.Name))

	// {} carries the selected line and is appended to the quoted prefix with
	// no space: the two together are one argument.
	previewCmd := previewCommand(deps.ConfigPath, "repo-overview", rootProject, prefix) + "{}"
	finder.WithPreview(previewCmd, "")

	repoName, err := finder.Run(ctx, buffer)
	if err != nil {
		if isCancelled(err) {
			return "", nil
		}
		return "", fmt.Errorf("unable to select a repository: %w", err)
	}
	return repoName, nil
}

// SelectBranch returns an interactively selected branch name. It is not part
// of the Selector: only `browse` picks a branch.
func SelectBranch(
	projName string,
	repo domain.Repository,
	deps app.RuntimeCLI,
) (string, error) {
	refs, err := deps.Git.Refs(deps.Ctx, repo.AbsPath)
	if err != nil {
		return "", fmt.Errorf("unable to list branches and tags: %w", err)
	}

	delimiter := "\t"

	branchLabel := deps.Theme.BranchIndicator.Render("branch") + delimiter
	tagLabel := deps.Theme.TagIndicator.Render("tag") + delimiter

	buffer := bytes.Buffer{}
	for _, ref := range refs {
		ref = strings.Replace(ref, "refs/tags/", tagLabel, 1)
		ref = strings.Replace(ref, "refs/heads/", branchLabel, 1)
		buffer.WriteString(ref)
		buffer.WriteByte('\n')
	}

	repoFullName := repo.GetNameWithNamespace()

	// Run fzf with the hidden sub-command 'branch-overview' as preview.
	finder := fzf.New(deps.Err, "--delimiter="+delimiter, "--nth=2").
		WithFinder(deps.Settings.Finder)
	finder.WithPrompt(fmt.Sprintf("[%s/%s] branch> ", projName, repoFullName))

	previewCmd := previewCommandf(
		deps.ConfigPath, "branch-overview", "{2}", projName, repoFullName)
	finder.WithPreview(previewCmd, "")

	selected, err := finder.Run(deps.Ctx, buffer)
	if err != nil {
		if isCancelled(err) {
			return "", domain.NewWarning("no branch selected")
		}
		return "", err
	}
	parts := strings.SplitN(selected, delimiter, 3)
	if len(parts) < branchLineFields {
		return "", domain.NewWarning("no branch selected")
	}
	return parts[1], nil
}

// Target parses the arguments and returns the project, and the repository
// when one was named, as the engine's target — prompting for whatever they
// left out. It is the step every bulk command takes before running.
func Target(args []string, deps app.RuntimeCLI) (command.Target, error) {
	return TargetWithTags(args, domain.TagSet{}, deps)
}

// TargetWithTags is Target narrowed to repositories carrying any of tags.
func TargetWithTags(
	args []string, tags domain.TagSet, deps app.RuntimeCLI,
) (command.Target, error) {
	project, repo, err := ParseArgsWithTags(args, tags, true, deps)
	if err != nil {
		return command.Target{}, err
	}
	return command.Target{Project: project, Repo: repo}, nil
}
