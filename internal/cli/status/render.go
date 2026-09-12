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
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/cli/walk"
	"github.com/rafi/gits/internal/types"
)

// renderGroups prints one compact table per project group to out and a
// summary footer to errW, returning every result error in stable tree order.
// The traversal (nil slots, error collection, titles, separators) is walk's;
// this body filters rows — active filters (Dirty, Unsynced) drop non-matching
// rows, error rows stay visible — and projects left with no rows disappear
// entirely.
func renderGroups(
	out, errW io.Writer,
	groups []walk.GroupResult,
	withTitles bool,
	opts Options,
	deps types.RuntimeCLI,
) []error {
	termWidth, _ := cli.TermWidth(out)
	var (
		all    []*repoStatus
		hidden int
	)
	errs := walk.RenderGroups(out, groups, deps, withTitles,
		func(g walk.GroupResult) (string, bool) {
			var sts []*repoStatus
			for _, res := range g.Results {
				if res == nil {
					continue
				}
				st, ok := res.Payload.(*repoStatus)
				if !ok {
					continue
				}
				if opts.filtered() && !opts.keep(st) {
					hidden++
					continue
				}
				sts = append(sts, st)
			}
			if opts.filtered() && len(sts) == 0 {
				return "", false
			}
			if len(sts) == 0 {
				return "", true
			}
			all = append(all, sts...)
			return renderTable(sts, termWidth, opts, deps), true
		})
	renderFooter(errW, all, hidden, deps.Theme)
	return errs
}

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
		title, branch := st.title, st.branch
		if st.repo.State != domain.RepoStateOK {
			title, branch = dim(title), dim(branch)
		}
		message := st.message
		// Real failures show their reason in red; benign non-OK states
		// (not cloned, remote-only) and ordinary commit subjects stay dim.
		if st.err != nil && (st.repo.State == domain.RepoStateOK ||
			st.repo.State == domain.RepoStateError) {
			message = th.Error.Render(message)
		} else {
			message = dim(message)
		}
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
			dim(st.commit),
			dim(shortAge(st.when, now)),
			message,
		)
	}

	// Measure every column once. Non-flex columns are pinned at their natural
	// width (style Width marks a column fixed for the resizer): when the
	// table is width-capped, only the flexible text columns (Repo, Version,
	// Message) may contract. Sparse count columns would otherwise be shrunk
	// first — their median width is 0 — and collapse to "…". The summed
	// widths also give the natural table width, so the cap decision happens
	// before the single render.
	pinned := map[int]int{}
	natural := 0
	for c, col := range cols {
		w := lipgloss.Width(col.title)
		for _, row := range rows {
			w = max(w, lipgloss.Width(row[c]))
		}
		pad := 2
		switch {
		case col.gutter:
			pad = 3 // PaddingLeft(2) + PaddingRight(1)
		case col.bare:
			pad = 0
		}
		natural += w + pad
		if !col.flex {
			pinned[c] = w + pad
		}
	}

	t := table.New().
		Border(lipgloss.Border{}).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderHeader(false).BorderRow(false).
		Wrap(false).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, c int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingRight(2)
			if c < 0 || c >= len(cols) {
				return s
			}
			col := cols[c]
			switch {
			case col.gutter:
				s = s.PaddingLeft(2).PaddingRight(1)
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
		})

	// When the natural width overflows the terminal, hand the width cap to
	// lipgloss, whose resizer contracts the widest flexible columns and
	// …-truncates their cells (Wrap(false)) so rows never hard-wrap.
	if termWidth > 0 && natural > termWidth {
		t = t.Width(termWidth)
	}
	return t.String()
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
	case domain.RepoStateNoLocal:
		return th.StatusDim.Render("/")
	case domain.RepoStateRemote:
		return th.StatusDim.Render(icons.DiffClean)
	case domain.RepoStateError:
		return th.Error.Render(icons.DiffError)
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
		icons.DiffClean, icons.NA, icons.Diverged, icons.Ahead, icons.Behind,
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
	case st.noUpstream:
		upstreamIcon = icons.NA
	case st.ahead > 0 && st.behind > 0:
		upstreamIcon = icons.Diverged
	case st.ahead > 0:
		upstreamIcon = icons.Ahead
	case st.behind > 0:
		upstreamIcon = icons.Behind
	}

	broken := st.repo.State != domain.RepoStateOK || st.err != nil
	fourth := pad(" ", " ", widths.fourth)
	switch {
	case st.err != nil && st.repo.State != domain.RepoStateNoLocal &&
		st.repo.State != domain.RepoStateRemote:
		fourth = pad(th.Error.Render(icons.DiffError), icons.DiffError, widths.fourth)
	case st.repo.State != domain.RepoStateOK:
		fourth = pad(th.StatusDim.Render(icons.NA), icons.NA, widths.fourth)
	}

	upstream := strings.Repeat(" ", widths.upstream)
	if !broken {
		upstream = pad(th.StatusDim.Render(upstreamIcon), upstreamIcon, widths.upstream)
	}

	return slot(st.staged > 0, icons.Staged, th.StatusFlag) +
		slot(st.unstaged > 0, icons.Unstaged, th.StatusFlag) +
		slot(st.untracked > 0, icons.Untracked, th.StatusFlag) +
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
		if st.added > 0 {
			subs[i].add = th.StatusAdded.Render("+" + compactCount(st.added))
		}
		if st.deleted > 0 {
			subs[i].del = th.StatusDeleted.Render("-" + compactCount(st.deleted))
		}
		if n := st.staged + st.unstaged; n > 0 {
			subs[i].mod = th.StatusFlag.Render(icons.Modified + compactCount(n))
		}
		if st.untracked > 0 {
			subs[i].unt = th.StatusFlag.Render(icons.Untracked + compactCount(st.untracked))
		}
		if st.ahead > 0 {
			subs[i].ahead = th.StatusAhead.Render(icons.Ahead + compactCount(st.ahead))
		}
		if st.behind > 0 {
			subs[i].behind = th.StatusBehind.Render(icons.Behind + compactCount(st.behind))
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
	case n >= 10000:
		return "∞"
	case n >= 1000:
		return strconv.Itoa(n/1000) + "K"
	case n >= 100:
		return strconv.Itoa(n/100) + "C"
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
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

// renderFooter prints the dim `○ Showing …` summary footer to stderr
// after a blank separator.
func renderFooter(w io.Writer, sts []*repoStatus, hidden int, th config.Theme) {
	repos := len(sts)
	changed, ahead, errCount := 0, 0, 0
	for _, st := range sts {
		if st.changed() {
			changed++
		}
		if st.ahead > 0 {
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
