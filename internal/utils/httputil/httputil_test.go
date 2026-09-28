package httputil

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"
)

func TestCreateHTTPClientHonorsInsecureTLSVerifyEnv(t *testing.T) {
	t.Setenv(InsecureSkipTLSVerifyEnv, "true")

	client := CreateHTTPClient(time.Second)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatalf("TLSClientConfig is nil")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify to be true")
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", transport.TLSClientConfig.MinVersion)
	}
}

func TestCreateHTTPClientKeepsTLSVerificationByDefault(t *testing.T) {
	t.Setenv(InsecureSkipTLSVerifyEnv, "")

	client := CreateHTTPClient(time.Second)

	transport := client.Transport.(*http.Transport)
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify to be false by default")
	}
}
