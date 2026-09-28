//go:build !production

package identity

import "testing"

func TestUntaggedBuildIsDevelopment(t *testing.T) {
	if Current() != Development {
		t.Errorf("Current() = %+v, want Development", Current())
	}
}
