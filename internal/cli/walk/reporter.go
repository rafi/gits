package walk

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"charm.land/huh/v2/spinner"
	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/internal/cli"
)

// spinnerFrames are huh's "Meter" spinner animation frames, reused so each
// active repo row shows the same spinner huh renders. huh advances them at
// 10fps, matching tickInterval.
var spinnerFrames = spinner.Meter.Frames

// huhSpinnerColor is the spinner color from huh's default theme (ThemeDefault),
// applied to each repo row's frame so it matches the library's look.
const huhSpinnerColor = "#F780E2"

const (
	// nameWidth is the default width of the leading repo-name column so every
	// bar starts at the same screen column. On a narrow terminal the column is
	// shrunk to fit (see layoutFor). The width is measured on the plain text
	// (before lipgloss color) so escape sequences never skew it.
	nameWidth = 24
	// barWidth is the number of cells inside the [..........] bar.
	barWidth = 14
	// tickInterval is how often the live block is redrawn.
	tickInterval = 100 * time.Millisecond

	// ANSI cursor controls for the in-place redraw: CursorUp moves the cursor up
	// one line, EraseLine clears the line it sits on. Kept local so the package
	// carries no progress-rendering dependency.
	ansiCursorUp  = "\x1b[A"
	ansiEraseLine = "\x1b[K"
)

// fitName pads s with spaces or truncates it with a trailing ellipsis so its
// display width is exactly w runes.
func fitName(s string, w int) string {
	if w < 1 {
		w = 1
	}
	r := []rune(s)
	if len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

// layoutFor adapts a row's layout to the terminal width: the name column is
// shrunk and the trailing stats dropped so a row never exceeds the terminal and
// wraps, which would corrupt the cursor-up rewind. A non-positive width means
// the size is unknown (tests, pipes), so the full layout is used.
func layoutFor(width int) (nameW int, showStats bool) {
	const core = len(" ... ") + 7 + 1 + barWidth + 2 // separators + pct cell + bar
	const statsRoom = 13                             // room for a trailing " [N in Ts]"
	switch {
	case width <= 0, width >= nameWidth+core+statsRoom:
		return nameWidth, true
	case width >= nameWidth+core:
		return nameWidth, false
	default:
		if nameW = width - core; nameW < 1 {
			nameW = 1
		}
		return nameW, false
	}
}

// RepoTracker is one repo's live tracker. The walker calls a completion marker
// when the repo finishes; the row spins (an indeterminate huh-style spinner)
// until then, so there is nothing to drive between start and completion.
type RepoTracker interface {
	MarkDone()
	MarkErrored()
}

// Reporter renders live progress for a walk: one in-place row per active repo
// plus an overall total pinned at the bottom. Implementations must tolerate the
// full lifecycle being driven concurrently from worker goroutines: Start once,
// RepoStart per repo (its row driven and marked on that worker — MarkErrored
// folds into the failed-count), Done per completed repo, and a final Stop that
// erases the live block and blocks until the render goroutine has drained
// (AC-7) so results can be flushed to stdout without interleaving.
type Reporter interface {
	Start(verb string, total int)       // begin; record the totals
	RepoStart(label string) RepoTracker // add a live row for one repo
	Done()                              // overall: one repo finished
	Stop()                              // erase the block, drain the render goroutine
}

// NewReporter selects a progress reporter based on whether w is a terminal.
// Non-TTY writers (pipes, CI logs) get a no-op reporter so ANSI cursor controls
// never garble captured output (AC-8).
func NewReporter(w io.Writer) Reporter {
	if _, isTTY := cli.TermWidth(w); isTTY {
		return newLiveReporter(w)
	}
	return &nopReporter{}
}

// nopReporter is the no-op reporter used for non-TTY writers and tests.
type nopReporter struct{}

func (*nopReporter) Start(string, int)            {}
func (*nopReporter) RepoStart(string) RepoTracker { return nopRepoTracker{} }
func (*nopReporter) Done()                        {}
func (*nopReporter) Stop()                        {}

// nopRepoTracker is the no-op per-repo tracker.
type nopRepoTracker struct{}

func (nopRepoTracker) MarkDone()    {}
func (nopRepoTracker) MarkErrored() {}

// styles holds the lipgloss styles for one render. Colors are emitted as-is and
// downsampled to the terminal's profile at the write site (see render).
type styles struct {
	name    lipgloss.Style
	overall lipgloss.Style
	percent lipgloss.Style
	fill    lipgloss.Style
	empty   lipgloss.Style
	stats   lipgloss.Style
	spinner lipgloss.Style // huh-themed color for the per-repo spinner frame
	clamp   lipgloss.Style // width-limited base used to truncate a row to the TTY
}

func newStyles() styles {
	return styles{
		name:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		overall: lipgloss.NewStyle().Bold(true),
		percent: lipgloss.NewStyle().Foreground(lipgloss.Color("14")),
		fill:    lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		empty:   lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		stats:   lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		spinner: lipgloss.NewStyle().Foreground(lipgloss.Color(huhSpinnerColor)),
		clamp:   lipgloss.NewStyle(),
	}
}

// liveRow is the mutable state of one active repo's row. Each row renders as a
// huh-style spinner plus the repo name; progress within a repo is conveyed by
// the spinner animation rather than a per-repo bar, so only the name is held.
type liveRow struct {
	name string
}

// liveReporter renders the live block itself rather than delegating to a tracker
// library: a row for each currently-active repo followed by an overall row,
// redrawn in place each tick. Because only active rows are drawn, finished repos
// leave no idle/blank row behind, and the block shrinks cleanly (each tick
// erases exactly what the previous tick drew). The block is ephemeral; the
// authoritative output is the tree printed afterwards.
type liveReporter struct {
	w     io.Writer
	st    styles
	width int // terminal width, 0 if unknown; rows are clamped to it

	mu      sync.Mutex
	rows    []*liveRow
	verb    string
	total   int
	doneCnt int
	errs    int
	started time.Time
	spin    int
	drawn   int // lines drawn by the previous frame, for the in-place rewind

	stop chan struct{}
	done chan struct{}
}

func newLiveReporter(w io.Writer) *liveReporter {
	width, _ := cli.TermWidth(w)
	return &liveReporter{w: w, st: newStyles(), width: width}
}

func (r *liveReporter) Start(verb string, total int) {
	r.mu.Lock()
	r.verb = verb
	r.total = total
	r.started = time.Now()
	r.mu.Unlock()

	r.stop = make(chan struct{})
	r.done = make(chan struct{})
	go r.renderLoop()
}

func (r *liveReporter) renderLoop() {
	defer close(r.done)
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			r.render() // ensure the final state is shown
			return
		case <-t.C:
			r.render()
		}
	}
}

func (r *liveReporter) RepoStart(label string) RepoTracker {
	row := &liveRow{name: label}
	r.mu.Lock()
	r.rows = append(r.rows, row)
	r.mu.Unlock()
	return &liveRepoTracker{r: r, row: row}
}

func (r *liveReporter) Done() {
	r.mu.Lock()
	r.doneCnt++
	r.mu.Unlock()
}

func (r *liveReporter) Stop() {
	if r.stop == nil {
		return
	}
	close(r.stop)
	<-r.done // render goroutine has emitted its final frame and exited (AC-7)

	// Erase the live block so the trackers are ephemeral; the caller prints the
	// authoritative tree-ordered output where the block stood.
	r.mu.Lock()
	n := r.drawn
	r.drawn = 0
	r.mu.Unlock()
	if n > 0 {
		_, _ = io.WriteString(r.w, eraseLines(n))
	}
}

// frame is an immutable snapshot of the reporter state for one render, taken
// under the lock so the (potentially blocking) string build and terminal write
// happen lock-free — workers never stall on a slow TTY while updating progress.
type frame struct {
	rows      []liveRow
	prevDrawn int
	spin      int
	width     int
	verb      string
	errs      int
	doneCnt   int
	total     int
	started   time.Time
}

// render redraws the whole live block in place: rewind over the previous frame,
// then draw a row per active repo plus the overall row.
func (r *liveReporter) render() {
	r.mu.Lock()
	r.spin++
	f := frame{
		rows:      make([]liveRow, len(r.rows)),
		prevDrawn: r.drawn,
		spin:      r.spin,
		width:     r.width,
		verb:      r.verb,
		errs:      r.errs,
		doneCnt:   r.doneCnt,
		total:     r.total,
		started:   r.started,
	}
	for i, row := range r.rows {
		f.rows[i] = *row
	}
	r.drawn = len(f.rows) + 1
	r.mu.Unlock()

	var b strings.Builder
	b.WriteString(eraseLines(f.prevDrawn))
	for i := range f.rows {
		b.WriteString(r.renderRow(&f.rows[i], f.spin, i, f.width))
		b.WriteByte('\n')
	}
	b.WriteString(r.renderOverall(f))
	b.WriteByte('\n')
	// Downsample colors to the terminal's profile (and honor NO_COLOR /
	// CLICOLOR_FORCE) at the write site, since styles are built profile-agnostic.
	_, _ = lipgloss.Fprint(r.w, b.String())
}

// renderRow draws one active repo as a huh-style spinner followed by the repo
// name (mirroring huh's "spinner frame + title" layout). idx staggers the frame
// per row so the rows animate out of phase, like independent spinners would.
func (r *liveReporter) renderRow(row *liveRow, spin, idx, width int) string {
	// Some huh spinner types pad the frame with a trailing space and some do not;
	// normalize to exactly one gap between the spinner and the repo name.
	frame := strings.TrimRight(spinnerFrames[(spin+idx)%len(spinnerFrames)], " ")
	line := r.st.spinner.Render(frame) + " " + r.st.name.Render(row.name)
	return r.clampLine(line, width)
}

func (r *liveReporter) renderOverall(f frame) string {
	nameW, showStats := layoutFor(f.width)
	msg := f.verb
	if f.errs > 0 {
		msg = fmt.Sprintf("%s (%d failed)", f.verb, f.errs)
	}
	bar, pct := r.bar(int64(f.doneCnt), int64(f.total))
	line := r.st.overall.Render(fitName(msg, nameW)) + " ... " + pct + " " + bar
	if showStats {
		line += "  " + r.totalStats(f.doneCnt, f.total, f.started)
	}
	return r.clampLine(line, f.width)
}

// clampLine truncates a rendered row to the terminal width (ANSI-aware) so it
// can never wrap onto a second display line and corrupt the rewind. A width of
// 0 (unknown size) leaves the line untouched.
func (r *liveReporter) clampLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	return r.st.clamp.MaxWidth(width).Render(line)
}

// bar returns the rendered [..........] determinate bar for the overall row and
// its fixed-width whole-number percentage cell (the overall progress counts
// whole repos, so a fractional percent would be meaningless). While work is in
// progress the percentage is floored at 1% so the bar never reads 0%; a
// non-positive total yields an empty bar at 0%.
func (r *liveReporter) bar(current, total int64) (string, string) {
	var ratio float64
	if total > 0 {
		ratio = float64(current) / float64(total)
		if ratio > 1 {
			ratio = 1
		}
	}
	filled := int(ratio*float64(barWidth) + 0.5)
	bar := "[" + r.st.fill.Render(strings.Repeat("#", filled)) +
		r.st.empty.Render(strings.Repeat(".", barWidth-filled)) + "]"
	pct := int(ratio*100 + 0.5)
	if total > 0 && pct < 1 {
		pct = 1 // in-progress bars start at 1%, never 0%
	}
	return bar, r.st.percent.Render(fmt.Sprintf("%6d%%", pct))
}

// totalStats renders the overall row's trailing stat: completed/total repos and
// the elapsed time, e.g. "3/11 · 4.2s".
func (r *liveReporter) totalStats(done, total int, start time.Time) string {
	return r.st.stats.Render(fmt.Sprintf("%d/%d · %s",
		done, total, time.Since(start).Round(100*time.Millisecond)))
}

// eraseLines emits the ANSI to move the cursor up n lines, clearing each, so the
// next write redraws the block in place.
func eraseLines(n int) string {
	var b strings.Builder
	for range n {
		b.WriteString(ansiCursorUp)
		b.WriteString(ansiEraseLine)
	}
	return b.String()
}

// liveRepoTracker maps the RepoTracker contract onto one live row. The row
// spins until the walker calls a completion marker, which removes the row from
// the block so finished repos disappear rather than lingering as idle bars; the
// run-wide failed-count is folded in by MarkErrored.
type liveRepoTracker struct {
	r    *liveReporter
	row  *liveRow
	once sync.Once
}

func (t *liveRepoTracker) MarkDone()    { t.finish(false) }
func (t *liveRepoTracker) MarkErrored() { t.finish(true) }

// finish drops the row from the live block so the finished repo's row
// vanishes, and folds a failure into the reporter's error count — the
// tracker owns the count, so a repo is tallied exactly once even on double
// completion.
func (t *liveRepoTracker) finish(errored bool) {
	t.once.Do(func() {
		t.r.mu.Lock()
		if errored {
			t.r.errs++
		}
		for i, row := range t.r.rows {
			if row == t.row {
				t.r.rows = append(t.r.rows[:i], t.r.rows[i+1:]...)
				break
			}
		}
		t.r.mu.Unlock()
	})
}
