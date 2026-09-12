package bulk

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// ansiRe strips SGR color sequences so a rendered cell can be compared as text.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainText(s string) string { return strings.TrimSpace(ansiRe.ReplaceAllString(s, "")) }

// syncBuffer is a concurrency-safe sink for the async render goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestNewReporterNonTTYReturnsNop verifies AC-8: a non-TTY writer must yield a
// no-op reporter so piped/CI output is not garbled by ANSI cursor controls.
func TestNewReporterNonTTYReturnsNop(t *testing.T) {
	t.Parallel()

	for _, w := range []io.Writer{io.Discard, &bytes.Buffer{}} {
		if _, ok := newReporter(w).(*nopReporter); !ok {
			t.Errorf("newReporter(%T) = %T, want *nopReporter", w, newReporter(w))
		}
	}
}

// TestNopReporterLifecycle exercises the full reporter call sequence against the
// no-op implementation: it must accept Begin, a Start per repo with its finish
// called either way, and a draining Stop without blocking or panicking.
func TestNopReporterLifecycle(t *testing.T) {
	t.Parallel()

	const total = 5
	var r reporter = &nopReporter{}

	r.Begin("fetching", total)
	for i := range total {
		r.Start("repo")(i%2 == 1)
	}

	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()
	<-done // Stop must return promptly (drains the render goroutine)
}

// TestFitName checks padding, exact fit, and ellipsis truncation, including the
// degenerate single-column width a very narrow terminal can force.
func TestFitName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		s    string
		w    int
		want string
	}{
		{"abc", 5, "abc  "},
		{"abc", 3, "abc"},
		{"abcdef", 4, "abc…"},
		{"abc", 1, "…"},
		{"abc", 0, "…"}, // w<1 is clamped to 1
	}
	for _, tt := range tests {
		if got := fitName(tt.s, tt.w); got != tt.want {
			t.Errorf("fitName(%q, %d) = %q, want %q", tt.s, tt.w, got, tt.want)
		}
	}
}

// TestLayoutFor verifies the width adapts the row: full name + stats when wide,
// stats dropped when tight, name column shrunk (never below 1) when very narrow,
// and the full layout when the size is unknown.
func TestLayoutFor(t *testing.T) {
	t.Parallel()

	const core = len(" ... ") + 7 + 1 + barWidth + 2
	tests := []struct {
		name      string
		width     int
		wantName  int
		wantStats bool
	}{
		{"unknown", 0, nameWidth, true},
		{"wide", 200, nameWidth, true},
		{"exact full", nameWidth + core + 13, nameWidth, true},
		{"no stats room", nameWidth + core, nameWidth, false},
		{"shrink name", nameWidth + core - 4, nameWidth - 4, false},
		{"floor at one", 1, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotName, gotStats := layoutFor(tt.width)
			if gotName != tt.wantName || gotStats != tt.wantStats {
				t.Errorf("layoutFor(%d) = (%d, %v), want (%d, %v)",
					tt.width, gotName, gotStats, tt.wantName, tt.wantStats)
			}
		})
	}
}

// TestBarPercentStartsAtOne verifies the overall percentage starts at 1% rather
// than 0% once there is work in progress, reaches 100% at completion, and stays
// 0% only when there is no work (non-positive total).
func TestBarPercentStartsAtOne(t *testing.T) {
	t.Parallel()

	r := newLiveReporter(&bytes.Buffer{})
	tests := []struct {
		current, total int64
		want           string
	}{
		{0, 10, "1%"},    // start: never 0% while work is pending
		{1, 1000, "1%"},  // tiny progress floors to 1%, not 0%
		{5, 10, "50%"},   // mid-run rounds normally
		{10, 10, "100%"}, // completion is exact
		{0, 0, "0%"},     // no work: empty bar at 0%
	}
	for _, tt := range tests {
		_, pct := r.bar(tt.current, tt.total)
		if got := plainText(pct); got != tt.want {
			t.Errorf("bar(%d, %d) pct = %q, want %q", tt.current, tt.total, got, tt.want)
		}
	}
}

// TestReporterSatisfiedByImpls is a compile-time guard that both reporters
// satisfy the interface.
func TestReporterSatisfiedByImpls(t *testing.T) {
	t.Parallel()

	var _ reporter = (*nopReporter)(nil)
	var _ reporter = (*liveReporter)(nil)
}

// TestLiveReporterConcurrent drives the real live reporter from several
// goroutines (as the worker pool does) against a buffer, asserting the full
// multi-tracker lifecycle runs race-free and Stop drains promptly.
func TestLiveReporterConcurrent(t *testing.T) {
	t.Parallel()

	const repos = 8
	r := newLiveReporter(&bytes.Buffer{})
	r.Begin("fetching", repos)

	var wg sync.WaitGroup
	for i := range repos {
		wg.Go(func() {
			r.Start("repo")(i%3 == 0)
		})
	}
	wg.Wait()

	r.mu.Lock()
	finished, failed, live := r.doneCnt, r.errs, len(r.rows)
	r.mu.Unlock()
	if finished != repos || failed != 3 || live != 0 {
		t.Errorf("done/failed/live = %d/%d/%d, want %d/3/0", finished, failed, live, repos)
	}

	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()
	<-done
}

// TestLiveReporterStopIsIdempotent guards the double-close panic: Stop closes
// r.stop, so a second call must be a no-op rather than close an already-closed
// channel. A Stop before Start stays a no-op too, and must not spend the guard —
// the reporter still has to stop for real once it has been started.
func TestLiveReporterStopIsIdempotent(t *testing.T) {
	t.Parallel()

	buf := &syncBuffer{}
	r := newLiveReporter(buf)

	r.Stop() // never started: nothing to stop
	r.Begin("fetching", 1)
	r.Start("alpha")(false)

	r.Stop()
	before := buf.String()
	r.Stop() // must not panic on close-of-closed
	if after := buf.String(); after != before {
		t.Errorf("second Stop wrote %q, want no further output", after[len(before):])
	}
}

// TestLiveReporterRedrawsInPlace is the regression guard for the
// accumulating-frames bug: an earlier implementation appended each tick instead
// of overwriting it. The renderer must emit cursor-up rewinds while rows stay
// live, and Stop must erase the block.
func TestLiveReporterRedrawsInPlace(t *testing.T) {
	t.Parallel()

	buf := &syncBuffer{}
	r := newLiveReporter(buf)
	r.Begin("fetching", 4)

	// Hold two rows live across several 100ms render ticks.
	finishAlpha := r.Start("alpha")
	finishBravo := r.Start("bravo")
	time.Sleep(350 * time.Millisecond)
	finishAlpha(false)
	finishBravo(false)
	r.Stop()

	out := buf.String()
	if !strings.Contains(out, ansiCursorUp) {
		t.Fatal("expected cursor-up rewinds (in-place redraw); got none — frames are accumulating")
	}
	if !strings.Contains(out, ansiEraseLine) {
		t.Fatal("expected the live block to be erased on Stop")
	}
}
