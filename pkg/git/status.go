package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WorkTree summarizes work-tree state from one `git status --porcelain` pass.
type WorkTree struct {
	Staged    int
	Unstaged  int
	Untracked int
}

// DiffStat totals uncommitted line changes against HEAD.
type DiffStat struct {
	Added   int
	Deleted int
}

// Head describes the current HEAD commit.
type Head struct {
	Hash    string
	Subject string
	Time    time.Time
}

func (g *Git) CurrentBranch(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	abbrRef, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("unable to find ref: %w", err)
	}
	return cleanOutput(abbrRef), nil
}

func (g *Git) UpstreamBranch(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"rev-parse", "--abbrev-ref", "@{upstream}"}
	out, err := g.Exec(ctx, path, args)
	if err != nil {
		// Only a genuine missing upstream maps to the sentinel — git names
		// the condition on stderr (folded into err by Exec). Anything else
		// (cancellation, corrupt repo) must keep its own identity so callers
		// don't mislabel real failures as "no upstream".
		msg := err.Error()
		if strings.Contains(msg, "no upstream configured") ||
			strings.Contains(msg, "does not point to a branch") {
			return "", ErrNoUpstream
		}
		return "", fmt.Errorf("unable to read upstream branch: %w", err)
	}
	return cleanOutput(out), nil
}

// Modified returns the number of files changed in the work tree or the
// index. `git status --porcelain` sees staged-but-uncommitted changes,
// which `git diff` alone would miss; untracked (??) entries are excluded.
func (g *Git) Modified(ctx context.Context, path string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"status", "--porcelain"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return 0, fmt.Errorf("unable to find modified files: %w", err)
	}
	modified := 0
	for line := range strings.SplitSeq(cleanOutput(output), "\n") {
		if line == "" || strings.HasPrefix(line, "??") {
			continue
		}
		modified++
	}
	return modified, nil
}

// Untracked returns the number of untracked files
func (g *Git) Untracked(ctx context.Context, path string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"ls-files", "--others", "--exclude-standard"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return 0, fmt.Errorf("unable to find untracked: %w", err)
	}
	return len(splitLines(cleanOutput(output))), nil
}

// WorkingState counts staged, unstaged and untracked paths in a single
// `git status --porcelain` pass, so status rows need one probe instead of the
// former Modified+Untracked pair.
func (g *Git) WorkingState(ctx context.Context, path string) (WorkTree, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	output, err := g.Exec(ctx, path, []string{"status", "--porcelain"})
	if err != nil {
		return WorkTree{}, fmt.Errorf("unable to read work-tree state: %w", err)
	}
	// No cleanOutput here: its TrimSpace would eat the leading space of the
	// first XY entry (e.g. " M file"), turning an unstaged file into a staged
	// one. parsePorcelain skips blank lines itself.
	return parsePorcelain(string(output)), nil
}

// parsePorcelain tallies porcelain v1 XY lines. Conflict entries count as both
// staged and unstaged; ignored (!!) entries are skipped.
func parsePorcelain(out string) WorkTree {
	var wt WorkTree
	for line := range strings.SplitSeq(out, "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		switch {
		case x == '?' && y == '?':
			wt.Untracked++
		case x == '!' && y == '!':
			// ignored entry
		default:
			if x != ' ' && x != '?' {
				wt.Staged++
			}
			if y != ' ' && y != '!' {
				wt.Unstaged++
			}
		}
	}
	return wt
}

// WorkingDiff sums uncommitted line changes — staged and unstaged, like
// `git diff HEAD --shortstat`. Untracked files contribute nothing, matching
// diff semantics.
func (g *Git) WorkingDiff(ctx context.Context, path string) (DiffStat, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	output, err := g.Exec(ctx, path, []string{"diff", "--shortstat", "HEAD"})
	if err != nil {
		return DiffStat{}, fmt.Errorf("unable to diff work tree: %w", err)
	}
	return parseShortStat(cleanOutput(output)), nil
}

// parseShortStat reads insertion/deletion totals from a --shortstat line such
// as "3 files changed, 27 insertions(+), 8 deletions(-)"; either part may be
// absent, and an empty line (clean tree) yields zeros.
func parseShortStat(out string) DiffStat {
	var ds DiffStat
	for part := range strings.SplitSeq(out, ",") {
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(fields[1], "insertion"):
			ds.Added = n
		case strings.HasPrefix(fields[1], "deletion"):
			ds.Deleted = n
		}
	}
	return ds
}

// HeadInfo returns the abbreviated hash, subject and commit time of HEAD.
func (g *Git) HeadInfo(ctx context.Context, path string) (Head, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"log", "-1", "--abbrev=8", "--format=%h%x1f%s%x1f%ct"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return Head{}, fmt.Errorf("unable to read HEAD: %w", err)
	}
	return parseHeadInfo(cleanOutput(output))
}

// parseHeadInfo splits the %h%x1f%s%x1f%ct log format into a Head. The hash is
// taken up to the first separator and the timestamp after the last one, so a
// stray %x1f inside the subject cannot corrupt either.
func parseHeadInfo(out string) (Head, error) {
	hash, rest, found := strings.Cut(out, "\x1f")
	if !found {
		return Head{}, fmt.Errorf("unexpected HEAD info: %q", out)
	}
	sep := strings.LastIndex(rest, "\x1f")
	if sep < 0 {
		return Head{}, fmt.Errorf("unexpected HEAD info: %q", out)
	}
	ts, err := strconv.ParseInt(rest[sep+1:], 10, 64)
	if err != nil {
		return Head{}, fmt.Errorf("unexpected HEAD commit time: %q", rest[sep+1:])
	}
	return Head{Hash: hash, Subject: rest[:sep], Time: time.Unix(ts, 0)}, nil
}

// CurrentPosition returns a short log description of HEAD
func (g *Git) CurrentPosition(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"log", "-1", "--color=always", "--format=%C(auto)%D %C(242)(%aN %ar)%Creset"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("unable to get current rev: %w", err)
	}
	return cleanOutput(output), nil
}

// Describe generates a version description based on tags and hash
func (g *Git) Describe(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"describe", "--tags", "--always"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("unable to describe rev: %w", err)
	}
	return cleanOutput(output), nil
}

// Diff returns a formatted string of ahead/behind counts
func (g *Git) Diff(ctx context.Context, path, branch, target string) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"rev-list", "--left-right", "--end-of-options", branch + "..." + target}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to compute diff: %w", err)
	}
	ahead, behind := 0, 0
	for rev := range strings.SplitSeq(cleanOutput(output), "\n") {
		if rev == "" {
			continue
		}
		switch rev[0] {
		case '<':
			ahead++
		case '>':
			behind++
		}
	}
	return ahead, behind, nil
}
