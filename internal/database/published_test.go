package database

import (
	"strconv"
	"testing"
	"time"
)

func TestNormalizePublishedAt(t *testing.T) {
	fetched := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	itemID := fetched.UnixMicro() + 123 // sub-second part of the ID is dropped
	f := fetched.Unix()

	tests := []struct {
		name string
		raw  string
		want int64
	}{
		{"missing", "", f},
		{"zero", "0", f},
		{"negative", "-5", f},
		{"unparsable", "yesterday", f},
		{"fractional", "1756728000.5", f},
		{"more than a day after fetch", itoa(f + 86400 + 1), f},
		{"exactly a day after fetch", itoa(f + 86400), f + 86400},
		{"slightly after fetch", itoa(f + 3600), f + 3600},
		{"before fetch", itoa(f - 30*86400), f - 30*86400},
		{"same as fetch", itoa(f), f},
		{"surrounding spaces", " " + itoa(f-60) + " ", f - 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizePublishedAt(tt.raw, itemID); got != tt.want {
				t.Errorf("NormalizePublishedAt(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestFetchedAt(t *testing.T) {
	// FreshRSS item IDs are fetch times in microseconds.
	if got := FetchedAt(1_700_000_000_999_999); !got.Equal(time.Unix(1_700_000_000, 999_999_000)) {
		t.Errorf("FetchedAt = %v", got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
