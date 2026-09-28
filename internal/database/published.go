package database

import (
	"strconv"
	"strings"
	"time"
)

// maxPublishedLead is how far past the fetch time a published date may lie
// before it is treated as bogus.
const maxPublishedLead = 24 * time.Hour

// FetchedAt is when FreshRSS fetched an item: its ID is that time in
// microseconds since the Unix epoch.
func FetchedAt(itemID int64) time.Time {
	return time.UnixMicro(itemID)
}

// NormalizePublishedAt turns the raw greader "published" value (Unix seconds)
// into the articles.published_at column, used only for display and ordering
// (spec D9). A value that is missing, zero or negative, unparsable, or more
// than a day after the fetch time is replaced by the fetch time; anything
// earlier is kept as is.
func NormalizePublishedAt(raw string, itemID int64) int64 {
	fetched := FetchedAt(itemID).Unix()
	published, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || published <= 0 || published-fetched > int64(maxPublishedLead/time.Second) {
		return fetched
	}
	return published
}
