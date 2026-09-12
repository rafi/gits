package git

// Compile-time assertion that the concrete *Git client satisfies the
// GitClient interface. If a method is added to GitClient without a matching
// implementation on *Git (or a signature drifts), this fails to compile.
var _ GitClient = (*Git)(nil)
