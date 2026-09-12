package cli

import (
	"io"
	"os"

	"golang.org/x/term"
)

// TermWidth returns the terminal column count of w and whether w is a
// terminal. Non-TTY writers (pipes, CI logs, test buffers) report (0, false);
// a terminal whose size cannot be determined reports (0, true).
func TermWidth(w io.Writer) (int, bool) {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return 0, false
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0, true
	}
	return cols, true
}
