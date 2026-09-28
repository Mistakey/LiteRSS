package browser

import (
	"errors"
	"testing"
)

func TestOpen(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "https://example.com/a?b=c#d", want: "https://example.com/a?b=c#d"},
		{raw: "  http://example.com/  ", want: "http://example.com/"},
		{raw: "HTTPS://Example.com/x", want: "https://Example.com/x"},
		{raw: "file:///C:/Windows/System32/calc.exe"},
		{raw: "javascript:alert(1)"},
		{raw: "ms-settings:privacy"},
		{raw: "/relative/path"},
		{raw: "https:///no-host"},
		{raw: ""},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			var opened []string
			err := New(func(u string) error {
				opened = append(opened, u)
				return nil
			}).Open(tt.raw)

			if tt.want == "" {
				if !errors.Is(err, ErrUnsupportedURL) || len(opened) != 0 {
					t.Fatalf("Open(%q) err=%v opened=%v, want rejection", tt.raw, err, opened)
				}
				return
			}
			if err != nil || len(opened) != 1 || opened[0] != tt.want {
				t.Fatalf("Open(%q) err=%v opened=%v, want %q", tt.raw, err, opened, tt.want)
			}
		})
	}
}

func TestOpenReturnsPlatformError(t *testing.T) {
	boom := errors.New("boom")
	if err := New(func(string) error { return boom }).Open("https://example.com"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}
