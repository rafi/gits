package status

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/runtime/command"
	"github.com/rafi/gits/internal/service/status"
)

// renderTables prints one compact table per project as Result Output and, for
// multi-repository views, a summary footer as Diagnostic Output. It walks the
// tree the run visited — depth-first, a project's own repositories before its
// sub-projects — and looks each repository's row up; a project left with no
// rows prints nothing, and consecutive tables are separated by a blank line.
func renderTables(res command.Results[*status.Report], opts Options, deps app.RuntimeCLI) {
	out := deps.Out
	termWidth, _ := style.TermWidth(out)
	index := newRows(res)
	var (
		all     []*status.Report
		hidden  int
		printed int
	)
	var visit func(p domain.Project)
	visit = func(p domain.Project) {
		sts, skipped := index.visible(p, opts)
		hidden += skipped
		if len(sts) > 0 {
			if printed > 0 {
				fmt.Fprintln(out)
			}
			printed++
			all = append(all, sts...)
			lipgloss.Fprintln(out, renderTable(sts, termWidth, opts, deps))
		}
		for _, sub := range p.SubProjects {
			visit(sub)
		}
	}
	visit(res.Project)
	renderFooter(deps.Err, all, hidden, deps.Theme)
}

const (
	// Column padding. The gutter is the leading glyph column, which sits
	// tighter against the title than the ordinary cells do.
	cellPad        = 2
	gutterPadLeft  = 2
	gutterPadRight = 1

	// The thresholds compactCount abbreviates at, so every count fits two
	// cells: exact below a hundred, then C, K, and ∞.
	countHundred     = 100
	countThousand    = 1000
	countUncountable = 10000

	// The coarser units shortAge renders the age of a commit in.
	day   = 24 * time.Hour
	week  = 7 * day
	month = 30 * day
	year  = 365 * day

	// minTextCell is the narrowest the two text columns, Repo and Message,
	// may become. Below it a cell is a stub like "ac…" that names nothing,
	// so the table drops an optional column instead.
	minTextCell = 12

	// minTailCell is the stub a keepTail column keeps even when the terminal
	// is too narrow for the table: "…-api" still tells two rows apart.
	minTailCell = 5
)

// tableColumn describes one display-order column of the status table.
type tableColumn struct {
	title  string
	gutter bool // leading glyph column: PaddingLeft(2), PaddingRight(1)
	right  bool // right-aligned counts
	flex   bool // may contract when the table is width-capped
	bare   bool // no trailing padding (last column)
	// keepTail truncates from the left, keeping the end of the text. Paths
	// share their leading components and differ at the end, so "…/api" name
	// a repository where "acme/pl…" does not.
	keepTail bool
	// drop orders the optional columns a narrow terminal sheds, lowest
	// first; 0 means the column always renders.
	drop int
}

// text reports whether the column holds prose or a path: the columns that can
// give up width by truncating, rather than by being dropped entirely.
func (c tableColumn) text() bool { return c.flex || c.keepTail }

// columnPad returns the horizontal padding columnStyle gives a column, so
// every measurement agrees with what is rendered.
func columnPad(col tableColumn) int {
	switch {
	case col.gutter:
		return gutterPadLeft + gutterPadRight
	case col.bare:
		return 0
	default:
		return cellPad
	}
}

// The drop order of the optional columns, shed lowest first as the terminal
// narrows. Age and the commit hash go first: they are reference data, not
// state. The two count columns follow, because the Status glyphs already say
// that a row is dirty or diverged — the counts only quantify it. Branch is
// last. The gutter, Repo, Status and Message columns always render: they are
// what makes a row identifiable and actionable.
const (
	dropAge = iota + 1
	dropCommit
	dropUpstream
	dropDelta
	dropBranch
)

// renderTable renders one project's repositories as a borderless aligned
// table: bold header, gutter glyph, fixed status slots, right-aligned counts
// and dim metadata.
func renderTable(sts []*status.Report, termWidth int, opts Options, deps app.RuntimeCLI) string {
	th, icons := deps.Theme, deps.Settings.Icons
	now := time.Now()
	widths := newSlotWidths(icons)
	counts := buildCountCells(sts, icons, th)

	// Columns are declared directly in display order; the HEAD± column
	// participates only with --stat.
	cols := []tableColumn{
		{title: "", gutter: true},
		{title: "Repo", keepTail: true},
		{title: "Branch", drop: dropBranch},
		{title: "Status"},
	}
	if opts.Stat {
		cols = append(cols, tableColumn{title: "HEAD±", right: true})
	}
	cols = append(cols,
		tableColumn{title: "Δ±", right: true, drop: dropDelta},
		tableColumn{title: "Upstream⇅", right: true, drop: dropUpstream},
		tableColumn{title: "Commit", drop: dropCommit},
		tableColumn{title: "Age", drop: dropAge},
		tableColumn{title: "Message", flex: true, bare: true},
	)

	dim := func(s string) string {
		if s == "" {
			return ""
		}
		return th.StatusDim.Render(s)
	}
	rows := make([][]string, len(sts))
	for i, st := range sts {
		title, branch := st.Repo.Path, st.Branch
		if st.Repo.State != domain.RepoStateOK {
			title, branch = dim(title), dim(branch)
		}
		message := messageCell(st, th, dim)
		row := make([]string, 0, len(cols))
		row = append(row, gutter(st, icons, th), title, branch,
			statusSlots(st, icons, th, widths))
		if opts.Stat {
			row = append(row, counts.stat[i])
		}
		rows[i] = append(row,
			counts.delta[i],
			counts.upstream[i],
			dim(st.Head.Hash),
			dim(shortAge(st.Head.Time, now)),
			message,
		)
	}

	// A narrow terminal sheds optional columns and left-truncates Repo, so
	// Message is the only column lipgloss still has to contract.
	cols, rows, colWidths := fitColumns(cols, rows, termWidth)

	headers := make([]string, len(cols))
	for c, col := range cols {
		headers[c] = col.title
	}

	t := table.New().
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderHeader(false).BorderRow(false).
		Wrap(false).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(columnStyle(cols, colWidths, th))

	// When the natural width overflows the terminal, hand the width cap to
	// lipgloss, whose resizer contracts the widest flexible columns and
	// …-truncates their cells (Wrap(false)) so rows never hard-wrap.
	if termWidth > 0 && sum(colWidths) > termWidth {
		t = t.Width(termWidth)
	}
	return t.String()
}

// fitColumns sheds optional columns, lowest drop order first, until the text
// columns fit termWidth without shrinking past minTextCell, then truncates the
// keepTail columns to whatever the rest of the table leaves them. It returns
// the surviving columns, their rows and their natural widths.
//
// This leaves Message as the only flexible column, so lipgloss' resizer has a
// single column to contract. Left to itself it shares the shortfall between
// Repo and Message in proportion to their width, which spends most of it on
// Repo and renders rows named "ac…". An unknown width (0, not a terminal) or a
// table that already fits keeps every column whole.
func fitColumns(cols []tableColumn, rows [][]string, termWidth int) ([]tableColumn, [][]string, []int) {
	widths := make([]int, len(cols))
	for c := range cols {
		widths[c] = naturalWidth(cols[c], rows, c)
	}
	if termWidth <= 0 {
		return cols, rows, widths
	}
	for demand(cols, widths) > termWidth {
		// The lowest surviving drop order is the next column to shed.
		victim := -1
		for c, col := range cols {
			if col.drop > 0 && (victim < 0 || col.drop < cols[victim].drop) {
				victim = c
			}
		}
		if victim < 0 {
			break // nothing optional left; the text columns absorb the rest
		}
		cols = slices.Delete(cols, victim, victim+1)
		widths = slices.Delete(widths, victim, victim+1)
		for i, row := range rows {
			rows[i] = slices.Delete(row, victim, victim+1)
		}
	}

	// Trim the keepTail columns from the left to the width the other columns
	// leave them. Cutting the head keeps the end of a path visible, so a
	// squeezed cell still reads "…/gits".
	for c, col := range cols {
		if !col.keepTail {
			continue
		}
		rest := demand(cols, widths) - demandOf(col, widths[c])
		give := min(widths[c], termWidth-rest) - columnPad(col)
		// Even a terminal too narrow for the other columns keeps a stub of
		// the name: it is what tells one row from another.
		give = max(give, minTailCell)
		for _, row := range rows {
			row[c] = truncateHead(row[c], give)
		}
		widths[c] = naturalWidth(col, rows, c)
	}
	return cols, rows, widths
}

// demand returns the width the table needs to render: every column at its
// natural width, except the text columns, which can truncate to minTextCell.
func demand(cols []tableColumn, widths []int) int {
	total := 0
	for c, col := range cols {
		total += demandOf(col, widths[c])
	}
	return total
}

// demandOf returns the width a column of natural width w needs, padding
// included.
func demandOf(col tableColumn, w int) int {
	if col.text() {
		return min(w, minTextCell+columnPad(col))
	}
	return w
}

// naturalWidth returns the width column c needs to show every cell and its
// header whole, padding included.
func naturalWidth(col tableColumn, rows [][]string, c int) int {
	w := lipgloss.Width(col.title)
	for _, row := range rows {
		w = max(w, lipgloss.Width(row[c]))
	}
	return w + columnPad(col)
}

// sum adds up the widths.
func sum(widths []int) int {
	total := 0
	for _, w := range widths {
		total += w
	}
	return total
}

// truncateHead truncates s from the left to exactly w cells, marking the cut
// with a leading "…". ansi.TruncateLeft takes the number of cells to drop, and
// returns nothing at all once asked to drop the whole string, so the count is
// derived from the width wanted and the degenerate widths are handled here.
func truncateHead(s string, w int) string {
	switch width := lipgloss.Width(s); {
	case w <= 0:
		return ""
	case width <= w:
		return s
	case w == 1:
		// Room for the marker alone: the name cannot be shown at all.
		return "…"
	default:
		// Dropping `width-w+1` cells leaves w-1, and the marker fills w.
		return ansi.TruncateLeft(s, width-w+1, "…")
	}
}

// columnStyle returns the table's per-cell style function: alignment and
// padding from the column's declaration, its natural width unless it is
// flexible, and the header style on the header row.
//
// A style Width marks a column fixed for lipgloss' resizer, which shrinks only
// the columns it is free to shrink. Every column but Message is pinned: Repo
// has already been truncated to fit, and the sparse count columns would
// otherwise be shrunk first — their median width is 0 — and collapse to "…".
// That leaves Message, the one column that can always say less, to absorb the
// remaining overflow.
func columnStyle(cols []tableColumn, widths []int, th style.Theme) func(row, c int) lipgloss.Style {
	return func(row, c int) lipgloss.Style {
		s := lipgloss.NewStyle().PaddingRight(cellPad)
		if c < 0 || c >= len(cols) {
			return s
		}
		col := cols[c]
		switch {
		case col.gutter:
			s = s.PaddingLeft(gutterPadLeft).PaddingRight(gutterPadRight)
		case col.right:
			s = s.Align(lipgloss.Right)
		case col.bare:
			s = s.PaddingRight(0)
		}
		if !col.flex {
			s = s.Width(widths[c])
		}
		if row == table.HeaderRow {
			s = s.Inherit(th.StatusHeader)
		}
		return s
	}
}

// messageCell renders the last column: the last commit's subject, or the
// bare failure reason in its place. Real failures show in red; benign non-OK
// states (not cloned, remote-only) and ordinary commit subjects stay dim.
func messageCell(st *status.Report, th style.Theme, dim func(string) string) string {
	switch {
	case st.Err == nil:
		return dim(st.Head.Subject)
	case st.Repo.State == domain.RepoStateOK, st.Repo.State == domain.RepoStateError:
		return th.Error.Render(st.Err.Error())
	default:
		return dim(st.Err.Error())
	}
}

// gutter returns the leading row-kind glyph, mapped onto
// repository states.
func gutter(st *status.Report, icons domain.Icons, th style.Theme) string {
	switch st.Repo.State {
	case domain.RepoStateOK:
		if st.Err != nil {
			return th.Error.Render(icons.DiffError)
		}
		return "+"
	case domain.RepoStateNotCloned:
		return th.StatusDim.Render("/")
	case domain.RepoStateRemoteOnly:
		return th.StatusDim.Render(icons.DiffClean)
	case domain.RepoStateError:
		return th.Error.Render(icons.DiffError)
	case domain.RepoStateUnknown:
		// Unclassified: the glyph says so rather than claiming a state.
		fallthrough
	default:
		return th.StatusDim.Render("?")
	}
}

// slotWidths carries the per-run constant slot widths of the status symbol
// column, derived once per table from the configured icons instead of being
// remeasured on every row.
type slotWidths struct {
	fourth   int // widest of the error/N-A glyphs
	upstream int // widest upstream glyph
}

// newSlotWidths measures the configured icons once for a whole table.
func newSlotWidths(icons domain.Icons) slotWidths {
	w := slotWidths{
		fourth: max(lipgloss.Width(icons.DiffError), lipgloss.Width(icons.NA), 1),
	}
	for _, icon := range []string{
		icons.DiffClean, icons.NA, icons.Gone, icons.Diverged, icons.Ahead, icons.Behind,
	} {
		w.upstream = max(w.upstream, lipgloss.Width(icon))
	}
	return w
}

// statusSlots renders the fixed-width status symbol column. Every row has the
// same five slots, absent symbols render as spaces so glyphs align vertically:
//
//	1 staged  2 unstaged  3 untracked  4 error/not-cloned  5 upstream
func statusSlots(st *status.Report, icons domain.Icons, th style.Theme, widths slotWidths) string {
	// slot renders icon in style, right-filled to width; an empty icon is a
	// blank slot.
	slot := func(icon string, style lipgloss.Style, width int) string {
		if icon == "" {
			return strings.Repeat(" ", width)
		}
		return style.Render(icon) + strings.Repeat(" ", width-lipgloss.Width(icon))
	}
	// flag renders a change glyph in its own width, or blanks it.
	flag := func(active bool, icon string) string {
		if !active {
			return strings.Repeat(" ", lipgloss.Width(icon))
		}
		return th.StatusFlag.Render(icon)
	}

	fourthIcon, fourthStyle := "", th.StatusDim
	switch {
	case st.Err != nil && st.Repo.State != domain.RepoStateNotCloned &&
		st.Repo.State != domain.RepoStateRemoteOnly:
		fourthIcon, fourthStyle = icons.DiffError, th.Error
	case st.Repo.State != domain.RepoStateOK:
		fourthIcon = icons.NA
	}

	upstreamIcon := ""
	switch broken := st.Repo.State != domain.RepoStateOK || st.Err != nil; {
	case broken:
	case st.GoneUpstream():
		// Ahead of the divergence glyphs: a Gone Upstream is what the row is
		// about, and the counts — measured against a fallback ref, when one
		// was found — still show in the Upstream⇅ column.
		upstreamIcon = icons.Gone
	case !st.Compared:
		upstreamIcon = icons.NA
	case st.Ahead > 0 && st.Behind > 0:
		upstreamIcon = icons.Diverged
	case st.Ahead > 0:
		upstreamIcon = icons.Ahead
	case st.Behind > 0:
		upstreamIcon = icons.Behind
	default:
		upstreamIcon = icons.DiffClean
	}

	return flag(st.Staged > 0, icons.Staged) +
		flag(st.Unstaged > 0, icons.Unstaged) +
		flag(st.Untracked > 0, icons.Untracked) +
		slot(fourthIcon, fourthStyle, widths.fourth) +
		slot(upstreamIcon, th.StatusDim, widths.upstream)
}

// countCells holds the HEAD±, Δ± and Upstream⇅ cell text per row.
type countCells struct {
	stat     []string
	delta    []string
	upstream []string
}

// countPair is the two sub-cells of one count column, before padding.
type countPair [2]string

// buildCountCells renders the HEAD±, Δ± and Upstream⇅ cells with per-table
// sub-cell padding, so counts right-align on the ones digit across the group.
func buildCountCells(sts []*status.Report, icons domain.Icons, th style.Theme) countCells {
	count := func(n int, prefix string, s lipgloss.Style) string {
		if n <= 0 {
			return ""
		}
		return s.Render(prefix + compactCount(n))
	}
	stat := make([]countPair, len(sts))
	delta := make([]countPair, len(sts))
	upstream := make([]countPair, len(sts))
	for i, st := range sts {
		if st.Err != nil || st.Repo.State != domain.RepoStateOK {
			continue
		}
		if st.Stat != nil {
			stat[i] = countPair{
				count(st.Stat.Added, "+", th.StatusAdded),
				count(st.Stat.Deleted, "-", th.StatusDeleted),
			}
		}
		delta[i] = countPair{
			count(st.Staged+st.Unstaged, icons.Modified, th.StatusFlag),
			count(st.Untracked, icons.Untracked, th.StatusFlag),
		}
		upstream[i] = countPair{
			count(st.Ahead, icons.Ahead, th.StatusAhead),
			count(st.Behind, icons.Behind, th.StatusBehind),
		}
	}
	return countCells{
		stat:     joinPairs(stat),
		delta:    joinPairs(delta),
		upstream: joinPairs(upstream),
	}
}

// joinPairs pads each sub-cell to the widest in its position and joins the
// two with a single-space gap. A row with neither collapses to empty so the
// column can shrink away.
func joinPairs(pairs []countPair) []string {
	var widths [2]int
	for _, p := range pairs {
		for k, s := range p {
			widths[k] = max(widths[k], lipgloss.Width(s))
		}
	}
	cells := make([]string, len(pairs))
	for i, p := range pairs {
		joined := strings.TrimRight(padLeft(p[0], widths[0])+" "+padLeft(p[1], widths[1]), " ")
		if strings.TrimSpace(joined) != "" {
			cells[i] = joined
		}
	}
	return cells
}

// padLeft pads s with leading spaces to the ANSI-aware width w.
func padLeft(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return strings.Repeat(" ", gap) + s
	}
	return s
}

// compactCount keeps counts to two cells: exact under 100, then compact
// hundreds (632 → 6C), thousands (4567 → 4K) and ∞ from 10000 up.
func compactCount(n int) string {
	switch {
	case n >= countUncountable:
		return "∞"
	case n >= countThousand:
		return strconv.Itoa(n/countThousand) + "K"
	case n >= countHundred:
		return strconv.Itoa(n/countHundred) + "C"
	default:
		return strconv.Itoa(n)
	}
}

// shortAge formats the duration since t in a short relative form.
func shortAge(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < day:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	case d < week:
		return strconv.Itoa(int(d/day)) + "d"
	case d < month:
		return strconv.Itoa(int(d/week)) + "w"
	case d < year:
		return strconv.Itoa(int(d/month)) + "mo"
	default:
		return strconv.Itoa(int(d/year)) + "y"
	}
}

// renderFooter prints the dim `○ Showing …` summary footer as Diagnostic
// Output, after a blank separator.
func renderFooter(w io.Writer, sts []*status.Report, hidden int, th style.Theme) {
	repos := len(sts)
	if repos == 1 && hidden == 0 {
		return
	}
	changed, ahead, errCount := 0, 0, 0
	for _, st := range sts {
		if st.Changed() {
			changed++
		}
		if st.Ahead > 0 {
			ahead++
		}
		if st.Err != nil {
			errCount++
		}
	}

	parts := []string{fmt.Sprintf("%d repo%s", repos, style.Plural(repos))}
	if changed > 0 {
		parts = append(parts, fmt.Sprintf("%d with changes", changed))
	}
	if ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d ahead", ahead))
	}
	if errCount > 0 {
		parts = append(parts, fmt.Sprintf("%d error%s", errCount, style.Plural(errCount)))
	}
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d hidden", hidden))
	}

	fmt.Fprintln(w)
	lipgloss.Fprintln(w, th.StatusFooter.Render(
		fmt.Sprintf("○ Showing %s", strings.Join(parts, ", "))))
}
