package style

import (
	"bytes"
	"os"
	"testing"
)

// TestTermWidth: non-terminal writers (buffers, /dev/null) report not-a-TTY
// with zero width, so callers can fall back to unbounded rendering.
func TestTermWidth(t *testing.T) {
	t.Parallel()

	if w, isTTY := TermWidth(&bytes.Buffer{}); w != 0 || isTTY {
		t.Errorf("TermWidth(buffer) = (%d, %v), want (0, false)", w, isTTY)
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if w, isTTY := TermWidth(null); w != 0 || isTTY {
		t.Errorf("TermWidth(devnull) = (%d, %v), want (0, false)", w, isTTY)
	}
}
