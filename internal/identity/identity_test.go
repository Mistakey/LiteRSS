package identity

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
)

const legacyPort = "1234"

var (
	exeDir    = filepath.Join("C:", "apps", "literss")
	configDir = filepath.Join("C:", "Users", "u", "AppData", "Roaming")
)

func port(t *testing.T, address string) string {
	t.Helper()
	host, p, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", address, err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Fatalf("address %q is not a loopback IP", address)
	}
	return p
}

func TestDevelopmentOverridesEveryProductionValue(t *testing.T) {
	if Production.UniqueID == Development.UniqueID {
		t.Errorf("UniqueID shared: %q", Production.UniqueID)
	}
	if Production.Name == Development.Name {
		t.Errorf("Name shared: %q", Production.Name)
	}
	if Production.AutostartValueName == Development.AutostartValueName {
		t.Errorf("AutostartValueName shared: %q", Production.AutostartValueName)
	}
	prodDir := Production.DataDir(exeDir, configDir, false)
	devDir := Development.DataDir(exeDir, configDir, false)
	if prodDir == devDir {
		t.Errorf("DataDir shared: %q", prodDir)
	}
	if strings.HasPrefix(devDir, configDir) {
		t.Errorf("development DataDir %q is under the user config dir", devDir)
	}
	if Production.APIAddress == Development.APIAddress {
		t.Errorf("APIAddress shared: %q", Production.APIAddress)
	}
}

func TestPorts(t *testing.T) {
	devPort := port(t, Development.APIAddress)
	prodPort := port(t, Production.APIAddress)

	if devPort != "1235" {
		t.Errorf("development port = %s, want 1235", devPort)
	}
	if prodPort == legacyPort || prodPort == devPort {
		t.Errorf("production port %s collides with legacy %s or development %s", prodPort, legacyPort, devPort)
	}
}

func TestProductionValues(t *testing.T) {
	if Production.UniqueID != "io.github.mistakey.literss" {
		t.Errorf("UniqueID = %q", Production.UniqueID)
	}
	if Production.Name != "LiteRSS" || Production.DataDirName != "LiteRSS" {
		t.Errorf("Name/DataDirName = %q/%q", Production.Name, Production.DataDirName)
	}
	// The legacy MrRSS registered "MrRSS" and polled Mistakey/MrRSS.
	if Production.AutostartValueName != "LiteRSS" {
		t.Errorf("AutostartValueName = %q", Production.AutostartValueName)
	}
	if Production.UpdateRepo != "Mistakey/LiteRSS" {
		t.Errorf("UpdateRepo = %q", Production.UpdateRepo)
	}
	if Production.Portable {
		t.Error("production identity must not be portable")
	}
	if Production.BrowserChannel {
		t.Error("production identity must not serve the browser forensics channel")
	}
	if !Production.SearchesLegacyLibrary || Development.SearchesLegacyLibrary {
		t.Error("only the production identity may look for the user's legacy MrRSS library")
	}
	if !Production.InstallsUpdates || Development.InstallsUpdates {
		t.Error("only the production identity may start a downloaded installer (spec D20)")
	}
	if strings.Contains(strings.ToLower(Production.UniqueID), "mrrss") {
		t.Errorf("UniqueID %q reuses the legacy identity", Production.UniqueID)
	}
}

func TestDevelopmentServesBrowserChannel(t *testing.T) {
	if !Development.BrowserChannel {
		t.Error("development identity must serve the browser forensics channel")
	}
}

func TestDataDir(t *testing.T) {
	tests := []struct {
		name   string
		id     Identity
		marker bool
		want   string
	}{
		{"production", Production, false, filepath.Join(configDir, "LiteRSS")},
		{"production with portable.txt", Production, true, filepath.Join(exeDir, "data")},
		{"development", Development, false, filepath.Join(exeDir, "data")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.DataDir(exeDir, configDir, tt.marker); got != tt.want {
				t.Errorf("DataDir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWebviewBrowserArgs(t *testing.T) {
	tests := []struct {
		name string
		id   Identity
		port string
		want string
	}{
		{"development with port", Development, "9333", "--remote-debugging-port=9333"},
		{"development without port", Development, "", ""},
		{"production ignores the variable", Production, "9333", ""},
		{"not a port", Development, "9333 --no-sandbox", ""},
		{"out of range", Development, "70000", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(tt.id.WebviewBrowserArgs(tt.port), " ")
			if got != tt.want {
				t.Errorf("WebviewBrowserArgs(%q) = %q, want %q", tt.port, got, tt.want)
			}
		})
	}
}
