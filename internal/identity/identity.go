// Package identity holds every value that tells an installed LiteRSS apart
// from a development build and from the legacy MrRSS install. Production
// builds are compiled with `-tags production`; any other build, including
// `go build`, `go test` and `wails3 dev`, uses the development identity so an
// agent can never launch a binary that shares the user's single-instance lock,
// data directory, WebView2 profile or loopback port.
package identity

import (
	"path/filepath"
	"strconv"
)

// Identity is one complete set of process-level identifiers.
type Identity struct {
	// Name is the application and main window title.
	Name string
	// UniqueID is the Wails single-instance lock name.
	UniqueID string
	// DataDirName is the directory created under the user config directory.
	DataDirName string
	// Portable keeps data beside the executable regardless of portable.txt.
	Portable bool
	// APIAddress is the loopback listener of the desktop API.
	APIAddress string
	// BrowserChannel is the development forensics switch (spec D5): the desktop
	// API listener also serves the frontend, so an agent can drive the UI at
	// http://<APIAddress>/, and WebviewDebugPortEnv is honoured.
	BrowserChannel bool
	// AutostartValueName is the registry value (Windows) or agent label (macOS)
	// used for launch at login.
	AutostartValueName string
	// UpdateRepo is the GitHub owner/name polled for new releases.
	UpdateRepo string
	// InstallsUpdates lets the in-app update start the downloaded installer
	// (spec D20). Development builds stop after the checksum and may point
	// the release check at a fake service instead.
	InstallsUpdates bool
}

// Production is the identity of installed builds. Its port must differ from
// 1234 (the legacy MrRSS listener, which may run side by side) and from the
// development port.
var Production = Identity{
	Name:               "LiteRSS",
	UniqueID:           "io.github.mistakey.literss",
	DataDirName:        "LiteRSS",
	APIAddress:         "127.0.0.1:1236",
	AutostartValueName: "LiteRSS",
	UpdateRepo:         "Mistakey/LiteRSS",
	InstallsUpdates:    true,
}

// Development replaces every Production value that could touch a running
// user instance.
var Development = Identity{
	Name:               "LiteRSS Dev",
	UniqueID:           "io.github.mistakey.literss.dev",
	DataDirName:        "LiteRSS-dev",
	Portable:           true,
	APIAddress:         "127.0.0.1:1235",
	BrowserChannel:     true,
	AutostartValueName: "LiteRSS-dev",
	UpdateRepo:         "Mistakey/LiteRSS",
}

// Current returns the identity selected at build time.
func Current() Identity {
	return current
}

// IsPortable reports whether data lives beside the executable: always for
// portable identities, and for installed builds started next to portable.txt.
func (id Identity) IsPortable(portableMarker bool) bool {
	return id.Portable || portableMarker
}

// DataDir resolves the data directory: <exe dir>/data when IsPortable, else a
// directory under userConfigDir.
func (id Identity) DataDir(exeDir, userConfigDir string, portableMarker bool) string {
	if id.IsPortable(portableMarker) {
		return filepath.Join(exeDir, "data")
	}
	return filepath.Join(userConfigDir, id.DataDirName)
}

// WebviewDataDir is the WebView2 user data folder inside dataDir. Leaving it
// unset makes WebView2 use %APPDATA%\<exe name>, which the legacy MrRSS.exe
// and every dev binary with the same file name would share.
func WebviewDataDir(dataDir string) string {
	return filepath.Join(dataDir, "webview2")
}

// WebviewDebugPortEnv names the variable that, in builds serving the browser
// channel, opens the WebView2 DevTools protocol on a loopback port so an agent
// can inspect the real window (docs/TESTING.md). WebView2's own
// WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS is ignored because Wails passes its
// own browser arguments.
const WebviewDebugPortEnv = "LITERSS_WEBVIEW2_DEBUG_PORT"

// WebviewBrowserArgs returns the extra WebView2 arguments for debugPort, the
// value of WebviewDebugPortEnv. Identities without the browser channel, and
// values that are not a port number, get none.
func (id Identity) WebviewBrowserArgs(debugPort string) []string {
	if !id.BrowserChannel || debugPort == "" {
		return nil
	}
	port, err := strconv.Atoi(debugPort)
	if err != nil || port < 1 || port > 65535 {
		return nil
	}
	return []string{"--remote-debugging-port=" + strconv.Itoa(port)}
}
