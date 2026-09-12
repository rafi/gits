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
	t.Parallel()

	requireGit(t)
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

	g := NewGit()

	t.Run("divergent branches", func(t *testing.T) {
		t.Parallel()

		ahead, behind, err := g.Diff(ctx, dir, "feature", "main")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if ahead != 3 || behind != 2 {
			t.Errorf("Diff(feature, main) = (%d, %d), want (3, 2)", ahead, behind)
		}
	})

	t.Run("equal refs", func(t *testing.T) {
		t.Parallel()

		ahead, behind, err := g.Diff(ctx, dir, "main", "main")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if ahead != 0 || behind != 0 {
			t.Errorf("Diff(main, main) = (%d, %d), want (0, 0)", ahead, behind)
		}
	})
}

// TestParseShortStat covers --shortstat lines with either or both totals
// present, and the empty line of a clean tree.
func TestParseShortStat(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			if got := parseShortStat(c.in); got != c.want {
				t.Errorf("parseShortStat(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

// TestParseHeadInfo covers the %h%x1f%s%x1f%ct%x1f%(describe:tags) log format
// split, including a describe placeholder an older git leaves unexpanded and a
// subject bearing the separator byte.
func TestParseHeadInfo(t *testing.T) {
	t.Parallel()

	head, err := parseHeadInfo("abc12345\x1fAdd feature: parse \x1f in subjects\x1f1700000000\x1fv1.0.0-2-gabc12345")
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
	if head.Describe != "v1.0.0-2-gabc12345" {
		t.Errorf("Describe = %q", head.Describe)
	}

	// A tagless repository: git expands %(describe:tags) to the empty string.
	tagless, err := parseHeadInfo("abc12345\x1fInitial commit\x1f1700000000\x1f")
	if err != nil {
		t.Fatalf("parseHeadInfo (tagless): %v", err)
	}
	if tagless.Describe != "" {
		t.Errorf("Describe = %q, want empty for a tagless repository", tagless.Describe)
	}

	// An older git (< 2.32) does not know %(describe:tags) and prints the
	// placeholder verbatim, which reads as no tag rather than a literal tag.
	old, err := parseHeadInfo("abc12345\x1fInitial commit\x1f1700000000\x1f%(describe:tags)")
	if err != nil {
		t.Fatalf("parseHeadInfo (old git): %v", err)
	}
	if old.Describe != "" {
		t.Errorf("Describe = %q, want empty when the placeholder is unexpanded", old.Describe)
	}

	if _, err := parseHeadInfo("garbage"); err == nil {
		t.Error("expected error for malformed output")
	}
	if _, err := parseHeadInfo("h\x1fs\x1fnot-a-number\x1fv1"); err == nil {
		t.Error("expected error for bad timestamp")
	}
}
