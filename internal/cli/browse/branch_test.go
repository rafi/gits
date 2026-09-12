package browse

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/pkg/git"
)

// fakeBrowseGit stubs the GitClient methods the previews call; everything else
// is inherited from clitest.FakeGit and panics if reached.
type fakeBrowseGit struct {
	clitest.FakeGit
	remotes     []string
	remoteRefs  []string
	ahead       int
	behind      int
	diffErr     error
	commitDates []string
	commitErr   error
	commitLog   string
	current     string
}

func (f fakeBrowseGit) Remotes(context.Context, string) ([]string, error) {
	return f.remotes, nil
}

func (f fakeBrowseGit) RemoteBranches(context.Context, string) ([]string, error) {
	return f.remoteRefs, nil
}

func (f fakeBrowseGit) Diff(context.Context, string, string, string) (int, int, error) {
	return f.ahead, f.behind, f.diffErr
}

func (f fakeBrowseGit) CommitDates(context.Context, string, string, int) ([]string, error) {
	return f.commitDates, f.commitErr
}

func (f fakeBrowseGit) CurrentBranch(context.Context, string) (string, error) {
	return f.current, nil
}

func (f fakeBrowseGit) Log(context.Context, string, string) (string, error) {
	return f.commitLog, nil
}

// compile-time check: fakeBrowseGit must satisfy the git client interface.
var _ git.GitClient = fakeBrowseGit{}

func TestRenderDigits(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{2, "₂"},
		{12, "₁₂"},
		{305, "₃₀₅"},
	}
	for _, tt := range tests {
		if got := renderDigits(tt.in); got != tt.want {
			t.Errorf("renderDigits(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderBranchDiffList(t *testing.T) {
	onlyMain := []string{"origin/main"}

	tests := []struct {
		name        string
		ahead       int
		behind      int
		wantSubstrs []string
	}{
		{"in sync", 0, 0, []string{"✓", "origin", "main"}},
		{"ahead only", 2, 0, []string{"▲2"}},
		{"behind only", 0, 3, []string{"▼3"}},
		{"ahead and behind", 2, 3, []string{"▲2 ▼3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := fakeBrowseGit{remoteRefs: onlyMain, ahead: tt.ahead, behind: tt.behind}
			out := renderBranchDiffList("/repo", "main", []string{"origin"}, clitest.New(t, g).RuntimeCLI)
			for _, want := range tt.wantSubstrs {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\ngot: %q", want, out)
				}
			}
		})
	}

	t.Run("branches listed in stable sorted order", func(t *testing.T) {
		g := fakeBrowseGit{remoteRefs: []string{
			"origin/main", "origin/master", "origin/dev", "origin/next",
			"fork/main", "fork/master", "fork/dev", "fork/next",
		}}
		first := renderBranchDiffList("/repo", "main", []string{"origin", "fork"}, clitest.New(t, g).RuntimeCLI)
		lines := strings.Split(strings.TrimSpace(first), "\n")
		if len(lines) < 4 {
			t.Fatalf("expected multiple branch lines, got %d:\n%s", len(lines), first)
		}
		for range 10 {
			again := renderBranchDiffList("/repo", "main", []string{"origin", "fork"}, clitest.New(t, g).RuntimeCLI)
			if again != first {
				t.Fatalf("output order not stable across renders:\n%q\nvs\n%q", first, again)
			}
		}
	})

	t.Run("diff error renders N/A row instead of blanking panel", func(t *testing.T) {
		g := fakeBrowseGit{remoteRefs: onlyMain, diffErr: context.DeadlineExceeded}
		deps := clitest.New(t, g)
		out := renderBranchDiffList("/repo", "main", []string{"origin"}, deps.RuntimeCLI)
		for _, want := range []string{deps.Settings.Icons.NA, "origin", "main"} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\ngot: %q", want, out)
			}
		}
	})
}

func TestRenderBranchChart(t *testing.T) {
	// Freeze the day axis so commit dates map deterministically.
	origNow := now
	now = func() time.Time { return time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = origNow })

	ctx := context.Background()
	repo := domain.Repository{AbsPath: "/repo"}

	t.Run("scales bars to the busiest day", func(t *testing.T) {
		// 2026-01-14 has 2 commits (the max), 2026-01-13 has 1.
		g := fakeBrowseGit{commitDates: []string{"2026-01-14", "2026-01-14", "2026-01-13"}}
		chart, err := renderBranchChart(ctx, g, repo, "main", 20)
		if err != nil {
			t.Fatalf("renderBranchChart: %v", err)
		}
		// barMaxSize = width(20) - dateLength(11) = 9; busiest day fills it.
		if !strings.Contains(chart, "2026-01-14 "+strings.Repeat("▇", 9)) {
			t.Errorf("busiest day not full-width\ngot: %q", chart)
		}
		// highest > 1 renders the subscript digit header.
		if !strings.Contains(chart, "₂") {
			t.Errorf("missing digit header for highest=2\ngot: %q", chart)
		}
	})

	t.Run("empty state when no commits", func(t *testing.T) {
		g := fakeBrowseGit{commitDates: nil}
		chart, err := renderBranchChart(ctx, g, repo, "main", 20)
		if err != nil {
			t.Fatalf("renderBranchChart: %v", err)
		}
		want := "\n<no commits in the last 7 days>"
		if chart != want {
			t.Errorf("empty-state chart = %q, want %q", chart, want)
		}
	})
}
