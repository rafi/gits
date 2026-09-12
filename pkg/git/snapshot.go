package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Snapshot is the result of a single `git status --porcelain=v2 --branch`
// pass: branch identity, upstream tracking counts and worktree state that
// previously required four separate git invocations.
type Snapshot struct {
	// Branch is the current branch short name; "HEAD" when detached, for
	// parity with `rev-parse --abbrev-ref HEAD`.
	Branch string
	// HasUpstream reports whether ahead/behind counts are available — the
	// upstream is configured AND its ref exists (a gone upstream leaves it
	// false).
	HasUpstream bool
	Ahead       int
	Behind      int
	WorkTree
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
			case "branch.ab":
				snap.HasUpstream = true
				for f := range strings.FieldsSeq(value) {
					if len(f) < 2 {
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
			if len(rest) >= 2 {
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
