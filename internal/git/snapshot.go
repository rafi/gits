package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	// abFieldLen is the shortest useful `branch.ab` field: a sign and a digit.
	abFieldLen = 2
	// xyFieldLen is the width of the XY status field an entry line opens with.
	xyFieldLen = 2
)

// Snapshot is the result of a single `git status --porcelain=v2 --branch`
// pass: branch identity, upstream tracking counts and worktree state that
// previously required four separate git invocations.
type Snapshot struct {
	WorkTree

	// Branch is the current branch short name; "HEAD" when detached, for
	// parity with `rev-parse --abbrev-ref HEAD`.
	Branch string
	// Upstream is the configured upstream's short name ("origin/main"), empty
	// when the branch has none. A branch tracking another local branch
	// abbreviates to a bare branch name, with no remote prefix.
	Upstream string
	// Tracking reports whether the upstream resolves — it is configured AND
	// its ref exists, which is also the condition for ahead/behind counts
	// being available.
	Tracking bool
	Ahead    int
	Behind   int
}

// GoneUpstream reports the Gone Upstream state: an upstream is configured but
// no ref resolves behind it, which is git's own `[gone]`. Stating the rule
// here keeps every caller off the field pair.
//
// This derives the state from the working-tree snapshot; HeadUpstream derives
// the same state from a ref walk, for callers that take no snapshot. A fix to
// one belongs in the other.
func (s Snapshot) GoneUpstream() bool {
	return s.Upstream != "" && !s.Tracking
}

// Snapshot reads branch, upstream tracking and worktree state in one git
// invocation.
func (g *Git) Snapshot(ctx context.Context, path string) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	out, err := g.Exec(ctx, path, []string{"status", "--porcelain=v2", "--branch"})
	if err != nil {
		return Snapshot{}, fmt.Errorf("unable to read repo snapshot: %w", err)
	}
	return parsePorcelainV2(string(out)), nil
}

// parsePorcelainV2 parses porcelain v2 output: `# branch.*` headers followed
// by entry lines (1 ordinary, 2 rename/copy, u unmerged, ? untracked,
// ! ignored). Conflict entries count as both staged and unstaged, matching
// the v1 parser.
func parsePorcelainV2(out string) Snapshot {
	var snap Snapshot
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" {
			continue
		}
		kind, rest, _ := strings.Cut(line, " ")
		switch kind {
		case "#":
			key, value, _ := strings.Cut(rest, " ")
			switch key {
			case "branch.head":
				snap.Branch = value
				if value == "(detached)" {
					snap.Branch = "HEAD"
				}
			case "branch.upstream":
				snap.Upstream = value
			case "branch.ab":
				// Git emits this header only when the upstream ref resolves;
				// a gone upstream emits branch.upstream without it.
				snap.Tracking = true
				for f := range strings.FieldsSeq(value) {
					if len(f) < abFieldLen {
						continue
					}
					n, err := strconv.Atoi(f[1:])
					if err != nil {
						continue
					}
					switch f[0] {
					case '+':
						snap.Ahead = n
					case '-':
						snap.Behind = n
					}
				}
			}
		case "1", "2":
			if len(rest) >= xyFieldLen {
				if rest[0] != '.' {
					snap.Staged++
				}
				if rest[1] != '.' {
					snap.Unstaged++
				}
			}
		case "u":
			snap.Staged++
			snap.Unstaged++
		case "?":
			snap.Untracked++
		}
	}
	return snap
}
