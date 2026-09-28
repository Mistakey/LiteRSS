//go:build !windows

package legacyprobe

// mutexExists is false outside Windows: the legacy app's lock there is not a
// named mutex, and the user's legacy install is on Windows.
func mutexExists(string) bool {
	return false
}
