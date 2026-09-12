package git

import (
	"context"
	"fmt"
	"strings"
)

const (
	// upstreamSep separates the ref-walk fields in git's output. NUL rather
	// than the more readable "|", because a branch name may legally contain a
	// pipe and nothing else would keep the field count honest. The format
	// spells it "%00": a literal NUL cannot travel in an exec argument.
	upstreamSep    = "\x00"
	upstreamSepFmt = "%00"
	// headUpstreamFormat asks for, per local branch: the HEAD marker ("*" for
	// the checked-out branch), the branch name, its upstream's short name
	// (empty when none is configured, and without a remote prefix when the
	// upstream is another local branch), and the tracking summary.
	headUpstreamFormat = "%(HEAD)" + upstreamSepFmt +
		"%(refname:short)" + upstreamSepFmt +
		"%(upstream:short)" + upstreamSepFmt +
		"%(upstream:track)"
	// goneTrack is what %(upstream:track) renders for an upstream whose ref
	// no longer exists — git's own word for the state.
	goneTrack = "[gone]"
	// detachedBranch is reported when no branch is checked out, for parity
	// with CurrentBranch, which renders a detached HEAD this way.
	detachedBranch = "HEAD"
)

// HeadRef is the current branch together with the state of its Upstream, the
// four cases being: an upstream that resolves, a Gone Upstream, no upstream at
// all (Upstream empty), and an upstream on no remote (a bare Upstream name,
// which SplitUpstream rejects).
type HeadRef struct {
	// Branch is the current branch short name; "HEAD" when detached, for
	// parity with `rev-parse --abbrev-ref HEAD`.
	Branch string
	// Upstream is the configured upstream's short name, empty when none is.
	Upstream string
	// Gone reports the Gone Upstream state: configured, with no ref behind it.
	Gone bool
}

// HeadUpstream reads the current branch and its Upstream in one call, taking
// git's own determination of whether that Upstream resolves — a fact no
// inference over git's human-readable error text can state.
//
// It walks refs and never touches the working tree, so its cost is
// independent of repository size. Snapshot.GoneUpstream derives the same
// state from the working-tree snapshot, for callers that take one anyway; a
// fix to one belongs in the other.
func (g *Git) HeadUpstream(ctx context.Context, path string) (HeadRef, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"for-each-ref", "--format=" + headUpstreamFormat, "refs/heads"}
	out, err := g.Exec(ctx, path, args)
	if err != nil {
		return HeadRef{}, fmt.Errorf("unable to read upstream branch: %w", err)
	}
	return parseHeadUpstream(cleanOutput(out)), nil
}

// parseHeadUpstream picks the row headUpstreamFormat marked as HEAD. A
// detached or unborn HEAD marks no row at all, and reports the branch the
// same way CurrentBranch does.
func parseHeadUpstream(out string) HeadRef {
	for _, line := range splitLines(out) {
		fields := strings.Split(line, upstreamSep)
		if len(fields) != 4 || fields[0] != "*" {
			continue
		}
		return HeadRef{
			Branch:   fields[1],
			Upstream: fields[2],
			Gone:     fields[3] == goneTrack,
		}
	}
	return HeadRef{Branch: detachedBranch}
}
