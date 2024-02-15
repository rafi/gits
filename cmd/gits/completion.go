package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/cache"
	"github.com/rafi/gits/internal/loader"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

func completionDeps() (deps types.Runtime, err error) {
	cacheClient, err := cache.NewCacheClient("file")
	if err != nil {
		return deps, err
	}
	return types.Runtime{
		Cache:    cacheClient,
		Projects: configFile.Projects,
	}, nil
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

// completeProjectRepo returns a list of repo names for shell completion.
func completeProjectRepo(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 1 {
		return completeProject(cmd, args, toComplete)
	}
	// Max 2 args: project, repo.
	if len(args) > 3 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	deps, err := completionDeps()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	proj, err := loader.GetProject(args[0], deps)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
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
	if len(args) < 2 {
		return completeProjectRepo(cmd, args, toComplete)
	}
	// Max 3 args: project, repo, branch.
	if len(args) > 3 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	deps, err := completionDeps()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Get project
	proj, err := loader.GetProject(args[0], deps)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Get repo
	repoName := args[1]
	repo, found := proj.GetRepo(repoName, "")
	if repo.Name == "" || !found {
		return nil, cobra.ShellCompDirectiveError
	}

	// Find branches
	var completions []string
	g, err := git.NewGit()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	gitRepo, err := g.Open(repo.AbsPath)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	branches, err := gitRepo.Branches()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	for _, branch := range branches {
		if toComplete == "" || strings.HasPrefix(branch, toComplete) {
			completions = append(completions, branch)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
