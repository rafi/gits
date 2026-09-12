package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// shortStatFields is the shortest `--shortstat` clause that states
	// anything: a count and its noun.
	shortStatFields = 2
	// revListCountFields is what `rev-list --left-right --count` prints:
	// ahead and behind.
	revListCountFields = 2
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
	// Describe is `git describe --tags` for HEAD, empty when no tag is
	// reachable. It is carried here so a single `git log` pass answers both
	// the commit and the version, rather than a second `describe` subprocess.
	Describe string
}

// CurrentBranch returns the checked-out branch's short name, or "HEAD" when
// HEAD is detached.
func (g *Git) CurrentBranch(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"rev-parse", "--abbrev-ref", headRev}
	abbrRef, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("unable to find ref: %w", err)
	}
	return cleanOutput(abbrRef), nil
}

// WorkingDiff sums uncommitted line changes — staged and unstaged, like
// `git diff HEAD --shortstat`. Untracked files contribute nothing, matching
// diff semantics.
func (g *Git) WorkingDiff(ctx context.Context, path string) (DiffStat, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	output, err := g.Exec(ctx, path, []string{"diff", "--shortstat", headRev})
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
		if len(fields) < shortStatFields {
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

// HeadInfo returns the abbreviated hash, subject, commit time and tag
// description of HEAD in one `git log` pass. `%(describe:tags)` needs git
// ≥ 2.32 (2021); an older git renders the placeholder literally, which
// parseHeadInfo treats as no tag.
func (g *Git) HeadInfo(ctx context.Context, path string) (Head, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{
		"log", "-1", "--abbrev=8",
		"--format=%h%x1f%s%x1f%ct%x1f%(describe:tags)",
	}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return Head{}, fmt.Errorf("unable to read HEAD: %w", err)
	}
	return parseHeadInfo(cleanOutput(output))
}

// parseHeadInfo splits the %h%x1f%s%x1f%ct%x1f%(describe:tags) log format into
// a Head. The hash is taken up to the first separator; the describe string
// after the last and the commit time before it, so a stray %x1f inside the
// subject cannot corrupt any of the fixed-shape fields around it. An older git
// that does not know %(describe:tags) emits the placeholder verbatim, which is
// read as no tag.
func parseHeadInfo(out string) (Head, error) {
	hash, rest, found := strings.Cut(out, "\x1f")
	if !found {
		return Head{}, fmt.Errorf("unexpected HEAD info: %q", out)
	}
	descSep := strings.LastIndex(rest, "\x1f")
	if descSep < 0 {
		return Head{}, fmt.Errorf("unexpected HEAD info: %q", out)
	}
	describe := rest[descSep+1:]
	if describe == "%(describe:tags)" {
		describe = "" // older git left the placeholder unexpanded
	}
	rest = rest[:descSep]
	tsSep := strings.LastIndex(rest, "\x1f")
	if tsSep < 0 {
		return Head{}, fmt.Errorf("unexpected HEAD info: %q", out)
	}
	ts, err := strconv.ParseInt(rest[tsSep+1:], 10, 64)
	if err != nil {
		return Head{}, fmt.Errorf("unexpected HEAD commit time: %q", rest[tsSep+1:])
	}
	return Head{
		Hash:     hash,
		Subject:  rest[:tsSep],
		Time:     time.Unix(ts, 0),
		Describe: describe,
	}, nil
}

// Diff returns a formatted string of ahead/behind counts.
func (g *Git) Diff(ctx context.Context, path, branch, target string) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{
		"rev-list", "--left-right", "--count", argEndOfOptions,
		branch + "..." + target,
	}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to compute diff: %w", err)
	}
	// --count prints a single "ahead<TAB>behind" line instead of one line
	// per revision, so git does the tallying.
	fields := strings.Fields(cleanOutput(output))
	if len(fields) != revListCountFields {
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
