package version

import "testing"

//nolint:paralleltest // rewrites the package-level version; must stay serial.
func TestGetMajorMinor(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	tests := []struct {
		name string
		ver  string
		want string
	}{
		{"plain semver", "v0.11.2", "v0.11"},
		{"prerelease stripped", "v1.2.3-rc1", "v1.2"},
		{"build metadata ignored", "v1.2.3+build.5", "v1.2"},
		{"already major.minor", "v2.0", "v2.0"},
		{"degenerate single part", "v1", "v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version = tt.ver
			if got := GetMajorMinor(); got != tt.want {
				t.Errorf("GetMajorMinor() with %q = %q, want %q", tt.ver, got, tt.want)
			}
		})
	}
}

//nolint:paralleltest // rewrites the package-level version and metadata; must stay serial.
func TestGetVersion(t *testing.T) {
	origVer, origMeta := version, metadata
	t.Cleanup(func() { version, metadata = origVer, origMeta })

	version = "v1.0.0"
	metadata = ""
	if got := GetVersion(); got != "v1.0.0" {
		t.Errorf("GetVersion() without metadata = %q, want %q", got, "v1.0.0")
	}

	metadata = "deadbeef"
	if got := GetVersion(); got != "v1.0.0+deadbeef" {
		t.Errorf("GetVersion() with metadata = %q, want %q", got, "v1.0.0+deadbeef")
	}
}
