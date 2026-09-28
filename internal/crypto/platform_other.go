//go:build !windows

package crypto

// marker prefixes the ciphertext off Windows, where the machine-key scheme
// is used.
const marker = machineKeyMarker

func encrypt(plaintext string) (string, error) {
	return encryptMachineKey(plaintext)
}

func decrypt(encoded string) (string, error) {
	return decryptMachineKey(encoded)
}
