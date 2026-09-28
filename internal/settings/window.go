package settings

// minimizedBound is the coordinate at or below which a saved window position
// is Windows' minimized placeholder, not a place on any screen. Windows
// reports -32000 for a minimized window, divided by the display scale when
// read as logical pixels (-21333 at 150%); a real monitor arrangement never
// reaches this far left or up (pitfall 33).
const minimizedBound = -10000

// MinimizedWindowPos reports whether a saved window position is the
// minimized placeholder, which is neither saved nor applied (spec D4).
func MinimizedWindowPos(x, y int) bool {
	return x <= minimizedBound || y <= minimizedBound
}
