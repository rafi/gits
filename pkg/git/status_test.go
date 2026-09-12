package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDiffCounts builds two divergent branches and asserts Diff tallies the
// left (<, ahead) and right (>, behind) revisions from `rev-list --left-right`.
func TestDiffCounts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git executable not available: %v", err)
	}
	ctx := context.Background()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	commit := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
		run("commit", "-m", name)
	}

	run("init", "-b", "main")
	commit("base")
	run("branch", "feature")
	// main gains 2 commits, feature gains 3 — diverging from the shared base.
	commit("m1")
	commit("m2")
	run("checkout", "feature")
	commit("f1")
	commit("f2")
	commit("f3")

	g, err := NewGit()
	if err != nil {
		t.Fatalf("NewGit: %v", err)
	}

	t.Run("divergent branches", func(t *testing.T) {
		ahead, behind, err := g.Diff(ctx, dir, "feature", "main")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if ahead != 3 || behind != 2 {
			t.Errorf("Diff(feature, main) = (%d, %d), want (3, 2)", ahead, behind)
		}
	})

	t.Run("equal refs", func(t *testing.T) {
		ahead, behind, err := g.Diff(ctx, dir, "main", "main")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if ahead != 0 || behind != 0 {
			t.Errorf("Diff(main, main) = (%d, %d), want (0, 0)", ahead, behind)
		}
	})
}

// TestModifiedCountsStagedChanges proves Modified sees the full dirty state:
// a fully staged (but uncommitted) change must count, not only unstaged
// worktree edits.
func TestModifiedCountsStagedChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git executable not available: %v", err)
	}
	ctx := context.Background()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-b", "main")
	write("staged.txt", "v1")
	write("unstaged.txt", "v1")
	run("add", ".")
	run("commit", "-m", "base")

	g, err := NewGit()
	if err != nil {
		t.Fatalf("NewGit: %v", err)
	}

	t.Run("clean repo counts zero", func(t *testing.T) {
		got, err := g.Modified(ctx, dir)
		if err != nil {
			t.Fatalf("Modified: %v", err)
		}
		if got != 0 {
			t.Errorf("Modified(clean) = %d, want 0", got)
		}
	})

	t.Run("staged and unstaged changes both count", func(t *testing.T) {
		write("staged.txt", "v2")
		run("add", "staged.txt") // fully staged, invisible to `git diff`
		write("unstaged.txt", "v2")

		got, err := g.Modified(ctx, dir)
		if err != nil {
			t.Fatalf("Modified: %v", err)
		}
		if got != 2 {
			t.Errorf("Modified(1 staged + 1 unstaged) = %d, want 2", got)
		}
	})

	t.Run("untracked files do not count as modified", func(t *testing.T) {
		write("new.txt", "v1")
		got, err := g.Modified(ctx, dir)
		if err != nil {
			t.Fatalf("Modified: %v", err)
		}
		if got != 2 {
			t.Errorf("Modified(with untracked) = %d, want 2 (untracked excluded)", got)
		}
	})
}

// TestWorkingStateUnstagedFirstLine: an unstaged entry leading the porcelain
// output keeps its " M" prefix — a whole-output TrimSpace would strip the
// leading space and miscount it as staged.
func TestWorkingStateUnstagedFirstLine(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git executable not available: %v", err)
	}
	ctx := context.Background()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-b", "main")
	write("a.txt", "v1")
	run("add", ".")
	run("commit", "-m", "base")
	write("a.txt", "v2") // modified, never staged

	g, err := NewGit()
	if err != nil {
		t.Fatalf("NewGit: %v", err)
	}
	wt, err := g.WorkingState(ctx, dir)
	if err != nil {
		t.Fatalf("WorkingState: %v", err)
	}
	if want := (WorkTree{Unstaged: 1}); wt != want {
		t.Errorf("WorkingState = %+v, want %+v", wt, want)
	}
}

// TestParsePorcelain covers the XY tally: staged, unstaged, untracked,
// conflicts counting as both, and ignored entries skipped.
func TestParsePorcelain(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want WorkTree
	}{
		{"empty", "", WorkTree{}},
		{"untracked", "?? new.txt", WorkTree{Untracked: 1}},
		{"staged", "M  staged.txt", WorkTree{Staged: 1}},
		{"unstaged", " M edited.txt", WorkTree{Unstaged: 1}},
		{"both", "MM both.txt", WorkTree{Staged: 1, Unstaged: 1}},
		{"renamed", "R  old -> new", WorkTree{Staged: 1}},
		{"conflict", "UU clash.txt", WorkTree{Staged: 1, Unstaged: 1}},
		{"ignored", "!! vendor/", WorkTree{}},
		{
			"mixed",
			"M  a\n M b\n?? c\nA  d\nMM e",
			WorkTree{Staged: 3, Unstaged: 2, Untracked: 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parsePorcelain(c.in); got != c.want {
				t.Errorf("parsePorcelain(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

// TestParseShortStat covers --shortstat lines with either or both totals
// present, and the empty line of a clean tree.
func TestParseShortStat(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want DiffStat
	}{
		{"clean", "", DiffStat{}},
		{
			"both",
			"3 files changed, 27 insertions(+), 8 deletions(-)",
			DiffStat{Added: 27, Deleted: 8},
		},
		{
			"singular insertion",
			"1 file changed, 1 insertion(+)",
			DiffStat{Added: 1},
		},
		{
			"deletions only",
			"2 files changed, 5 deletions(-)",
			DiffStat{Deleted: 5},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseShortStat(c.in); got != c.want {
				t.Errorf("parseShortStat(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

// TestParseHeadInfo covers the %h%x1f%s%x1f%ct log format split.
func TestParseHeadInfo(t *testing.T) {
	head, err := parseHeadInfo("abc12345\x1fAdd feature: parse \x1f in subjects\x1f1700000000")
	if err != nil {
		t.Fatalf("parseHeadInfo: %v", err)
	}
	if head.Hash != "abc12345" {
		t.Errorf("Hash = %q", head.Hash)
	}
	if head.Subject != "Add feature: parse \x1f in subjects" {
		t.Errorf("Subject = %q", head.Subject)
	}
	if head.Time.Unix() != 1700000000 {
		t.Errorf("Time = %v", head.Time)
	}

	if _, err := parseHeadInfo("garbage"); err == nil {
		t.Error("expected error for malformed output")
	}
	if _, err := parseHeadInfo("h\x1fs\x1fnot-a-number"); err == nil {
		t.Error("expected error for bad timestamp")
	}
}
