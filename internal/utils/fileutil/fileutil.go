// Package fileutil provides file system utilities including path management,
// directory operations, and platform-specific path handling.
package fileutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"LiteRSS/internal/identity"
)

var (
	portableMarker     bool
	portableMarkerOnce sync.Once
)

func hasPortableMarker() bool {
	portableMarkerOnce.Do(func() {
		exePath, err := os.Executable()
		if err != nil {
			return
		}
		_, err = os.Stat(filepath.Join(filepath.Dir(exePath), "portable.txt"))
		portableMarker = err == nil
	})
	return portableMarker
}

// IsPortableMode reports whether data lives beside the executable.
func IsPortableMode() bool {
	return identity.Current().IsPortable(hasPortableMarker())
}

// GetDataDir returns the data directory of the current build identity and
// creates it if needed.
func GetDataDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	configDir := ""
	if !IsPortableMode() {
		if configDir, err = os.UserConfigDir(); err != nil {
			return "", fmt.Errorf("resolve user config dir: %w", err)
		}
	}

	dataDir := identity.Current().DataDir(filepath.Dir(exePath), configDir, hasPortableMarker())
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	return dataDir, nil
}

// GetDBPath returns the full path to the database file.
func GetDBPath() (string, error) {
	dataDir, err := GetDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "rss.db"), nil
}

// GetLogPath returns the full path to the debug log file.
func GetLogPath() (string, error) {
	dataDir, err := GetDataDir()
	if err != nil {
		return "", err
	}

	logsDir := filepath.Join(dataDir, "logs")
	err = os.MkdirAll(logsDir, 0755)
	if err != nil {
		return "", err
	}

	return filepath.Join(logsDir, "debug.log"), nil
}

// OpenRotatedLog moves the previous run's log to <path>.1, replacing the one
// before it, and opens an empty log at path.
func OpenRotatedLog(path string) (*os.File, error) {
	if err := os.Rename(path, path+".1"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("rotate log: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	return f, nil
}

// IsWindows returns true if the current platform is Windows.
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsMacOS returns true if the current platform is MacOS (Darwin).
func IsMacOS() bool {
	return runtime.GOOS == "darwin"
}

// IsLinux returns true if the current platform is Linux.
func IsLinux() bool {
	return runtime.GOOS == "linux"
}
