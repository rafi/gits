package status

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
)

// renderTables prints one compact table per project as Result Output and a
// summary footer as Diagnostic Output. It walks the tree the run visited —
// depth-first, a project's own repositories before its sub-projects — and
// looks each repository's row up; a project left with no rows prints
// nothing, and consecutive tables are separated by a blank line.
func renderTables(res bulk.Results[*repoStatus], opts Options, deps types.RuntimeCLI) {
	out := deps.Out
	termWidth, _ := cli.TermWidth(out)
	index := newRows(res)
	var (
		all     []*repoStatus
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

	// hoursPerDay converts the age of a commit into the coarser units
	// shortAge renders it in.
	hoursPerDay  = 24
	daysPerWeek  = 7
	daysPerMonth = 30
	daysPerYear  = 365
)

// tableColumn describes one display-order column of the status table.
type tableColumn struct {
	title  string
	gutter bool // leading glyph column: PaddingLeft(2), PaddingRight(1)
	right  bool // right-aligned counts
	flex   bool // may contract when the table is width-capped
	bare   bool // no trailing padding (last column)
}

// renderTable renders one project's repositories as a borderless aligned
// table: bold header, gutter glyph, fixed status slots, right-aligned counts
// and dim metadata.
func renderTable(sts []*repoStatus, termWidth int, opts Options, deps types.RuntimeCLI) string {
	th, icons := deps.Theme, deps.Settings.Icons
	now := time.Now()
	widths := newSlotWidths(icons)
	counts := buildCountCells(sts, icons, th)

	// Columns are declared directly in display order; the HEAD± column
	// participates only with --stat.
	cols := []tableColumn{
		{title: "", gutter: true},
		{title: "Repo", flex: true},
		{title: "Branch"},
		{title: "Status"},
	}
	if opts.Stat {
		cols = append(cols, tableColumn{title: "HEAD±", right: true})
	}
	cols = append(cols,
		tableColumn{title: "Δ±", right: true},
		tableColumn{title: "Upstream⇅", right: true},
		tableColumn{title: "Version", flex: true},
		tableColumn{title: "Commit"},
		tableColumn{title: "Age"},
		tableColumn{title: "Message", flex: true, bare: true},
	)
	headers := make([]string, len(cols))
	for c, col := range cols {
		headers[c] = col.title
	}

	rows := make([][]string, len(sts))
	for i, st := range sts {
		dim := func(s string) string {
			if s == "" {
				return ""
			}
			return th.StatusDim.Render(s)
		}
		title, branch := st.repo.Path, st.Branch
		if st.repo.State != domain.RepoStateOK {
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
			dim(st.version),
			dim(st.head.Hash),
			dim(shortAge(st.head.Time, now)),
			message,
		)
	}

	pinned, natural := measureColumns(cols, rows)

	t := table.New().
		Border(lipgloss.Border{}).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderHeader(false).BorderRow(false).
		Wrap(false).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(columnStyle(cols, pinned, th))

	// When the natural width overflows the terminal, hand the width cap to
	// lipgloss, whose resizer contracts the widest flexible columns and
	// …-truncates their cells (Wrap(false)) so rows never hard-wrap.
	if termWidth > 0 && natural > termWidth {
		t = t.Width(termWidth)
	}
	return t.String()
}

// measureColumns measures every column once, returning the pinned widths
// (style Width marks a column fixed for the resizer) and the table's natural
// width. Only the flexible text columns (Repo, Version, Message) stay
// unpinned and may contract when the table is width-capped; sparse count
// columns would otherwise be shrunk first — their median width is 0 — and
// collapse to "…". The natural width is summed here so the cap decision
// happens before the single render.
func measureColumns(cols []tableColumn, rows [][]string) (map[int]int, int) {
	pinned := map[int]int{}
	natural := 0
	for c, col := range cols {
		w := lipgloss.Width(col.title)
		for _, row := range rows {
			w = max(w, lipgloss.Width(row[c]))
		}
		pad := cellPad
		switch {
		case col.gutter:
			pad = gutterPadLeft + gutterPadRight
		case col.bare:
			pad = 0
		}
		natural += w + pad
		if !col.flex {
			pinned[c] = w + pad
		}
	}
	return pinned, natural
}

// columnStyle returns the table's per-cell style function: alignment and
// padding from the column's declaration, its measured width when pinned, and
// the header style on the header row.
func columnStyle(cols []tableColumn, pinned map[int]int, th config.Theme) func(row, c int) lipgloss.Style {
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
		if w, ok := pinned[c]; ok {
			s = s.Width(w)
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
func messageCell(st *repoStatus, th config.Theme, dim func(string) string) string {
	switch {
	case st.err == nil:
		return dim(st.head.Subject)
	case st.repo.State == domain.RepoStateOK, st.repo.State == domain.RepoStateError:
		return th.Error.Render(st.err.Error())
	default:
		return dim(st.err.Error())
	}
}

// gutter returns the leading row-kind glyph, mapped onto
// repository states.
func gutter(st *repoStatus, icons domain.Icons, th config.Theme) string {
	switch st.repo.State {
	case domain.RepoStateOK:
		if st.err != nil {
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
func statusSlots(st *repoStatus, icons domain.Icons, th config.Theme, widths slotWidths) string {
	slot := func(active bool, icon string, style lipgloss.Style) string {
		if !active {
			return strings.Repeat(" ", lipgloss.Width(icon))
		}
		return style.Render(icon)
	}
	// pad right-fills a rendered glyph to its slot's fixed width.
	pad := func(rendered, icon string, width int) string {
		return rendered + strings.Repeat(" ", width-lipgloss.Width(icon))
	}

	upstreamIcon := icons.DiffClean
	switch {
	case st.GoneUpstream():
		// Ahead of the divergence glyphs: a Gone Upstream is what the row is
		// about, and the counts — measured against a fallback ref, when one
		// was found — still show in the Upstream⇅ column.
		upstreamIcon = icons.Gone
	case !st.compared:
		upstreamIcon = icons.NA
	case st.Ahead > 0 && st.Behind > 0:
		upstreamIcon = icons.Diverged
	case st.Ahead > 0:
		upstreamIcon = icons.Ahead
	case st.Behind > 0:
		upstreamIcon = icons.Behind
	}

	broken := st.repo.State != domain.RepoStateOK || st.err != nil
	fourth := pad(" ", " ", widths.fourth)
	switch {
	case st.err != nil && st.repo.State != domain.RepoStateNotCloned &&
		st.repo.State != domain.RepoStateRemoteOnly:
		fourth = pad(th.Error.Render(icons.DiffError), icons.DiffError, widths.fourth)
	case st.repo.State != domain.RepoStateOK:
		fourth = pad(th.StatusDim.Render(icons.NA), icons.NA, widths.fourth)
	}

	upstream := strings.Repeat(" ", widths.upstream)
	if !broken {
		upstream = pad(th.StatusDim.Render(upstreamIcon), upstreamIcon, widths.upstream)
	}

	return slot(st.Staged > 0, icons.Staged, th.StatusFlag) +
		slot(st.Unstaged > 0, icons.Unstaged, th.StatusFlag) +
		slot(st.Untracked > 0, icons.Untracked, th.StatusFlag) +
		fourth +
		upstream
}

// countCells holds the HEAD±, Δ± and Upstream⇅ cell text per row.
type countCells struct {
	stat     []string
	delta    []string
	upstream []string
}

// buildCountCells renders the HEAD±, Δ± and Upstream⇅ cells with per-table
// sub-cell padding, so counts right-align on the ones digit across the group.
func buildCountCells(
	sts []*repoStatus,
	icons domain.Icons,
	th config.Theme,
) countCells {
	type sub struct{ add, del, mod, unt, ahead, behind string }
	subs := make([]sub, len(sts))
	var wAdd, wDel, wMod, wUnt, wAhead, wBehind int
	for i, st := range sts {
		if st.err != nil || st.repo.State != domain.RepoStateOK {
			continue
		}
		if st.stat != nil && st.stat.Added > 0 {
			subs[i].add = th.StatusAdded.Render("+" + compactCount(st.stat.Added))
		}
		if st.stat != nil && st.stat.Deleted > 0 {
			subs[i].del = th.StatusDeleted.Render("-" + compactCount(st.stat.Deleted))
		}
		if n := st.Staged + st.Unstaged; n > 0 {
			subs[i].mod = th.StatusFlag.Render(icons.Modified + compactCount(n))
		}
		if st.Untracked > 0 {
			subs[i].unt = th.StatusFlag.Render(icons.Untracked + compactCount(st.Untracked))
		}
		if st.Ahead > 0 {
			subs[i].ahead = th.StatusAhead.Render(icons.Ahead + compactCount(st.Ahead))
		}
		if st.Behind > 0 {
			subs[i].behind = th.StatusBehind.Render(icons.Behind + compactCount(st.Behind))
		}
		wAdd = max(wAdd, lipgloss.Width(subs[i].add))
		wDel = max(wDel, lipgloss.Width(subs[i].del))
		wMod = max(wMod, lipgloss.Width(subs[i].mod))
		wUnt = max(wUnt, lipgloss.Width(subs[i].unt))
		wAhead = max(wAhead, lipgloss.Width(subs[i].ahead))
		wBehind = max(wBehind, lipgloss.Width(subs[i].behind))
	}
	cells := countCells{
		stat:     make([]string, len(sts)),
		delta:    make([]string, len(sts)),
		upstream: make([]string, len(sts)),
	}
	for i := range sts {
		cells.stat[i] = joinSubCells(
			padLeft(subs[i].add, wAdd), padLeft(subs[i].del, wDel))
		cells.delta[i] = joinSubCells(
			padLeft(subs[i].mod, wMod), padLeft(subs[i].unt, wUnt))
		cells.upstream[i] = joinSubCells(
			padLeft(subs[i].ahead, wAhead), padLeft(subs[i].behind, wBehind))
	}
	return cells
}

// joinSubCells joins two padded sub-cells with a single-space gap, collapsing
// to empty when both are blank so the column can shrink away.
func joinSubCells(a, b string) string {
	joined := strings.TrimRight(a+" "+b, " ")
	if strings.TrimSpace(joined) == "" {
		return ""
	}
	return joined
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
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < hoursPerDay*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < daysPerWeek*hoursPerDay*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/hoursPerDay))
	case d < daysPerMonth*hoursPerDay*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(hoursPerDay*daysPerWeek)))
	case d < daysPerYear*hoursPerDay*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(hoursPerDay*daysPerMonth)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(hoursPerDay*daysPerYear)))
	}
}

// renderFooter prints the dim `○ Showing …` summary footer as Diagnostic
// Output, after a blank separator.
func renderFooter(w io.Writer, sts []*repoStatus, hidden int, th config.Theme) {
	repos := len(sts)
	changed, ahead, errCount := 0, 0, 0
	for _, st := range sts {
		if st.changed() {
			changed++
		}
		if st.Ahead > 0 {
			ahead++
		}
		if st.err != nil {
			errCount++
		}
	}

	parts := []string{fmt.Sprintf("%d repo%s", repos, cli.Plural(repos))}
	if changed > 0 {
		parts = append(parts, fmt.Sprintf("%d with changes", changed))
	}
	if ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d ahead", ahead))
	}
	if errCount > 0 {
		parts = append(parts, fmt.Sprintf("%d error%s", errCount, cli.Plural(errCount)))
	}
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d hidden", hidden))
	}

	fmt.Fprintln(w)
	lipgloss.Fprintln(w, th.StatusFooter.Render(
		fmt.Sprintf("○ Showing %s", strings.Join(parts, ", "))))
}
