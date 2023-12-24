package git

import (
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

type Repository struct {
	client *git.Repository
}

func (r *Repository) Branches() ([]string, error) {
	branches := []string{}
	refs, err := r.client.References()
	if err != nil {
		return nil, err
	}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.SymbolicReference {
			branches = append(branches, ref.Name().Short())
		}
		return nil
	})
	return branches, err
}

func (r *Repository) CurrentBranch() (string, error) {
	head, err := r.client.Head()
	if err != nil {
		return "", err
	}
	return head.Name().Short(), nil
}

func (r *Repository) IsLocalBranch(branch string) bool {
	_, err := r.client.Reference(
		plumbing.NewBranchReferenceName(branch), true)
	return err == nil
}

func (r *Repository) IsRemoteBranch(remote, branch string) bool {
	_, err := r.client.Reference(
		plumbing.NewRemoteReferenceName(remote, branch), true)
	return err == nil
}

func (r *Repository) Remotes() ([]string, error) {
	s := []string{}
	remotes, err := r.client.Remotes()
	if err != nil {
		return s, err
	}
	for _, remote := range remotes {
		s = append(s, remote.Config().Name)
	}
	return s, nil
}

func (r *Repository) Checkout(branch string) error {
	branchNoRemote, err := r.stripRemoteFromBranch(branch)
	if err != nil {
		return err
	}
	branchRef := plumbing.NewBranchReferenceName(branchNoRemote)

	opts := &git.CheckoutOptions{
		Create: false,
		Branch: branchRef,
	}

	if _, err := r.client.Reference(branchRef, false); err != nil {
		opts.Create = true
		h, err := r.client.ResolveRevision(plumbing.Revision(branch))
		if err != nil {
			return err
		}
		opts.Hash = *h
	}

	w, err := r.client.Worktree()
	if err != nil {
		return err
	}
	return w.Checkout(opts)
}

func (r *Repository) stripRemoteFromBranch(branch string) (string, error) {
	remotes, err := r.Remotes()
	if err != nil {
		return "", err
	}

	for _, remote := range remotes {
		if remote == "" {
			continue
		}
		branch = strings.TrimPrefix(branch, remote+"/")
	}
	return branch, nil
}
