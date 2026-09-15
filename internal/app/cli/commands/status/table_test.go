package status

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/infra/git"
	"github.com/rafi/gits/internal/runtime/command"
	"github.com/rafi/gits/internal/service/status"
)

// fixture builds one row bound to a repository in the given state.
func fixture(name string, state domain.RepoState, st status.Report) *status.Report {
	st.Repo = command.Repo{
		Name: name, AbsPath: "/code/acme/" + name, State: state,
		Path: name,
	}
	return &st
}

func fixtureStatuses() []*status.Report {
	now := time.Now()
	return []*status.Report{
		fixture("api", domain.RepoStateOK, status.Report{
			Branch: "main", Ahead: 2,
			Staged: 1, Unstaged: 11,
			Compared: true,
			Version:  "v2.1.0",
			Head:     git.Head{Hash: "f3a9c2d1", Subject: "Add rate limiter", Time: now.Add(-2 * time.Hour)},
		}),
		fixture("web", domain.RepoStateOK, status.Report{
			Branch: "develop", Behind: 1,
			Untracked: 4567,
			Compared:  true,
			Version:   "v0.9.0",
			Head:      git.Head{Hash: "0e631add", Subject: "Initial commit", Time: now.Add(-26 * time.Hour)},
		}),
		fixture("infra", domain.RepoStateNotCloned, status.Report{Err: errFixture}),
	}
}

// project bundles rows under a project node so the renderer's tree walk
// finds them.
func project(name string, sts ...*status.Report) domain.Project {
	p := domain.Project{Name: name}
	for _, st := range sts {
		p.Repos = append(p.Repos, st.Repo.Repository)
	}
	return p
}

// resultsOf wraps rows as the run hands them to the renderer, under root.
func resultsOf(root domain.Project, sts ...*status.Report) command.Results[*status.Report] {
	res := command.Results[*status.Report]{Project: root}
	for _, st := range sts {
		res.Results = append(res.Results, command.Result[*status.Report]{Repo: st.Repo, Value: st, Err: st.Err})
	}
	return res
}

var errFixture = &fixtureError{}

type fixtureError struct{}

func (e *fixtureError) Error() string { return "not cloned" }

// TestRenderTableRows: every repo renders exactly one line, wide counts grow
// the column instead of wrapping, and all expected fields appear.
func TestRenderTableRows(t *testing.T) {
	t.Parallel()

	out := renderTable(fixtureStatuses(), 0, Options{}, statusDeps(t, fakeGit{}))
	plain := ansi.Strip(out)
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")

	if len(lines) != 4 { // header + three repos
		t.Fatalf("expected 4 lines (header + 3 rows), got %d:\n%s", len(lines), plain)
	}
	for _, want := range []string{"Repo", "Branch", "Status", "Δ±", "Upstream⇅",
		"Commit", "Age", "Message"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("header %q missing %q", lines[0], want)
		}
	}
	if strings.Contains(plain, "Version") || strings.Contains(plain, "v2.1.0") {
		t.Errorf("table still renders the Version column:\n%s", plain)
	}
	for _, want := range []string{"≠12", "?4K", "⇡2", "⇣1",
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
	t.Parallel()

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

// TestRenderTablesFooter: the table is Result Output and the footer
// summarizes counts to the error writer; no project title precedes it.
func TestRenderTablesFooter(t *testing.T) {
	t.Parallel()

	sts := fixtureStatuses()
	deps := clitest.New(t, nil)
	renderTables(resultsOf(project("acme", sts...), sts...), Options{}, deps.RuntimeCLI)

	if got := deps.Result(); strings.Contains(got, "acme") || strings.Contains(got, "::") {
		t.Errorf("Result Output = %q, want no project title", got)
	}
	footer := deps.Diagnostic()
	for _, want := range []string{"○ Showing 3 repos", "2 with changes", "1 ahead"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q missing %q", footer, want)
		}
	}
}

// TestRenderTablesSingleRepoHasNoFooter: one visible row summarizes itself, so
// the `○ Showing 1 repo` footer would only restate the table.
func TestRenderTablesSingleRepoHasNoFooter(t *testing.T) {
	t.Parallel()

	st := fixtureStatuses()[0]
	deps := clitest.New(t, nil)
	renderTables(resultsOf(project("acme", st), st), Options{}, deps.RuntimeCLI)

	if got := deps.Result(); !strings.Contains(got, "api") {
		t.Errorf("Result Output = %q, want the repository's row", got)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want no footer for a single repository", got)
	}
}

// TestRenderTablesSingleRepoWithHiddenKeepsFooter: a filtered view still needs
// the footer, because the hidden count is the only thing that reports the rows
// the filter removed.
func TestRenderTablesSingleRepoWithHiddenKeepsFooter(t *testing.T) {
	t.Parallel()

	sts := fixtureStatuses()
	clean := fixture("tidy", domain.RepoStateOK, status.Report{Branch: "main"})
	root := project("acme", sts[0], clean)

	deps := clitest.New(t, nil)
	renderTables(resultsOf(root, sts[0], clean), Options{Dirty: true}, deps.RuntimeCLI)

	footer := deps.Diagnostic()
	for _, want := range []string{"○ Showing 1 repo", "1 hidden"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q missing %q", footer, want)
		}
	}
}

// TestRenderTablesSeparatesAndSkips: the tables of two projects are separated
// by a blank line, a project with no rows prints nothing, and a repository
// the run never started leaves no row at all.
func TestRenderTablesSeparatesAndSkips(t *testing.T) {
	t.Parallel()

	sts := fixtureStatuses()
	vendor := project("vendor", sts[1])
	hollow := domain.Project{Name: "hollow"}
	root := project("acme", sts[0], sts[2]) // sts[2] is in the tree but never ran
	root.SubProjects = []domain.Project{hollow, vendor}

	deps := clitest.New(t, nil)
	renderTables(resultsOf(root, sts[0], sts[1]), Options{}, deps.RuntimeCLI)

	plain := deps.Result()
	for _, want := range []string{"api", "web"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output missing %q:\n%s", want, plain)
		}
	}
	for _, banned := range []string{"infra", "hollow"} {
		if strings.Contains(plain, banned) {
			t.Errorf("output has %q, want nothing for a row that never ran or a project with none:\n%s",
				banned, plain)
		}
	}
	if !strings.Contains(plain, "\n\n") {
		t.Errorf("tables are not separated by a blank line:\n%s", plain)
	}
	// The two rows that did render are the whole footer count.
	if footer := deps.Diagnostic(); !strings.Contains(footer, "○ Showing 2 repos") {
		t.Errorf("footer %q, want only the rendered rows counted", footer)
	}
}

// TestRenderTablesDirtyFilter: --dirty drops clean repos but keeps error rows,
// hides projects left with no rows, and reports the hidden count in the footer.
func TestRenderTablesDirtyFilter(t *testing.T) {
	t.Parallel()

	sts := fixtureStatuses() // api dirty, web dirty (untracked), infra error
	clean := fixture("tidy", domain.RepoStateOK, status.Report{
		Branch: "main", Version: "v1.0.0",
	})
	pristine := project("pristine", clean)
	root := project("acme", sts[0], clean, sts[2])
	root.SubProjects = []domain.Project{pristine}

	deps := clitest.New(t, nil)
	renderTables(resultsOf(root, sts[0], clean, sts[2], clean), Options{Dirty: true}, deps.RuntimeCLI)

	plain := deps.Result()
	for _, want := range []string{"api", "not cloned"} {
		if !strings.Contains(plain, want) {
			t.Errorf("dirty output missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "tidy") {
		t.Errorf("dirty output should hide the clean repository:\n%s", plain)
	}
	footer := deps.Diagnostic()
	for _, want := range []string{"○ Showing 2 repos", "2 hidden"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q missing %q", footer, want)
		}
	}
}

// TestRenderTablesUnsyncedFilter: --unsynced keeps only repos ahead or behind
// upstream (no-upstream repos are not unsynced), and combined with --dirty the
// filters are a union.
func TestRenderTablesUnsyncedFilter(t *testing.T) {
	t.Parallel()

	dirtyInSync := fixture("edited", domain.RepoStateOK, status.Report{
		Unstaged: 2, Compared: true,
	})
	cleanAhead := fixture("racer", domain.RepoStateOK, status.Report{
		Ahead: 3, Compared: true,
	})
	noUp := fixture("loner", domain.RepoStateOK, status.Report{})
	sts := []*status.Report{dirtyInSync, cleanAhead, noUp}
	res := resultsOf(project("acme", sts...), sts...)

	deps := clitest.New(t, nil)
	renderTables(res, Options{Unsynced: true}, deps.RuntimeCLI)
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
	renderTables(res, Options{Dirty: true, Unsynced: true}, union.RuntimeCLI)
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
	t.Parallel()

	sts := fixtureStatuses()
	sts[0].Head.Subject = strings.Repeat("very long commit subject ", 8)
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
	// Pinned columns must survive the squeeze intact: counts, flags and
	// their headers never truncate. Commit and Age are optional at this
	// width and may have been dropped — TestRenderTableNarrowDropOrder
	// covers that — but whatever remains is whole.
	for _, want := range []string{"≠12", "?4K", "⇡2", "⇣1", "+!", "Upstream⇅"} {
		if !strings.Contains(plain, want) {
			t.Errorf("pinned column content %q lost in clamping:\n%s", want, plain)
		}
	}
}

// TestRenderTableNarrowKeepsRepoReadable: as the terminal narrows the table
// sheds optional columns — age, then commit, then the counts, then branch —
// instead of squeezing Repo and Message into unreadable stubs. Whatever is
// dropped, every row still names its repository in full and keeps its status
// glyphs and message.
func TestRenderTableNarrowKeepsRepoReadable(t *testing.T) {
	t.Parallel()

	for _, width := range []int{70, 60, 50, 40} {
		sts := fixtureStatuses()
		sts[0].Repo.Path = "platform-api"
		sts[0].Head.Subject = strings.Repeat("very long commit subject ", 4)

		out := renderTable(sts, width, Options{}, statusDeps(t, fakeGit{}))
		plain := ansi.Strip(out)
		lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")

		if len(lines) != 4 {
			t.Fatalf("width %d: expected 4 lines, got %d:\n%s", width, len(lines), plain)
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("width %d: line %d is %d wide:\n%s", width, i, w, line)
			}
		}
		// The columns that survive at every width.
		for _, want := range []string{"Repo", "Status", "Message",
			"platform-api", "web", "infra", "not cloned"} {
			if !strings.Contains(plain, want) {
				t.Errorf("width %d: dropped essential content %q:\n%s", width, want, plain)
			}
		}
	}
}

// TestRenderTableExtremelyNarrow: even at widths no table can honestly fill,
// rows never wrap or overflow, and each still carries a stub of its
// repository name to tell it from the others.
func TestRenderTableExtremelyNarrow(t *testing.T) {
	t.Parallel()

	for _, width := range []int{30, 20, 10, 5, 1} {
		sts := fixtureStatuses()
		sts[0].Repo.Path = "acme/backend/platform-api"

		plain := ansi.Strip(renderTable(sts, width, Options{}, statusDeps(t, fakeGit{})))
		lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")

		if len(lines) != 4 {
			t.Fatalf("width %d: expected 4 lines, got %d:\n%s", width, len(lines), plain)
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("width %d: line %d is %d wide:\n%s", width, i, w, line)
			}
		}
		// Below this the gutter and padding alone consume the terminal.
		if width >= 10 && !strings.Contains(plain, "-api") {
			t.Errorf("width %d: no trace of the repository name:\n%s", width, plain)
		}
	}
}

// TestRenderTableNarrowDropOrder: the optional columns disappear in priority
// order — age first, then the commit hash, the counts and finally branch — so
// the set of columns a narrower terminal shows is always a subset of what a
// wider one shows, and a wide terminal shows them all.
func TestRenderTableNarrowDropOrder(t *testing.T) {
	t.Parallel()

	header := func(width int) string {
		sts := fixtureStatuses()
		sts[0].Repo.Path = "platform-api"
		sts[0].Head.Subject = strings.Repeat("very long commit subject ", 4)
		out := ansi.Strip(renderTable(sts, width, Options{}, statusDeps(t, fakeGit{})))
		return strings.Split(out, "\n")[0]
	}

	// Widest to narrowest, in the order the columns are shed.
	optional := []string{"Age", "Commit", "Upstream⇅", "Δ±", "Branch"}
	if got := header(120); !strings.Contains(got, "Age") {
		t.Errorf("a wide terminal should keep every column, got %q", got)
	}

	prev := 0 // how many optional columns the previous, wider table dropped
	for _, width := range []int{120, 90, 70, 60, 50, 40, 30} {
		got := header(width)
		dropped := 0
		for _, col := range optional {
			if !strings.Contains(got, col) {
				dropped++
			}
		}
		// The dropped columns are always a prefix of the drop order: the
		// first `dropped` optional columns are gone, the rest remain.
		for i, col := range optional {
			if missing := !strings.Contains(got, col); missing != (i < dropped) {
				t.Errorf("width %d: %q out of drop order in %q", width, col, got)
			}
		}
		if dropped < prev {
			t.Errorf("width %d dropped %d columns, wider table dropped %d: %q",
				width, dropped, prev, got)
		}
		prev = dropped
		// Repo, Status and Message are never optional.
		for _, want := range []string{"Repo", "Status", "Message"} {
			if !strings.Contains(got, want) {
				t.Errorf("width %d: header %q lost %q", width, got, want)
			}
		}
	}
}

// TestRenderTableRepoTruncatesFromTheLeft: repository paths share their
// leading components and differ at the end, so a squeezed Repo cell keeps its
// tail and marks the cut with a leading "…".
func TestRenderTableRepoTruncatesFromTheLeft(t *testing.T) {
	t.Parallel()

	sts := fixtureStatuses()
	sts[0].Repo.Path = "acme/backend/platform-api"

	out := renderTable(sts, 60, Options{}, statusDeps(t, fakeGit{}))
	plain := ansi.Strip(out)
	row := strings.Split(strings.TrimRight(plain, "\n"), "\n")[1]

	if !strings.Contains(row, "platform-api") {
		t.Errorf("Repo cell lost its distinguishing tail:\n%s", row)
	}
	if !strings.Contains(row, "…") {
		t.Errorf("expected a … marking the truncated head:\n%s", row)
	}
	if strings.Contains(row, "acme/backend") {
		t.Errorf("Repo cell should have dropped its leading components:\n%s", row)
	}
}

// TestTruncateHead: the left-truncation helper yields exactly the requested
// width, keeps the tail, and degrades sensibly when there is no room.
// ansi.TruncateLeft counts the cells to drop rather than the width to keep,
// and returns "" once asked to drop everything, so the boundaries matter.
func TestTruncateHead(t *testing.T) {
	t.Parallel()

	const s = "acme/platform-api" // 17 cells

	cases := []struct {
		w    int
		want string
	}{
		{w: 25, want: s},                  // wider than the text: untouched
		{w: 17, want: s},                  // exactly the text: untouched
		{w: 16, want: "…me/platform-api"}, // one cell over: one dropped, one marked
		{w: 6, want: "…m-api"},
		{w: 2, want: "…i"},
		{w: 1, want: "…"},
		{w: 0, want: ""},
		{w: -3, want: ""},
	}
	for _, c := range cases {
		got := truncateHead(s, c.w)
		if got != c.want {
			t.Errorf("truncateHead(%q, %d) = %q, want %q", s, c.w, got, c.want)
		}
		if c.w > 0 && lipgloss.Width(got) > c.w {
			t.Errorf("truncateHead(%q, %d) = %q, wider than %d", s, c.w, got, c.w)
		}
		if c.w > 1 && c.w < lipgloss.Width(s) && !strings.HasSuffix(got, "i") {
			t.Errorf("truncateHead(%q, %d) = %q, want the tail kept", s, c.w, got)
		}
	}
}

// TestRenderTableStatColumn: --stat adds the HEAD± column with green/red line
// counts between Status and Δ±; without it the column is absent entirely.
func TestRenderTableStatColumn(t *testing.T) {
	t.Parallel()

	sts := fixtureStatuses()
	sts[0].Stat = &git.DiffStat{Added: 27, Deleted: 8}
	sts[1].Stat = &git.DiffStat{Added: 4321}

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	icons := domain.Icons{DiffError: "✗✗"} // 2-cell error icon
	icons.ApplyDefaults()
	th := style.NewThemeDefault()
	sts := []*status.Report{
		fixture("a", domain.RepoStateOK, status.Report{}),
		fixture("b", domain.RepoStateOK, status.Report{
			Staged: 1, Err: errFixture,
		}),
		fixture("c", domain.RepoStateNotCloned, status.Report{Err: errFixture}),
		fixture("d", domain.RepoStateOK, status.Report{Ahead: 3}),
	}
	widths := newSlotWidths(icons)
	want := lipgloss.Width(statusSlots(sts[0], icons, th, widths))
	for i, st := range sts[1:] {
		if got := lipgloss.Width(statusSlots(st, icons, th, widths)); got != want {
			t.Errorf("row %d slot width = %d, want %d", i+1, got, want)
		}
	}
}
