// Package version reports the build version stamped in at link time.
package version

import "strings"

var (
	// The version is of the format Major.Minor.Patch[-Prerelease][+BuildMetadata]
	//
	// Increment major number for new feature additions and behavioral changes.
	// Increment minor number for bug fixes and performance enhancements.
	version = "v0.11.2"

	// metadata is extra build time data.
	metadata = ""
)

// majorMinorFields is how many dot-separated fields a version must have
// before a major.minor prefix can be taken from it.
const majorMinorFields = 2

// GetVersion returns the semver string of the version.
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
	if len(parts) < majorMinorFields {
		return version
	}
	return parts[0] + "." + parts[1]
}
