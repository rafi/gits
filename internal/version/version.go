package version

import "strings"

var (
	// The version is of the format Major.Minor.Patch[-Prerelease][+BuildMetadata]
	//
	// Increment major number for new feature additions and behavioral changes.
	// Increment minor number for bug fixes and performance enhancements.
	version = "v0.11.2"

	// metadata is extra build time data
	metadata = ""
)

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
