package crypto

import (
	"errors"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
	}{
		{"simple text", "hello world"},
		{"api key", "sk-1234567890abcdefghijklmnopqrstuvwxyz"},
		{"password with special chars", "P@ssw0rd!#$%^&*()"},
		{"unicode text", "你好世界🌍"},
		{"long text", strings.Repeat("a", 1000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}
			if strings.Contains(encrypted, tt.plaintext) {
				t.Errorf("ciphertext %q contains the plaintext", encrypted)
			}
			decrypted, err := Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}
			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptEmptyStaysEmpty(t *testing.T) {
	encrypted, err := Encrypt("")
	if err != nil || encrypted != "" {
		t.Fatalf("Encrypt(\"\") = %q, %v; want empty", encrypted, err)
	}
	decrypted, err := Decrypt("")
	if err != nil || decrypted != "" {
		t.Fatalf("Decrypt(\"\") = %q, %v; want empty", decrypted, err)
	}
}

func TestEncryptIsRandomized(t *testing.T) {
	a, err := Encrypt("test-secret-key")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt("test-secret-key")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Errorf("two encryptions of the same plaintext are equal: %q", a)
	}
}

func TestDecryptInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"plain text", "not-encrypted"},
		{"marker with bad base64", marker + "not-valid-base64!@#$"},
		{"marker with garbage blob", marker + "SGVsbG8gV29ybGQh"},
		{"legacy value", legacyMarker + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decrypt(tt.input); !errors.Is(err, ErrDecryptionFailed) {
				t.Errorf("Decrypt(%q) error = %v, want ErrDecryptionFailed", tt.input, err)
			}
		})
	}
}

func TestDecryptLegacyRoundTrip(t *testing.T) {
	encrypted, err := encryptLegacy("sk-legacy-secret")
	if err != nil {
		t.Fatalf("encryptLegacy() error = %v", err)
	}
	if !strings.HasPrefix(encrypted, legacyMarker) {
		t.Fatalf("legacy ciphertext %q lacks marker %q", encrypted, legacyMarker)
	}
	decrypted, err := DecryptLegacy(encrypted)
	if err != nil {
		t.Fatalf("DecryptLegacy() error = %v", err)
	}
	if decrypted != "sk-legacy-secret" {
		t.Errorf("DecryptLegacy() = %q", decrypted)
	}
}

func TestDecryptLegacyInvalidInput(t *testing.T) {
	for _, input := range []string{
		"no-marker",
		legacyMarker + "not-valid-base64!@#$",
		legacyMarker + "YWJj",
		legacyMarker + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		if _, err := DecryptLegacy(input); err == nil {
			t.Errorf("DecryptLegacy(%q) succeeded, want error", input)
		}
	}
}

func TestMachineIDIsStable(t *testing.T) {
	a, err := machineID()
	if err != nil {
		t.Fatalf("machineID() error = %v", err)
	}
	b, err := machineID()
	if err != nil {
		t.Fatalf("machineID() error = %v", err)
	}
	if a == "" || a != b {
		t.Errorf("machineID() = %q then %q, want the same non-empty value", a, b)
	}
}

func TestDeriveKey(t *testing.T) {
	derive := func(id string, salt []byte) string {
		t.Helper()
		k, err := deriveKey(id, salt)
		if err != nil {
			t.Fatalf("deriveKey() error = %v", err)
		}
		return string(k)
	}
	salt := []byte("1234567890123456")
	key := derive("test-machine", salt)
	if len(key) != keySize {
		t.Fatalf("deriveKey() returned %d bytes, want %d", len(key), keySize)
	}
	if key != derive("test-machine", salt) {
		t.Error("deriveKey() is not deterministic")
	}
	if key == derive("test-machine", []byte("6543210987654321")) {
		t.Error("deriveKey() ignores the salt")
	}
	if key == derive("other-machine", salt) {
		t.Error("deriveKey() ignores the machine ID")
	}
}
