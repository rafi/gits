package version

import "strings"

var (
	// The version is of the format Major.Minor.Patch[-Prerelease][+BuildMetadata]
	//
	// Increment major number for new feature additions and behavioral changes.
	// Increment minor number for bug fixes and performance enhancements.
	version = "v0.9.0"

	// metadata is extra build time data
	metadata = ""
	// gitCommit is the git sha1
	gitCommit = "HEAD"
	// gitTreeState is the state of the git tree
	gitTreeState = ""
)

// BuildInfo describes the compile time information.
type BuildInfo struct {
	// Version is the current semver.
	Version string `json:"version,omitempty"`
	// GitCommit is the git sha1.
	GitCommit string `json:"git_commit,omitempty"`
	// GitTreeState is the state of the git tree.
	GitTreeState string `json:"git_tree_state,omitempty"`
}

// GetVersion returns the semver string of the version
func GetVersion() string {
	if metadata == "" {
		return version
	}
	return version + "+" + metadata
}

// GetMajorMinor returns the major.minor version for cache files.
func GetMajorMinor() string {
	beforeDash := version
	parts := strings.Split(version, "-")
	if len(parts) > 1 {
		beforeDash = parts[0]
	}
	parts = strings.Split(beforeDash, ".")
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

// Get returns build info
func Get() BuildInfo {
	return BuildInfo{
		Version:      GetVersion(),
		GitCommit:    gitCommit,
		GitTreeState: gitTreeState,
	}
}
