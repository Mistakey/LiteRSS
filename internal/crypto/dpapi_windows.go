package crypto

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// marker prefixes a base64 DPAPI blob.
const marker = "dpapi:"

func encrypt(plaintext string) (string, error) {
	blob, err := dpapiProtect([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return marker + base64.StdEncoding.EncodeToString(blob), nil
}

func decrypt(encoded string) (string, error) {
	blob, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: base64: %v", ErrDecryptionFailed, err)
	}
	plain, err := dpapiUnprotect(blob)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// dpapiProtect seals data for the current Windows user; no other user and no
// other machine can open it.
func dpapiProtect(data []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(newBlob(data), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	return takeBlob(&out), nil
}

func dpapiUnprotect(blob []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(newBlob(blob), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("%w: CryptUnprotectData: %v", ErrDecryptionFailed, err)
	}
	return takeBlob(&out), nil
}

func newBlob(data []byte) *windows.DataBlob {
	if len(data) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

// takeBlob copies a blob the system allocated, then zeroes and frees the
// original: after CryptUnprotectData it holds a plaintext credential.
func takeBlob(b *windows.DataBlob) []byte {
	src := unsafe.Slice(b.Data, b.Size)
	out := append([]byte(nil), src...)
	clear(src)
	windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	return out
}
