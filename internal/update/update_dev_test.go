//go:build !production

package update

import (
	"net/http"
	"testing"
)

func TestNewHonoursTheFakeReleaseServiceInDevelopmentBuilds(t *testing.T) {
	t.Setenv(APIEnv, "http://127.0.0.1:1241/")
	c := New(http.DefaultClient)
	if c.API != "http://127.0.0.1:1241" || c.Downloads != "http://127.0.0.1:1241" {
		t.Fatalf("development build: %+v", c)
	}
}
