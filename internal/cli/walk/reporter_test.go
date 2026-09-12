package walk

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
	for _, w := range []io.Writer{io.Discard, &bytes.Buffer{}} {
		if _, ok := NewReporter(w).(*nopReporter); !ok {
			t.Errorf("NewReporter(%T) = %T, want *nopReporter", w, NewReporter(w))
		}
	}
}

// TestNopReporterLifecycle exercises the full Reporter call sequence against the
// no-op implementation, including per-repo trackers: it must accept Start, a
// RepoStart with its tracker driven and marked, Done×total and a draining
// Stop without blocking or panicking.
func TestNopReporterLifecycle(t *testing.T) {
	const total = 5
	var r Reporter = &nopReporter{}

	r.Start("fetching", total)
	for i := range total {
		rt := r.RepoStart("repo")
		if i%2 == 0 {
			rt.MarkDone()
		} else {
			rt.MarkErrored()
		}
		r.Done()
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

// TestReporterSatisfiedByImpls is a compile-time guard that both reporters and
// both repo-tracker implementations satisfy their interfaces.
func TestReporterSatisfiedByImpls(t *testing.T) {
	var _ Reporter = (*nopReporter)(nil)
	var _ Reporter = (*liveReporter)(nil)
	var _ RepoTracker = (*nopRepoTracker)(nil)
	var _ RepoTracker = (*liveRepoTracker)(nil)
}

// TestLiveReporterConcurrent drives the real live reporter from several
// goroutines (as the walker does) against a buffer, asserting the full
// multi-tracker lifecycle runs race-free and Stop drains promptly.
func TestLiveReporterConcurrent(t *testing.T) {
	const repos = 8
	r := newLiveReporter(&bytes.Buffer{})
	r.Start("fetching", repos)

	var wg sync.WaitGroup
	for i := range repos {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rt := r.RepoStart("repo")
			if i%3 == 0 {
				rt.MarkErrored()
			} else {
				rt.MarkDone()
			}
			r.Done()
		}(i)
	}
	wg.Wait()

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
	buf := &syncBuffer{}
	r := newLiveReporter(buf)

	r.Stop() // never started: nothing to stop
	r.Start("fetching", 1)
	r.RepoStart("alpha").MarkDone()
	r.Done()

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
	buf := &syncBuffer{}
	r := newLiveReporter(buf)
	r.Start("fetching", 4)

	// Hold two rows live across several 100ms render ticks.
	rt := r.RepoStart("alpha")
	rt2 := r.RepoStart("bravo")
	time.Sleep(350 * time.Millisecond)
	rt.MarkDone()
	rt2.MarkDone()
	r.Done()
	r.Done()
	r.Stop()

	out := buf.String()
	if !strings.Contains(out, ansiCursorUp) {
		t.Fatal("expected cursor-up rewinds (in-place redraw); got none — frames are accumulating")
	}
	if !strings.Contains(out, ansiEraseLine) {
		t.Fatal("expected the live block to be erased on Stop")
	}
}
