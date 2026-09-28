package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"LiteRSS/internal/identity"
	"LiteRSS/internal/update"
	"LiteRSS/internal/update/updatetest"
)

func TestOpenInBrowserOnlyTakesWebLinks(t *testing.T) {
	api := newTestAPI(t)
	if rec := api.send(t, http.MethodPost, "/api/browser/open", `{"url": " https://example.com/a?b=1 "}`); rec.Code != http.StatusNoContent {
		t.Fatalf("https link: %d %s", rec.Code, rec.Body)
	}
	for _, body := range []string{
		`{"url": "file:///C:/Windows/System32/calc.exe"}`,
		`{"url": "javascript:alert(1)"}`,
		`{"url": "ms-settings:"}`,
		`{"url": "//example.com/a"}`,
		`{"url": ""}`,
		`{"href": "https://example.com/"}`,
	} {
		if rec := api.send(t, http.MethodPost, "/api/browser/open", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, rec.Code)
		}
	}
	if !slices.Equal(api.opened, []string{"https://example.com/a?b=1"}) {
		t.Fatalf("opened %v", api.opened)
	}
}

// testUpdater is the updater main wires in, on a fake release service with
// a newer release; in tests the build is a development one, so an update
// stops after the checksum.
func testUpdater(t *testing.T, target update.Target) (*update.Updater, *[]string) {
	t.Helper()
	repo := identity.Current().UpdateRepo
	fake := updatetest.New(repo)
	fake.Publish(updatetest.Release{Tag: "v99.0.0", Files: map[string][]byte{
		"LiteRSS-99.0.0-windows-amd64-installer.exe": []byte("installer"),
	}})
	var paths []string
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(github.Close)
	checker := update.New(github.Client())
	checker.API, checker.Downloads = github.URL, github.URL
	u := update.NewUpdater(context.Background(), checker)
	u.GOOS, u.GOARCH, u.Installs = "windows", "amd64", identity.Current().InstallsUpdates
	u.Dir = filepath.Join(t.TempDir(), "LiteRSS-update")
	u.Locate = func() update.Target { return target }
	u.Run = func(update.Plan) error { t.Error("a development build ran the installer"); return nil }
	u.Quit = func() { t.Error("a development build quit for an update") }
	return u, &paths
}

// TestCheckUpdate checks the release of the build identity's repository.
func TestCheckUpdate(t *testing.T) {
	u, paths := testUpdater(t, update.Target{Kind: update.WindowsInstaller, Dir: `C:\LiteRSS`})
	api := newTestAPI(t)
	api.updates = u
	api.handler = Handler(api.deps())

	var got update.Result
	api.getJSON(t, "/api/update/check", &got)
	if !got.UpdateAvailable || got.LatestVersion != "99.0.0" || !got.InApp || got.Message != "" {
		t.Fatalf("check = %+v", got)
	}
	if want := "/repos/" + identity.Current().UpdateRepo + "/releases/latest"; !slices.Equal(*paths, []string{want}) {
		t.Fatalf("asked GitHub for %v, want %s", *paths, want)
	}
}

// TestStartUpdate walks an update through the routes: start answers 202 with
// the status, and status reports the development build's stop.
func TestStartUpdate(t *testing.T) {
	u, _ := testUpdater(t, update.Target{Kind: update.WindowsInstaller, Dir: `C:\LiteRSS`})
	api := newTestAPI(t)
	api.updates = u
	api.handler = Handler(api.deps())

	var idle update.Status
	api.getJSON(t, "/api/update/status", &idle)
	if idle.State != update.StateIdle {
		t.Fatalf("status before start = %+v", idle)
	}
	rec := api.send(t, http.MethodPost, "/api/update/start", "")
	var started update.Status
	if rec.Code != http.StatusAccepted || json.NewDecoder(rec.Body).Decode(&started) != nil || started.State != update.StateDownloading {
		t.Fatalf("POST update/start: %d %+v", rec.Code, started)
	}
	deadline := time.Now().Add(10 * time.Second)
	var st update.Status
	for api.getJSON(t, "/api/update/status", &st); st.State != update.StateNotInstalled; api.getJSON(t, "/api/update/status", &st) {
		if time.Now().After(deadline) || st.State == update.StateFailed {
			t.Fatalf("status = %+v", st)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if st.Version != "99.0.0" || st.Received != int64(len("installer")) {
		t.Fatalf("status = %+v", st)
	}
}
