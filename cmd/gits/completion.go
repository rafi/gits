package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/loader"
	"github.com/rafi/gits/internal/types"
)

func completionDeps() types.Runtime {
	// Shell completion discards any setting warnings: pressing Tab must stay
	// silent, and a bad duration has already fallen back to its default.
	deps, _ := newRuntime(context.Background())
	return deps
}

// completeValues offers a fixed set of flag values, filtered by what the user
// has typed. The set is always passed in from wherever the value is
// validated — bulk.Formats, list.Formats, config.ColorChoices — so a value
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
	proj, err := loader.GetProject(args[0], deps, loader.CacheOnly())
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
	proj, err := loader.GetProject(args[0], deps, loader.CacheOnly())
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
