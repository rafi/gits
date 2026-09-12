package status

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"golang.org/x/term"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/cli/walk"
	"github.com/rafi/gits/internal/types"
)

// Logical column indexes of the status table. HEAD± is only visible with
// --stat; rows are always built with every column and projected before render.
const (
	colGutter = iota
	colRepo
	colBranch
	colStatus
	colStat // HEAD± uncommitted line diffs (--stat only)
	colDelta
	colUpstream
	colVersion
	colCommit
	colAge
	colMessage
)

// renderGroups prints one compact table per project group to out and a
// summary footer to errW, returning every result error in stable tree order.
// Active filters (Dirty, Unsynced) drop non-matching rows — error rows stay
// visible — and projects left with no rows disappear entirely.
func renderGroups(
	out, errW io.Writer,
	groups []walk.GroupResult,
	withTitles bool,
	opts Options,
	deps types.RuntimeCLI,
) []error {
	termWidth := writerWidth(out)
	var (
		errs    []error
		all     []*repoStatus
		hidden  int
		printed int
	)
	for _, g := range groups {
		var sts []*repoStatus
		for _, res := range g.Results {
			if res == nil {
				continue // not started (cancelled before dequeue)
			}
			if res.Err != nil {
				errs = append(errs, res.Err)
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
			continue
		}
		if withTitles {
			if printed > 0 {
				fmt.Fprintln(out)
			}
			lipgloss.Fprintln(out, cli.ProjectTitleWithBullet(g.Project, deps.Theme))
		}
		printed++
		if len(sts) == 0 {
			continue
		}
		all = append(all, sts...)
		lipgloss.Fprintln(out, renderTable(sts, termWidth, opts, deps))
	}
	renderFooter(errW, all, hidden, deps.Theme)
	return errs
}

// renderTable renders one project's repositories as a borderless aligned
// table: bold header, gutter glyph, fixed status slots, right-aligned counts
// and dim metadata.
func renderTable(sts []*repoStatus, termWidth int, opts Options, deps types.RuntimeCLI) string {
	th, icons := deps.Theme, deps.Settings.Icons
	now := time.Now()
	widths := newSlotWidths(icons)

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
		rows[i] = []string{
			gutter(st, th),
			title,
			branch,
			statusSlots(st, icons, th, widths),
			"", // HEAD± filled below with per-table sub-widths
			"", // Δ± filled below
			"", // Upstream⇅ filled below
			dim(st.version),
			dim(st.commit),
			dim(shortAge(st.when, now)),
			message,
		}
	}
	fillCountColumns(rows, sts, icons, th)

	headers := []string{"", "Repo", "Branch", "Status", "HEAD±", "Δ±",
		"Upstream⇅", "Version", "Commit", "Age", "Message"}

	// Visible column set: HEAD± only participates with --stat. Headers and
	// rows are projected through it; styling maps back to logical indexes.
	visible := make([]int, 0, len(headers))
	for c := range headers {
		if c == colStat && !opts.Stat {
			continue
		}
		visible = append(visible, c)
	}
	pick := func(cells []string) []string {
		out := make([]string, len(visible))
		for i, c := range visible {
			out[i] = cells[c]
		}
		return out
	}
	shown := make([][]string, len(rows))
	for i, row := range rows {
		shown[i] = pick(row)
	}

	// Pin the glyph, count and branch columns at their natural width (style
	// Width marks a column fixed for the resizer): when the table is
	// width-capped, only the flexible text columns (Repo, Version, Message)
	// may contract. Sparse count columns would otherwise be shrunk first —
	// their median width is 0 — and collapse to "…".
	pinned := map[int]int{}
	for _, col := range []int{colGutter, colBranch, colStatus, colStat,
		colDelta, colUpstream, colCommit, colAge} {
		w := lipgloss.Width(headers[col])
		for _, row := range rows {
			w = max(w, lipgloss.Width(row[col]))
		}
		pad := 2
		if col == colGutter {
			pad = 3 // PaddingLeft(2) + PaddingRight(1)
		}
		pinned[col] = w + pad
	}

	t := table.New().
		Border(lipgloss.Border{}).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderHeader(false).BorderRow(false).
		Wrap(false).
		Headers(pick(headers)...).
		Rows(shown...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingRight(2)
			if col < 0 || col >= len(visible) {
				return s
			}
			switch visible[col] {
			case colGutter:
				s = s.PaddingLeft(2).PaddingRight(1)
			case colStat, colDelta, colUpstream:
				s = s.Align(lipgloss.Right)
			case colMessage:
				s = s.PaddingRight(0)
			}
			if w, ok := pinned[visible[col]]; ok {
				s = s.Width(w)
			}
			if row == table.HeaderRow {
				s = s.Inherit(th.StatusHeader)
			}
			return s
		})

	// Natural width first; when the table overflows the terminal, hand the
	// width cap to lipgloss, whose resizer contracts the widest columns and
	// …-truncates their cells (Wrap(false)) so rows never hard-wrap.
	rendered := t.String()
	if termWidth > 0 && lipgloss.Width(rendered) > termWidth {
		rendered = t.Width(termWidth).String()
	}
	return rendered
}

// gutter returns the leading row-kind glyph, mapped onto
// repository states.
func gutter(st *repoStatus, th config.Theme) string {
	switch st.repo.State {
	case domain.RepoStateOK:
		if st.err != nil {
			return th.Error.Render("✘")
		}
		return "+"
	case domain.RepoStateNoLocal:
		return th.StatusDim.Render("/")
	case domain.RepoStateRemote:
		return th.StatusDim.Render("|")
	case domain.RepoStateError:
		return th.Error.Render("✘")
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

// fillCountColumns renders the HEAD±, Δ± and Upstream⇅ cells with per-table
// sub-cell padding, so counts right-align on the ones digit across the group.
func fillCountColumns(
	rows [][]string,
	sts []*repoStatus,
	icons domain.Icons,
	th config.Theme,
) {
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
	for i := range rows {
		rows[i][colStat] = joinSubCells(
			padLeft(subs[i].add, wAdd), padLeft(subs[i].del, wDel))
		rows[i][colDelta] = joinSubCells(
			padLeft(subs[i].mod, wMod), padLeft(subs[i].unt, wUnt))
		rows[i][colUpstream] = joinSubCells(
			padLeft(subs[i].ahead, wAhead), padLeft(subs[i].behind, wBehind))
	}
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

	parts := []string{fmt.Sprintf("%d repo%s", repos, plural(repos))}
	if changed > 0 {
		parts = append(parts, fmt.Sprintf("%d with changes", changed))
	}
	if ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d ahead", ahead))
	}
	if errCount > 0 {
		parts = append(parts, fmt.Sprintf("%d error%s", errCount, plural(errCount)))
	}
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d hidden", hidden))
	}

	fmt.Fprintln(w)
	lipgloss.Fprintln(w, th.StatusFooter.Render(
		fmt.Sprintf("○ Showing %s", strings.Join(parts, ", "))))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// writerWidth returns the terminal width of w, or 0 when w is not a terminal.
func writerWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return cols
}
