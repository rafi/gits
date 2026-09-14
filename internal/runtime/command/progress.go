package command

// Progress receives the lifecycle of a run so a view can show it live. It is
// driven concurrently from the worker goroutines: Begin once, Start per
// repository — its finish is called on the worker that ran it, and reports
// whether the repository failed — then a single Stop that must not return
// until nothing more will be written, so results never interleave with it.
type Progress interface {
	Begin(verb string, total int)
	Start(label string) func(failed bool)
	Stop()
}

// Nop reports nothing. It is the default: an engine handed no Progress runs
// silently rather than assuming a terminal.
type Nop struct{}

// Begin does nothing.
func (Nop) Begin(string, int) {}

// Start does nothing and returns a finish that does nothing.
func (Nop) Start(string) func(failed bool) { return func(bool) {} }

// Stop does nothing.
func (Nop) Stop() {}

// progress returns the command's reporter, or the no-op when it has none.
func (c Command[T]) progress() Progress {
	if c.Progress == nil {
		return Nop{}
	}
	return c.Progress
}
