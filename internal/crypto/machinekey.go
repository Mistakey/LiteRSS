package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
)

// The machine-key scheme, used on macOS: AES-256-GCM under a PBKDF2 key
// derived from the host name, OS and architecture. The marker keeps the
// string it was first written with, so stored values stay readable.
const (
	pbkdf2Iterations = 600000
	keySize          = 32
	saltSize         = 16
	machineKeyMarker = "MrRSS-v1:"
)

// machineID feeds the key derivation.
func machineID() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %w", err)
	}
	return fmt.Sprintf("%s-%s-%s-", hostname, runtime.GOOS, runtime.GOARCH), nil
}

// derivedKeys caches keys by machine ID and salt. Each stored value has its
// own salt, and 600k PBKDF2 rounds per credential would make every settings
// read on macOS take seconds.
var derivedKeys sync.Map

func deriveKey(machineID string, salt []byte) ([]byte, error) {
	cacheKey := machineID + "\x00" + string(salt)
	if k, ok := derivedKeys.Load(cacheKey); ok {
		return k.([]byte), nil
	}
	k, err := pbkdf2.Key(sha256.New, machineID, salt, pbkdf2Iterations, keySize)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	derivedKeys.Store(cacheKey, k)
	return k, nil
}

// encryptMachineKey writes [salt][nonce][ciphertext+tag], base64, behind the marker.
func encryptMachineKey(plaintext string) (string, error) {
	id, err := machineID()
	if err != nil {
		return "", err
	}
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	key, err := deriveKey(id, salt)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	out := append(salt, nonce...)
	out = gcm.Seal(out, nonce, []byte(plaintext), nil)
	return machineKeyMarker + base64.StdEncoding.EncodeToString(out), nil
}

func decryptMachineKey(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: base64: %v", ErrDecryptionFailed, err)
	}
	id, err := machineID()
	if err != nil {
		return "", err
	}
	if len(data) < saltSize {
		return "", fmt.Errorf("%w: ciphertext too short", ErrDecryptionFailed)
	}
	key, err := deriveKey(id, data[:saltSize])
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	data = data[saltSize:]
	if len(data) < gcm.NonceSize()+gcm.Overhead() {
		return "", fmt.Errorf("%w: ciphertext too short", ErrDecryptionFailed)
	}
	plaintext, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}
	return string(plaintext), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return gcm, nil
}
