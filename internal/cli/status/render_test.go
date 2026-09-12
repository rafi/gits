package status

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/cli/walk"
)

func fixtureStatuses() []*repoStatus {
	now := time.Now()
	return []*repoStatus{
		{
			repo:     domain.Repository{Name: "api", State: domain.RepoStateOK},
			title:    "api",
			branch:   "main",
			staged:   1,
			unstaged: 11,
			ahead:    2,
			version:  "v2.1.0",
			commit:   "f3a9c2d1",
			message:  "Add rate limiter",
			when:     now.Add(-2 * time.Hour),
		},
		{
			repo:      domain.Repository{Name: "web", State: domain.RepoStateOK},
			title:     "web",
			branch:    "develop",
			untracked: 4567,
			behind:    1,
			version:   "v0.9.0",
			commit:    "0e631add",
			message:   "Initial commit",
			when:      now.Add(-26 * time.Hour),
		},
		{
			repo:    domain.Repository{Name: "infra", State: domain.RepoStateNotCloned},
			title:   "infra",
			message: "not cloned",
			err:     errFixture,
		},
	}
}

var errFixture = &fixtureErr{}

type fixtureErr struct{}

func (e *fixtureErr) Error() string { return "not cloned" }

// TestRenderTableRows: every repo renders exactly one line, wide counts grow
// the column instead of wrapping, and all expected fields appear.
func TestRenderTableRows(t *testing.T) {
	out := renderTable(fixtureStatuses(), 0, Options{}, statusDeps(t, fakeGit{}))
	plain := ansi.Strip(out)
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")

	if len(lines) != 4 { // header + three repos
		t.Fatalf("expected 4 lines (header + 3 rows), got %d:\n%s", len(lines), plain)
	}
	for _, want := range []string{"Repo", "Branch", "Status", "Δ±", "Upstream⇅",
		"Version", "Commit", "Age", "Message"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("header %q missing %q", lines[0], want)
		}
	}
	for _, want := range []string{"≠12", "?4K", "⇡2", "⇣1", "v2.1.0",
		"f3a9c2d1", "2h", "1d", "not cloned"} {
		if !strings.Contains(plain, want) {
			t.Errorf("table missing %q:\n%s", want, plain)
		}
	}
	// Gutter glyphs: worktree rows +, not-cloned /.
	if !strings.HasPrefix(lines[1], "  + ") || !strings.HasPrefix(lines[3], "  / ") {
		t.Errorf("unexpected gutters:\n%s", plain)
	}
}

// TestRenderTableSlotAlignment: the status symbol column occupies identical
// positions across rows so flags line up vertically.
func TestRenderTableSlotAlignment(t *testing.T) {
	sts := fixtureStatuses()
	out := renderTable(sts, 0, Options{}, statusDeps(t, fakeGit{}))
	lines := strings.Split(strings.TrimRight(ansi.Strip(out), "\n"), "\n")

	// Row 1 has staged+unstaged (`+!`), row 2 untracked (`?`): the untracked
	// flag of row 2 must sit exactly two columns right of row 1's staged flag.
	idx1 := strings.Index(lines[1], "+!")
	idx2 := strings.Index(lines[2], "?")
	if idx1 < 0 || idx2 < 0 {
		t.Fatalf("expected status flags in rows:\n%s\n%s", lines[1], lines[2])
	}
	if idx2 != idx1+2 {
		t.Errorf("untracked slot at %d, want %d (aligned slots):\n%s\n%s",
			idx2, idx1+2, lines[1], lines[2])
	}
}

// TestRenderGroupsTitlesAndFooter: project headers precede tables, and the
// footer summarizes counts to the error writer.
func TestRenderGroupsTitlesAndFooter(t *testing.T) {
	sts := fixtureStatuses()
	results := make([]*walk.RepoResult, len(sts))
	for i, st := range sts {
		results[i] = &walk.RepoResult{Payload: st}
	}
	groups := []walk.GroupResult{{
		Project: domain.Project{Name: "acme"},
		Results: results,
	}}

	deps := clitest.New(t, nil)
	renderGroups(groups, true, Options{}, deps.RuntimeCLI)

	if !strings.Contains(deps.Result(), ":: acme") {
		t.Errorf("missing project title:\n%s", deps.Result())
	}
	footer := deps.Diagnostic()
	for _, want := range []string{"○ Showing 3 repos", "2 with changes", "1 ahead"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q missing %q", footer, want)
		}
	}
}

// TestRenderGroupsDirtyFilter: --dirty drops clean repos but keeps error rows,
// hides projects left with no rows, and reports the hidden count in the footer.
func TestRenderGroupsDirtyFilter(t *testing.T) {
	sts := fixtureStatuses() // api dirty, web dirty (untracked), infra error
	clean := &repoStatus{
		repo:    domain.Repository{Name: "tidy", State: domain.RepoStateOK},
		title:   "tidy",
		branch:  "main",
		version: "v1.0.0",
	}
	toResults := func(sts ...*repoStatus) []*walk.RepoResult {
		results := make([]*walk.RepoResult, len(sts))
		for i, st := range sts {
			results[i] = &walk.RepoResult{Payload: st}
		}
		return results
	}
	groups := []walk.GroupResult{
		{Project: domain.Project{Name: "acme"}, Results: toResults(sts[0], clean, sts[2])},
		{Project: domain.Project{Name: "pristine"}, Results: toResults(clean)},
	}

	deps := clitest.New(t, nil)
	renderGroups(groups, true, Options{Dirty: true}, deps.RuntimeCLI)

	plain := deps.Result()
	for _, want := range []string{"api", "not cloned"} {
		if !strings.Contains(plain, want) {
			t.Errorf("dirty output missing %q:\n%s", want, plain)
		}
	}
	for _, banned := range []string{"tidy", ":: pristine"} {
		if strings.Contains(plain, banned) {
			t.Errorf("dirty output should hide %q:\n%s", banned, plain)
		}
	}
	footer := deps.Diagnostic()
	for _, want := range []string{"○ Showing 2 repos", "2 hidden"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q missing %q", footer, want)
		}
	}
}

// TestRenderGroupsUnsyncedFilter: --unsynced keeps only repos ahead or behind
// upstream (no-upstream repos are not unsynced), and combined with --dirty the
// filters are a union.
func TestRenderGroupsUnsyncedFilter(t *testing.T) {
	ok := func(name string) domain.Repository {
		return domain.Repository{Name: name, State: domain.RepoStateOK}
	}
	dirtyInSync := &repoStatus{repo: ok("edited"), title: "edited", unstaged: 2}
	cleanAhead := &repoStatus{repo: ok("racer"), title: "racer", ahead: 3}
	noUp := &repoStatus{repo: ok("loner"), title: "loner", noUpstream: true}
	results := make([]*walk.RepoResult, 0, 3)
	for _, st := range []*repoStatus{dirtyInSync, cleanAhead, noUp} {
		results = append(results, &walk.RepoResult{Payload: st})
	}
	groups := []walk.GroupResult{{Project: domain.Project{Name: "acme"}, Results: results}}

	deps := clitest.New(t, nil)
	renderGroups(groups, true, Options{Unsynced: true}, deps.RuntimeCLI)
	plain := deps.Result()
	if !strings.Contains(plain, "racer") {
		t.Errorf("unsynced output missing ahead repo:\n%s", plain)
	}
	for _, banned := range []string{"edited", "loner"} {
		if strings.Contains(plain, banned) {
			t.Errorf("unsynced output should hide %q:\n%s", banned, plain)
		}
	}
	if footer := deps.Diagnostic(); !strings.Contains(footer, "2 hidden") {
		t.Errorf("footer %q missing hidden count", footer)
	}

	// Union: --dirty --unsynced shows dirty-in-sync and clean-ahead repos.
	union := clitest.New(t, nil)
	renderGroups(groups, true, Options{Dirty: true, Unsynced: true}, union.RuntimeCLI)
	plain = union.Result()
	for _, want := range []string{"edited", "racer"} {
		if !strings.Contains(plain, want) {
			t.Errorf("union output missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "loner") {
		t.Errorf("union output should hide no-upstream repo:\n%s", plain)
	}
}

// TestRenderTableClampsToTerminal: when the natural table is wider than the
// terminal, columns contract and cells truncate with … so no row exceeds the
// terminal width (no hard-wrapping), while count/status glyphs survive.
func TestRenderTableClampsToTerminal(t *testing.T) {
	sts := fixtureStatuses()
	sts[0].message = strings.Repeat("very long commit subject ", 8)
	const width = 72

	out := renderTable(sts, width, Options{}, statusDeps(t, fakeGit{}))
	plain := ansi.Strip(out)
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")

	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (header + 3 rows), got %d:\n%s", len(lines), plain)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("line %d width %d exceeds terminal width %d:\n%s", i, w, width, line)
		}
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("expected truncated cells with …:\n%s", plain)
	}
	// Pinned columns must survive the squeeze intact: counts, flags, commit
	// hashes, ages and their headers never truncate.
	for _, want := range []string{"≠12", "?4K", "⇡2", "⇣1", "+!",
		"f3a9c2d1", "0e631add", "2h", "1d", "Upstream⇅", "Commit"} {
		if !strings.Contains(plain, want) {
			t.Errorf("pinned column content %q lost in clamping:\n%s", want, plain)
		}
	}
}

// TestRenderTableStatColumn: --stat adds the HEAD± column with green/red line
// counts between Status and Δ±; without it the column is absent entirely.
func TestRenderTableStatColumn(t *testing.T) {
	sts := fixtureStatuses()
	sts[0].added, sts[0].deleted = 27, 8
	sts[1].added = 4321

	out := renderTable(sts, 0, Options{Stat: true}, statusDeps(t, fakeGit{}))
	plain := ansi.Strip(out)
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")

	head := strings.Index(lines[0], "HEAD±")
	delta := strings.Index(lines[0], "Δ±")
	if head < 0 || delta < 0 || head > delta {
		t.Fatalf("expected HEAD± before Δ± in header:\n%s", lines[0])
	}
	for _, want := range []string{"+27", "-8", "+4K"} {
		if !strings.Contains(plain, want) {
			t.Errorf("stat table missing %q:\n%s", want, plain)
		}
	}
	// The clean/not-cloned row leaves the cell blank rather than showing 0.
	if strings.Contains(lines[3], "+0") || strings.Contains(lines[3], "-0") {
		t.Errorf("zero counts should render blank:\n%s", lines[3])
	}

	plain = ansi.Strip(renderTable(sts, 0, Options{}, statusDeps(t, fakeGit{})))
	if strings.Contains(plain, "HEAD±") || strings.Contains(plain, "+27") {
		t.Errorf("HEAD± column rendered without --stat:\n%s", plain)
	}
}

func TestCompactCount(t *testing.T) {
	cases := map[int]string{
		0:     "0",
		42:    "42",
		99:    "99",
		100:   "1C",
		632:   "6C",
		1000:  "1K",
		4567:  "4K",
		10000: "∞",
	}
	for n, want := range cases {
		if got := compactCount(n); got != want {
			t.Errorf("compactCount(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestShortAge(t *testing.T) {
	now := time.Now()
	cases := []struct {
		age  time.Duration
		want string
	}{
		{30 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{26 * time.Hour, "1d"},
		{8 * 24 * time.Hour, "1w"},
		{40 * 24 * time.Hour, "1mo"},
		{800 * 24 * time.Hour, "2y"},
	}
	for _, c := range cases {
		if got := shortAge(now.Add(-c.age), now); got != c.want {
			t.Errorf("shortAge(-%v) = %q, want %q", c.age, got, c.want)
		}
	}
	if got := shortAge(time.Time{}, now); got != "" {
		t.Errorf("shortAge(zero) = %q, want empty", got)
	}
}

// TestStatusSlotsWidthWithWideIcons: the four fixed slots must occupy the
// same width on every row, including when the error icon is 2 cells wide —
// an error row, an N/A row and a clean row all align.
func TestStatusSlotsWidthWithWideIcons(t *testing.T) {
	icons := domain.Icons{DiffError: "✗✗"} // 2-cell error icon
	icons.ApplyDefaults()
	th := config.NewThemeDefault()
	sts := []*repoStatus{
		{repo: domain.Repository{State: domain.RepoStateOK}},
		{repo: domain.Repository{State: domain.RepoStateOK}, staged: 1, err: errFixture},
		{repo: domain.Repository{State: domain.RepoStateNotCloned}, err: errFixture},
		{repo: domain.Repository{State: domain.RepoStateOK}, ahead: 3},
	}
	widths := newSlotWidths(icons)
	want := lipgloss.Width(statusSlots(sts[0], icons, th, widths))
	for i, st := range sts[1:] {
		if got := lipgloss.Width(statusSlots(st, icons, th, widths)); got != want {
			t.Errorf("row %d slot width = %d, want %d", i+1, got, want)
		}
	}
}
