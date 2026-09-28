package routes

import "net/http"

// Window is the main window as the frameless title bar drives it (spec D4);
// the desktop shell implements it.
type Window interface {
	Minimise()
	// ToggleMaximise maximizes the window, or restores it when maximized.
	ToggleMaximise()
	// Close does what the window's × does: hide to the tray or quit, as
	// close_to_tray says.
	Close()
}

// MinimiseWindow answers POST /api/window/minimise, 204.
func MinimiseWindow(win Window) http.Handler {
	return windowAction(win, Window.Minimise)
}

// ToggleMaximiseWindow answers POST /api/window/maximise, 204; the title
// bar's maximize button and a double click on it on Windows both call it.
func ToggleMaximiseWindow(win Window) http.Handler {
	return windowAction(win, Window.ToggleMaximise)
}

// CloseWindow answers POST /api/window/close, 204.
func CloseWindow(win Window) http.Handler {
	return windowAction(win, Window.Close)
}

func windowAction(win Window, act func(Window)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		act(win)
		w.WriteHeader(http.StatusNoContent)
	})
}
