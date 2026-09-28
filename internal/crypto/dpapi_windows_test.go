package crypto

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// dpapiBlobHeader starts every DPAPI blob: dwVersion 1, then the provider GUID
// {df9d8cd0-1501-11d1-8c7a-00c04fc297eb} in its little-endian layout.
var dpapiBlobHeader = []byte{
	0x01, 0x00, 0x00, 0x00,
	0xd0, 0x8c, 0x9d, 0xdf, 0x01, 0x15, 0xd1, 0x11, 0x8c, 0x7a, 0x00, 0xc0, 0x4f, 0xc2, 0x97, 0xeb,
}

func TestEncryptProducesDPAPIBlob(t *testing.T) {
	encrypted, err := Encrypt("freshrss-api-password")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if !strings.HasPrefix(encrypted, marker) {
		t.Fatalf("ciphertext %q lacks marker %q", encrypted, marker)
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, marker))
	if err != nil {
		t.Fatalf("ciphertext is not base64: %v", err)
	}
	if !bytes.HasPrefix(blob, dpapiBlobHeader) {
		t.Errorf("blob header = % x, want DPAPI header % x", blob[:min(len(blob), len(dpapiBlobHeader))], dpapiBlobHeader)
	}

	// The blob is plain DPAPI: CryptUnprotectData opens it with no help from us.
	plain, err := dpapiUnprotect(blob)
	if err != nil || string(plain) != "freshrss-api-password" {
		t.Errorf("dpapiUnprotect() = %q, %v", plain, err)
	}
}
