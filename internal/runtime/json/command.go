package json

// Command is what one of the line commands — pull, fetch, push, clone,
// exec — made of a repository.
//
// The command's own name is a field rather than the key it nests under. A key
// that varied by command could not be spliced in without editing encoded
// JSON, and a consumer had to know which command ran before it could find the
// output; `.command.output` now works whatever ran.
//
// Exactly one of Output, Skipped and Error is set. Build one with OK, Skipped
// or Failed rather than by hand.
type Command struct {
	// Name is the command that ran: "pull" for `gits pull`.
	Name string `json:"name"`
	// Output is what the command produced for the repository — git's own
	// report, or the child's combined output under `exec` — with no terminal
	// styling. Present, even when empty, whenever the command succeeded.
	Output *string `json:"output,omitempty"`
	// Skipped is the reason the command passed the repository over: a
	// documented pass-over, such as a branch with no Upstream, which does not
	// fail the command. It is not an error and is not reported as one.
	Skipped string `json:"skipped,omitempty"`
	// Error is why the command failed on the repository.
	Error string `json:"error,omitempty"`
}

// OK reports a command that ran and produced output. The output is carried
// even when empty, which is what distinguishes "ran and said nothing" from
// "did not run".
func OK(name, output string) *Command {
	return &Command{Name: name, Output: &output}
}

// Skipped reports a documented pass-over: the command declined to act, and
// the run does not fail for it.
func Skipped(name, reason string) *Command {
	return &Command{Name: name, Skipped: reason}
}

// Failed reports a command that failed on the repository.
func Failed(name string, err error) *Command {
	return &Command{Name: name, Error: err.Error()}
}
