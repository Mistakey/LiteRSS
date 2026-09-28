//go:build !windows

package crypto

// marker prefixes the ciphertext off Windows: macOS keeps the MrRSS scheme.
const marker = legacyMarker

func encrypt(plaintext string) (string, error) {
	return encryptLegacy(plaintext)
}

func decrypt(encoded string) (string, error) {
	return decryptLegacyPayload(encoded)
}
