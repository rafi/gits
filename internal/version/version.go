// Package version reports the build version stamped in at link time.
package version

import "strings"

var (
	// The version is of the format Major.Minor.Patch[-Prerelease][+BuildMetadata]
	//
	// Increment major for breaking changes, minor for backwards-compatible
	// feature additions, patch for bug fixes and performance work.
	//
	// GetMajorMinor derives the cache-file key from this value, so a major or
	// minor bump invalidates every cache; a patch bump does not.
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
