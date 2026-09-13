package output

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/rafi/gits/internal/app/cli/style"
)

// PlainView is the transitional view for a command whose body still builds
// its own styled string. It is deleted once every command returns a report.
var PlainView = View[string]{
	Line: func(s string, _ style.Theme) string { return s },
	Text: plain,
}

// plain strips what a body adds for the terminal: styling, and the
// whitespace a line's layout leaves at its ends.
func plain(s string) string {
	return strings.TrimSpace(ansi.Strip(s))
}
