package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/domain"

	coreruntime "github.com/rafi/gits/internal/runtime"
	"github.com/rafi/gits/internal/runtime/projects"
)

func completionDeps() coreruntime.Runtime {
	// Shell completion discards any setting warnings: pressing Tab must stay
	// silent, and a bad duration has already fallen back to its default.
	deps, _ := newRuntime(context.Background())
	return deps
}

// completeValues offers a fixed set of flag values, filtered by what the user
// has typed. The set is always passed in from wherever the value is
// validated — output.Formats, list.Formats, config.ColorChoices — so a value
// can never be offered that the command would then reject.
func completeValues(values []string) func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		var completions []cobra.Completion
		for _, v := range values {
			if strings.HasPrefix(v, toComplete) {
				completions = append(completions, v)
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeProject returns a list of project names for shell completion.
func completeProject(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var completions []string
	for key, proj := range configFile.Projects {
		if toComplete == "" || strings.HasPrefix(key, toComplete) {
			completions = append(completions, fmt.Sprintf("%s\t%s", key, proj.Desc))
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeTags offers the tags declared in the config file, completing the
// last element of a comma-separated value.
func completeTags(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	prefix, last := splitLastTag(toComplete)

	tags := make([]string, 0, len(configFile.Projects))
	for _, proj := range configFile.Projects {
		tags = append(tags, collectTags(proj)...)
	}

	var completions []cobra.Completion
	for _, tag := range domain.MergeTags(tags) {
		if strings.HasPrefix(tag, domain.NormalizeTag(last)) {
			completions = append(completions, prefix+tag)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// collectTags returns every tag declared in the project subtree.
func collectTags(project domain.Project) []string {
	tags := append([]string{}, project.Tags...)
	for _, repo := range project.Repos {
		tags = append(tags, repo.Tags...)
	}
	for _, sub := range project.SubProjects {
		tags = append(tags, collectTags(sub)...)
	}
	return tags
}

// splitLastTag splits a partial `--tag` value into the completed prefix,
// including its trailing comma, and the tag being typed.
func splitLastTag(toComplete string) (prefix, last string) {
	idx := strings.LastIndex(toComplete, ",")
	if idx < 0 {
		return "", toComplete
	}
	return toComplete[:idx+1], toComplete[idx+1:]
}

// completeAddArgs completes `gits add`: the project name first, then
// directories for every repository argument after it, since those name
// clones on disk rather than repositories the project already knows.
func completeAddArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 1 {
		return completeProject(cmd, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveFilterDirs
}

// completeDirs offers directories for the first argument.
func completeDirs(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveFilterDirs
}

const (
	// The positional argument counts past which a completion function has
	// nothing left to offer: project, repo, branch.
	argsProjectRepo       = 2
	argsProjectRepoBranch = 3
)

// completeProjectRepo returns a list of repo names for shell completion.
func completeProjectRepo(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 1 {
		return completeProject(cmd, args, toComplete)
	}
	if len(args) >= argsProjectRepo {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	deps := completionDeps()

	// Load cache-only: pressing Tab must never trigger a provider network
	// fetch or a tokenCommand passphrase prompt. A remote project with a cold
	// cache simply offers no repository candidates.
	proj, err := projects.LoadOne(args[0], deps, projects.CacheOnly())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	repos := proj.ListReposWithNamespace()
	for _, repoName := range repos {
		if toComplete == "" || strings.HasPrefix(repoName, toComplete) {
			completions = append(completions, repoName)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeProjectRepoBranch returns a list of branch names for shell completion.
func completeProjectRepoBranch(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 1 {
		return completeProject(cmd, args, toComplete)
	}
	if len(args) < argsProjectRepo {
		return completeProjectRepo(cmd, args, toComplete)
	}
	if len(args) >= argsProjectRepoBranch {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	deps := completionDeps()

	// Get project (cache-only, so Tab never fetches or prompts — see above).
	proj, err := projects.LoadOne(args[0], deps, projects.CacheOnly())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Get repo
	repoName := args[1]
	repo, found := proj.GetRepo(repoName, "")
	if !found {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Find branches
	var completions []string
	branches, err := deps.Git.Branches(deps.Ctx, repo.AbsPath)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	for _, branch := range branches {
		if toComplete == "" || strings.HasPrefix(branch, toComplete) {
			completions = append(completions, branch)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
