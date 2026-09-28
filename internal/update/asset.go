package update

import (
	"bufio"
	"encoding/hex"
	"errors"
	"strings"
)

// SumsName is the release asset that lists every other asset's SHA-256, one
// "<hex>  <name>" line each, as sha256sum writes it.
const SumsName = "SHA256SUMS"

// InstallerNames lists the asset names, in order of preference, that install
// version on goos/goarch: the NSIS installer on Windows, the DMG on macOS
// (release.yml builds one universal DMG). Other platforms have none.
func InstallerNames(ver, goos, goarch string) []string {
	switch goos {
	case "windows":
		return []string{"LiteRSS-" + ver + "-windows-" + goarch + "-installer.exe"}
	case "darwin":
		return []string{"LiteRSS-" + ver + "-darwin-universal.dmg", "LiteRSS-" + ver + "-darwin-" + goarch + ".dmg"}
	}
	return nil
}

// errNoInstaller and errNoSums say which asset the release lacks.
var (
	errNoInstaller = errors.New("the release has no installer for this platform")
	errNoSums      = errors.New("the release has no " + SumsName)
)

// pickAssets finds the installer for goos/goarch and the checksum list among
// a release's assets. An asset only counts when its download link is exactly
// where the repository's release serves it: <downloads>/<repo>/releases/download/<tag>/<name>.
func pickAssets(rel release, downloads, repo, goos, goarch string) (installer, sums Asset, err error) {
	prefix := strings.TrimSuffix(downloads, "/") + "/" + repo + "/releases/download/" + rel.TagName + "/"
	find := func(name string) (Asset, bool) {
		for _, a := range rel.Assets {
			if a.Name == name && a.URL == prefix+name {
				return a, true
			}
		}
		return Asset{}, false
	}
	ver := strings.TrimPrefix(rel.TagName, "v")
	found := false
	for _, name := range InstallerNames(ver, goos, goarch) {
		if installer, found = find(name); found {
			break
		}
	}
	if !found {
		return Asset{}, Asset{}, errNoInstaller
	}
	if sums, found = find(SumsName); !found {
		return Asset{}, Asset{}, errNoSums
	}
	return installer, sums, nil
}

// sumFor returns the lowercase hex SHA-256 that a SHA256SUMS file lists for
// name, "" when it lists none. A '*' before the name (binary mode) is allowed.
func sumFor(sums, name string) string {
	sc := bufio.NewScanner(strings.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		sum := strings.ToLower(fields[0])
		if b, err := hex.DecodeString(sum); err == nil && len(b) == 32 {
			return sum
		}
	}
	return ""
}
