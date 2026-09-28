// Package update asks GitHub whether a newer LiteRSS is released and, when
// the user clicks 更新, downloads the release's installer, checks it against
// the release's SHA256SUMS and installs it (spec D20): Windows runs the
// installer silently over the running directory, macOS swaps the .app after
// the app quits. Portable copies only get the release page, and development
// builds stop after the checksum.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"LiteRSS/internal/identity"
	"LiteRSS/internal/version"
)

// GitHubAPI is the releases API's base URL. The repository must be public:
// the request carries no credentials (spec D17).
const GitHubAPI = "https://api.github.com"

// GitHubDownloads is where GitHub serves release assets.
const GitHubDownloads = "https://github.com"

// APIEnv names the variable that, in development builds only, points both the
// releases API and the asset downloads at a fake release service
// (tools/fake-release), so an isolated instance can walk the update. Builds
// that install updates ignore it.
const APIEnv = "LITERSS_UPDATE_API"

// checkTimeout bounds one check; the shared client's own timeout is set for
// the model and is longer.
const checkTimeout = 20 * time.Second

// Checker checks one repository's latest release against the running
// version.
type Checker struct {
	// Repo is the GitHub owner/name.
	Repo string
	// API is the releases API's base URL.
	API string
	// Downloads is the base URL an asset's download link must start with:
	// <Downloads>/<Repo>/releases/download/<tag>/<name>.
	Downloads string
	// Current is the running version.
	Current string
	// HTTP carries the request; the caller passes the shared outbound client
	// (pitfall 8).
	HTTP *http.Client
}

// New returns the checker of this build: the repository of its identity and
// the version it was built as. A development build honours APIEnv.
func New(client *http.Client) *Checker {
	c := &Checker{Repo: identity.Current().UpdateRepo, API: GitHubAPI, Downloads: GitHubDownloads,
		Current: version.Version, HTTP: client}
	if base := strings.TrimSuffix(os.Getenv(APIEnv), "/"); base != "" && !identity.Current().InstallsUpdates {
		c.API, c.Downloads = base, base
	}
	return c
}

// Result is the answer to a check. ReleaseURL is the page to open: the newest
// release's, or the repository's release list. Message is the Chinese reason
// when the check failed; the other fields but CurrentVersion are then empty.
// InApp says the update can be installed from the app (Updater.Check sets it);
// otherwise the user is sent to ReleaseURL.
type Result struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url"`
	InApp           bool   `json:"in_app"`
	Message         string `json:"message"`
}

// release is what the releases API says of the latest release.
type release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Asset is one file of a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// ReleasesPage is the repository's release list.
func (c *Checker) ReleasesPage() string {
	return "https://github.com/" + c.Repo + "/releases"
}

// Check asks for the latest release, which GitHub reports without drafts and
// pre-releases.
func (c *Checker) Check(ctx context.Context) Result {
	res, _ := c.latest(ctx)
	return res
}

// latest is Check that also returns the release it read, zero when the check
// failed.
func (c *Checker) latest(ctx context.Context) (Result, release) {
	res := Result{CurrentVersion: c.Current}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(c.API, "/")+"/repos/"+c.Repo+"/releases/latest", nil)
	if err != nil {
		log.Printf("Update check: %v", err)
		res.Message = "检查更新失败。"
		return res, release{}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		log.Printf("Update check: %v", err)
		res.Message = "连不上 GitHub，请检查网络或代理设置。"
		return res, release{}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		res.Message = "还没有发布过版本。"
		return res, release{}
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		res.Message = "GitHub 访问太频繁，稍后再试。"
		return res, release{}
	case resp.StatusCode != http.StatusOK:
		log.Printf("Update check: GitHub answered %d", resp.StatusCode)
		res.Message = fmt.Sprintf("GitHub 返回错误 %d，稍后再试。", resp.StatusCode)
		return res, release{}
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil || rel.TagName == "" {
		log.Printf("Update check: unreadable release: %v", err)
		res.Message = "GitHub 返回的发布信息读不懂，稍后再试。"
		return res, release{}
	}
	res.LatestVersion = strings.TrimPrefix(rel.TagName, "v")
	res.UpdateAvailable = Newer(res.LatestVersion, c.Current)
	// The page opened in the browser must be the repository's own.
	res.ReleaseURL = c.ReleasesPage()
	if strings.HasPrefix(rel.HTMLURL, res.ReleaseURL+"/") {
		res.ReleaseURL = rel.HTMLURL
	}
	return res, rel
}

// Newer reports whether version a is newer than b, comparing the dotted
// numbers; a leading "v" and anything after a '-' or '+' are ignored, and a
// part that is not a number counts as 0.
func Newer(a, b string) bool {
	pa, pb := parts(a), parts(b)
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func parts(v string) []int {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, s := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(s)
		out = append(out, n)
	}
	return out
}
