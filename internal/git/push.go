package git

import (
	"context"
	"fmt"
	"strings"
)

// PushOptions is the vetted subset of `git push` flags gits passes through.
// The set is closed by design: it has no representation for `--force`,
// `--force-with-lease`, `-u`/`--set-upstream`, `--mirror` or `-q`/`--quiet`,
// so no caller can reach them. See docs/adr/0002-push-safety-model.md.
type PushOptions struct {
	All        bool // push all branches
	Branches   bool // git's synonym for --all
	Tags       bool // push all tags instead of the current branch
	FollowTags bool // include annotated tags reachable from the pushed refs
	Atomic     bool // one atomic transaction on the remote
	Prune      bool // remove remote refs matching the pushed refspec
	DryRun     bool // -n: report what would be pushed, push nothing
}

// Validate rejects flag combinations gits refuses to fan out. The three
// ref-selecting flags are mutually exclusive: git itself only rejects --tags
// against --all/--branches, treating --all and --branches as synonyms, but a
// command given both meant one of them and the ambiguity is not worth
// resolving across a whole project.
func (o PushOptions) Validate() error {
	if o.All && o.Branches || o.All && o.Tags || o.Branches && o.Tags {
		return fmt.Errorf("--all, --branches and --tags are mutually exclusive")
	}
	return nil
}

// SelectsRefs reports whether a flag chose which refs to push. When it does,
// the current branch's Upstream is irrelevant: the caller pushes without
// resolving one, and git resolves the destination.
func (o PushOptions) SelectsRefs() bool {
	return o.All || o.Branches || o.Tags
}

// args renders the options as git flags, in a stable order.
func (o PushOptions) args() []string {
	flags := []struct {
		on   bool
		flag string
	}{
		{o.All, "--all"},
		{o.Branches, "--branches"},
		{o.Tags, "--tags"},
		{o.FollowTags, "--follow-tags"},
		{o.Atomic, "--atomic"},
		{o.Prune, "--prune"},
		{o.DryRun, "--dry-run"},
	}
	args := []string{}
	for _, f := range flags {
		if f.on {
			args = append(args, f.flag)
		}
	}
	return args
}

// PushTarget names where a push goes: a remote and the refspec mapping a
// local branch onto its Upstream. The zero value means "let git resolve the
// destination", which is what the ref-selecting flags pass.
//
// Default-mode pushes always name both, rather than relying on a bare
// `git push`: that means whatever the user's push.default says, and under
// `matching` it would push every same-named branch in every repository.
type PushTarget struct {
	Remote  string
	Refspec string
}

// Push pushes from the repository at path. A target's remote is separated
// from the flags by --end-of-options, so a remote named like a flag is
// rejected as an unknown remote rather than parsed as one.
func (g *Git) Push(
	ctx context.Context,
	path string,
	target PushTarget,
	opts PushOptions,
) (string, error) {
	// Checked again here, not only at the CLI boundary: the primitive is what
	// a bad combination would be fanned out through.
	if err := opts.Validate(); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, g.networkTimeout())
	defer cancel()

	args := append([]string{"push"}, opts.args()...)
	if target.Remote != "" {
		args = append(args, "--end-of-options", target.Remote)
		if target.Refspec != "" {
			args = append(args, target.Refspec)
		}
	}

	output, err := g.ExecCombined(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("error during push: %w", err)
	}
	return cleanOutput(output), nil
}

// SplitUpstream splits an upstream ref such as "origin/main" into its remote
// and branch. Remote names cannot contain "/", so everything before the first
// separator is the remote.
//
// ok is false when the ref names no remote. A branch tracking another local
// branch (branch.<name>.remote = ".") abbreviates to a bare branch name: it
// has an Upstream, so it is not the skip condition, but there is nothing to
// push to either.
func SplitUpstream(upstream string) (remote, branch string, ok bool) {
	remote, branch, ok = strings.Cut(upstream, "/")
	if !ok || remote == "" || remote == "." || branch == "" {
		return "", "", false
	}
	return remote, branch, true
}
