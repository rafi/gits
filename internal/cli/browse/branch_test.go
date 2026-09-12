package browse

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

// fakeBrowseGit stubs the GitClient methods the branch preview calls.
type fakeBrowseGit struct {
	git.GitClient
	hasBranch   func(remote, branch string) bool
	ahead       int
	behind      int
	diffErr     error
	commitDates []string
	commitErr   error
}

func (f fakeBrowseGit) HasRemoteBranch(_ context.Context, _, remote, branch string) bool {
	return f.hasBranch(remote, branch)
}

func (f fakeBrowseGit) Diff(context.Context, string, string, string) (int, int, error) {
	return f.ahead, f.behind, f.diffErr
}

func (f fakeBrowseGit) CommitDates(context.Context, string, string, int) ([]string, error) {
	return f.commitDates, f.commitErr
}

func browseDeps(g git.GitClient) types.RuntimeCLI {
	return types.RuntimeCLI{
		Theme: config.NewThemeDefault(),
		Runtime: types.Runtime{
			Ctx: context.Background(),
			Git: g,
		},
	}
}

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
	onlyMain := func(_, branch string) bool { return branch == "main" }

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
			g := fakeBrowseGit{hasBranch: onlyMain, ahead: tt.ahead, behind: tt.behind}
			out, err := renderBranchDiffList("/repo", "main", []string{"origin"}, browseDeps(g))
			if err != nil {
				t.Fatalf("renderBranchDiffList: %v", err)
			}
			for _, want := range tt.wantSubstrs {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\ngot: %q", want, out)
				}
			}
		})
	}

	t.Run("diff error propagates", func(t *testing.T) {
		g := fakeBrowseGit{hasBranch: onlyMain, diffErr: context.DeadlineExceeded}
		if _, err := renderBranchDiffList("/repo", "main", []string{"origin"}, browseDeps(g)); err == nil {
			t.Error("renderBranchDiffList = nil error, want propagated Diff error")
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
