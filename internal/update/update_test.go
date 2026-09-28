package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"LiteRSS/internal/identity"
	"LiteRSS/internal/version"
)

// fakeGitHub answers the latest-release request with status and body, and
// records the paths asked for.
func fakeGitHub(t *testing.T, status int, body string) (*httptest.Server, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &paths
}

func TestNewChecksTheIdentityRepository(t *testing.T) {
	c := New(http.DefaultClient)
	if c.Repo != identity.Current().UpdateRepo || c.Current != version.Version || c.API != GitHubAPI {
		t.Fatalf("New = %+v", c)
	}
	if c.ReleasesPage() != "https://github.com/"+identity.Current().UpdateRepo+"/releases" {
		t.Fatalf("releases page = %s", c.ReleasesPage())
	}
}

func TestCheckFindsANewerRelease(t *testing.T) {
	repo := identity.Current().UpdateRepo
	srv, paths := fakeGitHub(t, http.StatusOK,
		`{"tag_name": "v2.0.1", "html_url": "https://github.com/`+repo+`/releases/tag/v2.0.1"}`)
	c := New(srv.Client())
	c.API, c.Current = srv.URL, "2.0.0"

	got := c.Check(context.Background())
	want := Result{CurrentVersion: "2.0.0", LatestVersion: "2.0.1", UpdateAvailable: true,
		ReleaseURL: "https://github.com/" + repo + "/releases/tag/v2.0.1"}
	if got != want {
		t.Fatalf("Check = %+v, want %+v", got, want)
	}
	if len(*paths) != 1 || (*paths)[0] != "/repos/"+repo+"/releases/latest" {
		t.Fatalf("asked for %v", *paths)
	}
}

func TestCheckOpensOnlyTheRepositoryPages(t *testing.T) {
	srv, _ := fakeGitHub(t, http.StatusOK, `{"tag_name": "1.0.0", "html_url": "https://evil.example/x"}`)
	c := New(srv.Client())
	c.API, c.Current = srv.URL, "1.0.0"

	got := c.Check(context.Background())
	if got.UpdateAvailable || got.ReleaseURL != c.ReleasesPage() || got.Message != "" {
		t.Fatalf("Check = %+v", got)
	}
}

func TestCheckFailuresSayWhyInChinese(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusNotFound, `{}`, "还没有发布过版本。"},
		{http.StatusForbidden, `{}`, "GitHub 访问太频繁，稍后再试。"},
		{http.StatusBadGateway, ``, "GitHub 返回错误 502，稍后再试。"},
		{http.StatusOK, `not json`, "GitHub 返回的发布信息读不懂，稍后再试。"},
	} {
		srv, _ := fakeGitHub(t, tc.status, tc.body)
		c := New(srv.Client())
		c.API = srv.URL
		got := c.Check(context.Background())
		if got != (Result{CurrentVersion: version.Version, Message: tc.want}) {
			t.Errorf("status %d: %+v, want message %q", tc.status, got, tc.want)
		}
	}

	c := New(http.DefaultClient)
	c.API = "http://127.0.0.1:1" // nothing listens
	if got := c.Check(context.Background()); got.Message != "连不上 GitHub，请检查网络或代理设置。" {
		t.Fatalf("unreachable: %+v", got)
	}
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"1.3.29", "1.3.28", true},
		{"1.10.0", "1.9.9", true},
		{"v2.0", "1.99.99", true},
		{"1.3.28", "1.3.28", false},
		{"1.3.28", "v1.3.28", false},
		{"1.3", "1.3.0", false},
		{"1.3.27", "1.3.28", false},
		{"2.0.0-rc1", "2.0.0", false},
	} {
		if got := Newer(tc.a, tc.b); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
