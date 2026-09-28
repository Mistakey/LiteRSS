//go:build production

package update

import (
	"net/http"
	"testing"
)

func TestNewIgnoresTheFakeReleaseServiceInProductionBuilds(t *testing.T) {
	t.Setenv(APIEnv, "http://127.0.0.1:1241")
	if c := New(http.DefaultClient); c.API != GitHubAPI || c.Downloads != GitHubDownloads {
		t.Fatalf("production build: %+v", c)
	}
}
