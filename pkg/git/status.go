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

	args := []string{
		"rev-list", "--left-right", "--count", "--end-of-options",
		branch + "..." + target,
	}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to compute diff: %w", err)
	}
	// --count prints a single "ahead<TAB>behind" line instead of one line
	// per revision, so git does the tallying.
	fields := strings.Fields(cleanOutput(output))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list count output: %q", cleanOutput(output))
	}
	ahead, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("unexpected rev-list count output: %q", cleanOutput(output))
	}
	behind, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("unexpected rev-list count output: %q", cleanOutput(output))
	}
	return ahead, behind, nil
}
