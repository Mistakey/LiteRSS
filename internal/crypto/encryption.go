// Package crypto encrypts the credentials kept in settings (spec D3).
//
// Encrypt and Decrypt use the platform scheme: DPAPI in the current user's
// scope on Windows, the machine-key AES scheme on macOS.
package crypto

import (
	"errors"
	"fmt"
	"strings"
)

// ErrDecryptionFailed is returned when a ciphertext cannot be opened, for
// example because it was written by another user or on another machine.
var ErrDecryptionFailed = errors.New("decryption failed")

// Encrypt returns the platform ciphertext of plaintext, prefixed with the
// platform marker. An empty plaintext stays empty.
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	return encrypt(plaintext)
}

// Decrypt opens a value written by Encrypt on this platform. An empty value
// stays empty; anything without the platform marker is an error.
func Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, marker) {
		return "", fmt.Errorf("%w: missing or invalid version marker", ErrDecryptionFailed)
	}
	return decrypt(strings.TrimPrefix(ciphertext, marker))
}
