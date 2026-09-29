// Package feedicon serves feed icons from FreshRSS's favicon cache (spec
// D15). An icon is fetched the first time it is asked for and kept in the
// library until the feed's iconUrl changes; a feed FreshRSS has no icon for is
// remembered too, and asked again after NoIconRetry. The frontend shows the
// letter badge whenever there is no icon.
package feedicon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"LiteRSS/internal/freshrss"
)

// NoIconRetry is how long a feed found without an icon waits before FreshRSS
// is asked again; FreshRSS refreshes its own favicons about as rarely.
const NoIconRetry = 7 * 24 * time.Hour

var (
	// ErrNoIcon: the feed is unknown, has no iconUrl, or FreshRSS has no icon
	// for it.
	ErrNoIcon = errors.New("feedicon: no icon")
	// ErrUnavailable: FreshRSS could not be asked now (not configured, or
	// unreachable); nothing is remembered, so the next request asks again.
	ErrUnavailable = errors.New("feedicon: FreshRSS unavailable")
)

// Source fetches an icon for an iconUrl; *freshrss.Client is one.
type Source interface {
	Icon(ctx context.Context, iconURL string) (freshrss.Icon, error)
}

// Service answers icons for feeds of one library.
type Service struct {
	db     *sql.DB
	source func() (Source, error)
	now    func() time.Time

	mu       sync.Mutex
	inflight map[string]chan struct{}
}

// New returns a service; source is called for each fetch, so it follows the
// FreshRSS account in the settings.
func New(db *sql.DB, source func() (Source, error)) *Service {
	return &Service{db: db, source: source, now: time.Now, inflight: map[string]chan struct{}{}}
}

// Icon returns the icon of the feed with this stream id. Concurrent requests
// for one feed share a single fetch.
func (s *Service) Icon(ctx context.Context, streamID string) (freshrss.Icon, error) {
	for {
		icon, iconURL, fresh, err := s.cached(ctx, streamID)
		if err != nil || fresh {
			return icon, err
		}

		s.mu.Lock()
		wait, busy := s.inflight[streamID]
		if !busy {
			s.inflight[streamID] = make(chan struct{})
		}
		s.mu.Unlock()
		if busy {
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return freshrss.Icon{}, ctx.Err()
			}
		}

		icon, err = s.fetch(ctx, streamID, iconURL)
		s.mu.Lock()
		close(s.inflight[streamID])
		delete(s.inflight, streamID)
		s.mu.Unlock()
		return icon, err
	}
}

// cached reads what the library holds for the feed. fresh reports that the
// answer stands without asking FreshRSS; otherwise iconURL is what to fetch.
func (s *Service) cached(ctx context.Context, streamID string) (icon freshrss.Icon, iconURL string, fresh bool, err error) {
	var cachedURL sql.NullString
	var data []byte
	var contentType sql.NullString
	var fetchedAt sql.NullInt64
	err = s.db.QueryRowContext(ctx,
		`SELECT f.icon_url, i.icon_url, i.data, i.content_type, i.fetched_at
		   FROM feeds f LEFT JOIN feed_icons i ON i.stream_id = f.stream_id
		  WHERE f.stream_id = ?`, streamID).
		Scan(&iconURL, &cachedURL, &data, &contentType, &fetchedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return icon, "", true, ErrNoIcon
	case err != nil:
		return icon, "", true, fmt.Errorf("read feed icon: %w", err)
	case iconURL == "":
		return icon, "", true, ErrNoIcon
	case !cachedURL.Valid || cachedURL.String != iconURL:
		return icon, iconURL, false, nil
	case len(data) > 0:
		return freshrss.Icon{Data: data, ContentType: contentType.String}, iconURL, true, nil
	case s.now().Sub(time.Unix(fetchedAt.Int64, 0)) < NoIconRetry:
		return icon, iconURL, true, ErrNoIcon
	}
	return icon, iconURL, false, nil
}

// fetch asks FreshRSS and keeps its answer: the icon, or that there is none.
// A failure to reach FreshRSS is not kept.
func (s *Service) fetch(ctx context.Context, streamID, iconURL string) (freshrss.Icon, error) {
	src, err := s.source()
	if err != nil {
		return freshrss.Icon{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	icon, err := src.Icon(ctx, iconURL)
	var apiErr *freshrss.APIError
	switch {
	case errors.Is(err, freshrss.ErrNoIcon), errors.As(err, &apiErr):
		// Empty, not nil: data is NOT NULL.
		icon, err = freshrss.Icon{Data: []byte{}}, ErrNoIcon
	case err != nil:
		return freshrss.Icon{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	// The feed may have left or changed its iconUrl meanwhile; only the
	// answer for its current iconUrl is kept.
	if _, dbErr := s.db.ExecContext(ctx,
		`INSERT INTO feed_icons (stream_id, icon_url, data, content_type, fetched_at)
		 SELECT stream_id, icon_url, ?, ?, ? FROM feeds WHERE stream_id = ? AND icon_url = ?
		 ON CONFLICT (stream_id) DO UPDATE SET
		   icon_url = excluded.icon_url, data = excluded.data,
		   content_type = excluded.content_type, fetched_at = excluded.fetched_at`,
		icon.Data, icon.ContentType, s.now().Unix(), streamID, iconURL); dbErr != nil {
		return freshrss.Icon{}, fmt.Errorf("keep feed icon: %w", dbErr)
	}
	return icon, err
}
