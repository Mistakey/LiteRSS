// Package legacyprobe tells whether the legacy MrRSS is running, which turns
// articles read here back to unread on FreshRSS (spec D18). It only looks:
// nothing here can take the legacy app's single-instance lock.
package legacyprobe

// MutexName is the Windows mutex Wails v3 creates for the installed MrRSS's
// single-instance lock: "wails-app-" + its UniqueID + "-sim". It is fixed by
// the installed legacy binary, not by this repository (pitfall 32).
const MutexName = "wails-app-com.mrrss.app-sim"

// Running reports whether the legacy MrRSS runs in this session. Only
// Windows can tell; elsewhere it is always false.
func Running() bool {
	return mutexExists(MutexName)
}
