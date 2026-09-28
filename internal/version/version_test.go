package version

import "testing"

func TestVersionIsSet(t *testing.T) {
	if Version == "" {
		t.Fatalf("Version should not be empty")
	}
}
