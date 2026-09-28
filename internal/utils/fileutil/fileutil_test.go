package fileutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// go test builds without -tags production, so these run under the
// development identity and must never resolve to the user config directory.
func TestGetDataDirIsBesideExecutable(t *testing.T) {
	dir, err := GetDataDir()
	if err != nil {
		t.Fatalf("GetDataDir failed: %v", err)
	}
	exePath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(filepath.Dir(exePath), "data"); dir != want {
		t.Errorf("GetDataDir = %q, want %q", dir, want)
	}
	if configDir, err := os.UserConfigDir(); err == nil && strings.HasPrefix(dir, configDir) {
		t.Errorf("development data dir %q is under %q", dir, configDir)
	}
	if !IsPortableMode() {
		t.Error("development build must report portable mode")
	}
}

func TestGetLogPath(t *testing.T) {
	path, err := GetLogPath()
	if err != nil {
		t.Fatalf("GetLogPath failed: %v", err)
	}
	if filepath.Base(path) != "debug.log" || filepath.Base(filepath.Dir(path)) != "logs" {
		t.Errorf("GetLogPath = %q, want .../logs/debug.log", path)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeRun(t *testing.T, path, content string) {
	t.Helper()
	f, err := OpenRotatedLog(path)
	if err != nil {
		t.Fatalf("OpenRotatedLog: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRotatedLogKeepsPreviousRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")

	writeRun(t, path, "run 1")
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("first run created %s.1 (err=%v)", path, err)
	}

	writeRun(t, path, "run 2")
	if got := readFile(t, path+".1"); got != "run 1" {
		t.Errorf("previous log = %q, want %q", got, "run 1")
	}

	writeRun(t, path, "run 3")
	if got := readFile(t, path+".1"); got != "run 2" {
		t.Errorf("previous log = %q, want %q", got, "run 2")
	}
	if got := readFile(t, path); got != "run 3" {
		t.Errorf("current log = %q, want %q", got, "run 3")
	}
}
