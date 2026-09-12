package git

// Compile-time assertion that the concrete *Git client satisfies the
// Client interface. If a method is added to Client without a matching
// implementation on *Git (or a signature drifts), this fails to compile.
var _ Client = (*Git)(nil)
