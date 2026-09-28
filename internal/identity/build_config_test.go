package identity

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"LiteRSS/internal/version"
)

// The installer and bundle metadata under build/ repeat the production
// identity; these tests keep them in step with it and apart from the legacy
// install (spec D2): its install directory, bundle ID and product name.

const buildDir = "../../build"

func readBuildFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(buildDir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// capture returns the first submatch of pattern in the named build file.
func capture(t *testing.T, name, pattern string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(readBuildFile(t, name))
	if m == nil {
		t.Fatalf("%s: no match for %s", name, pattern)
	}
	return m[1]
}

// plistValue returns the string after <key>key</key> in a plist.
func plistValue(t *testing.T, name, key string) string {
	t.Helper()
	return capture(t, name, `<key>`+key+`</key>\s*<string>([^<]*)</string>`)
}

func TestWindowsInstallerMatchesProduction(t *testing.T) {
	const nsi = "windows/installer.nsi"
	if got := capture(t, nsi, `!define APP_NAME "([^"]*)"`); got != Production.Name {
		t.Errorf("APP_NAME = %q, want %q", got, Production.Name)
	}
	if got := capture(t, nsi, `!define APP_EXE "([^"]*)"`); got != Production.Name+".exe" {
		t.Errorf("APP_EXE = %q", got)
	}
	// The legacy installer used $PROGRAMFILES64\MrRSS. LiteRSS installs per
	// user, so the in-app update runs it silently without UAC (spec D20);
	// the directory and the uninstall key both come from APP_NAME.
	if got := capture(t, nsi, `(?m)^InstallDir "([^"]*)"`); got != `$LOCALAPPDATA\Programs\${APP_NAME}` {
		t.Errorf("InstallDir = %q", got)
	}
	if got := capture(t, nsi, `(?m)^RequestExecutionLevel (\w+)`); got != "user" {
		t.Errorf("RequestExecutionLevel = %q, want user", got)
	}
	if got := capture(t, nsi, `(?m)^InstallDirRegKey (\w+)`); got != "HKCU" {
		t.Errorf("InstallDirRegKey root = %q, want HKCU", got)
	}
	for _, line := range strings.Split(readBuildFile(t, nsi), "\n") {
		if strings.Contains(line, "HKLM") && !strings.HasPrefix(strings.TrimSpace(line), ";") {
			t.Errorf("installer.nsi writes HKLM: %s", strings.TrimSpace(line))
		}
	}
	// The app is started through explorer.exe, so an installer that the
	// update elevated never leaves it running as administrator.
	if got := capture(t, nsi, `(?m)^\s*Exec '"\$WINDIR\\explorer\.exe" (.*)'`); got != `"$INSTDIR\${APP_EXE}"` {
		t.Errorf("explorer.exe launch = %q", got)
	}
	if got := capture(t, nsi, `!define MUI_FINISHPAGE_RUN_FUNCTION (\S+)`); got != "LaunchApp" {
		t.Errorf("finish page run function = %q, want LaunchApp", got)
	}
	if got := capture(t, nsi, `!define APP_VERSION "([^"]*)"`); got != version.Version {
		t.Errorf("APP_VERSION = %q, want %q", got, version.Version)
	}
	// Windows version resources take four numbers.
	if got := capture(t, nsi, `!define APP_VERSION_NUMERIC "([^"]*)"`); got != version.Version+".0" {
		t.Errorf("APP_VERSION_NUMERIC = %q", got)
	}
	const tools = "windows/nsis/wails_tools.nsh"
	if got := capture(t, tools, `!define INFO_PRODUCTNAME "([^"]*)"`); got != Production.Name {
		t.Errorf("INFO_PRODUCTNAME = %q", got)
	}
	if got := capture(t, tools, `!define INFO_PRODUCTVERSION "([^"]*)"`); got != version.Version {
		t.Errorf("INFO_PRODUCTVERSION = %q", got)
	}
	const info = "windows/info.json"
	if got := capture(t, info, `"ProductName": "([^"]*)"`); got != Production.Name {
		t.Errorf("info.json ProductName = %q", got)
	}
	if got := capture(t, info, `"ProductVersion": "([^"]*)"`); got != version.Version {
		t.Errorf("info.json ProductVersion = %q", got)
	}
	if got := capture(t, info, `"file_version": "([^"]*)"`); got != version.Version {
		t.Errorf("info.json file_version = %q", got)
	}
	const manifest = "windows/wails.exe.manifest"
	if got := capture(t, manifest, `assemblyIdentity type="win32" name="([^"]*)"`); got != Production.UniqueID {
		t.Errorf("manifest assemblyIdentity = %q", got)
	}
	if got := capture(t, manifest, `assemblyIdentity type="win32" name="[^"]*" version="([^"]*)"`); got != version.Version+".0" {
		t.Errorf("manifest assemblyIdentity version = %q", got)
	}
}

func TestBuildConfigMatchesProduction(t *testing.T) {
	const cfg = "config.yml"
	if got := capture(t, cfg, `productName: "([^"]*)"`); got != Production.Name {
		t.Errorf("productName = %q", got)
	}
	if got := capture(t, cfg, `productIdentifier: "([^"]*)"`); got != Production.UniqueID {
		t.Errorf("productIdentifier = %q", got)
	}
	if got := capture(t, cfg, `(?m)^  version: "([^"]*)"`); got != version.Version {
		t.Errorf("version = %q", got)
	}
}

func TestMacBundleMatchesIdentity(t *testing.T) {
	for _, tt := range []struct {
		file string
		id   Identity
	}{
		{"darwin/Info.plist", Production},
		{"darwin/Info.dev.plist", Development},
	} {
		if got := plistValue(t, tt.file, "CFBundleIdentifier"); got != tt.id.UniqueID {
			t.Errorf("%s CFBundleIdentifier = %q, want %q", tt.file, got, tt.id.UniqueID)
		}
		if got := plistValue(t, tt.file, "CFBundleName"); got != Production.Name {
			t.Errorf("%s CFBundleName = %q", tt.file, got)
		}
		for _, key := range []string{"CFBundleShortVersionString", "CFBundleVersion"} {
			if got := plistValue(t, tt.file, key); got != version.Version {
				t.Errorf("%s %s = %q", tt.file, key, got)
			}
		}
	}
	// darwin:package writes its own Info.plist into the bundle.
	if got := plistValue(t, "darwin/Taskfile.yml", "CFBundleIdentifier"); got != Production.UniqueID {
		t.Errorf("darwin/Taskfile.yml CFBundleIdentifier = %q", got)
	}
	for _, key := range []string{"CFBundleShortVersionString", "CFBundleVersion"} {
		if got := plistValue(t, "darwin/Taskfile.yml", key); got != version.Version {
			t.Errorf("darwin/Taskfile.yml %s = %q", key, got)
		}
	}
}

// TestBuildFilesNeverNameLegacyApp keeps the legacy product name and bundle
// ID out of every text file that goes into a build or an installer.
func TestBuildFilesNeverNameLegacyApp(t *testing.T) {
	binary := map[string]bool{".exe": true, ".png": true, ".ico": true, ".icns": true, ".syso": true}
	err := filepath.WalkDir(buildDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if binary[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(data)), "mrrss") {
			t.Errorf("%s names the legacy app", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
