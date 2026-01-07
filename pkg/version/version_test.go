package version

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}

	// Version should be semver-like
	parts := strings.Split(Version, ".")
	if len(parts) < 2 {
		t.Errorf("Version should have at least major.minor format, got: %s", Version)
	}
}
