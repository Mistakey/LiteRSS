package syncer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html"

	"LiteRSS/internal/database"
	"LiteRSS/internal/freshrss"
)

// scanOverlap reaches back below the local high-water mark: a concurrent
// FreshRSS refresh can commit an entry after one with a larger id.
const scanOverlap = int64(10 * time.Minute / time.Microsecond)

// pull brings the FreshRSS-owned side of the library up to date: feeds and
// tags, entries new to the library, and the server_read mirror.
func (s *Service) pull(ctx context.Context, remote Remote) (Result, error) {
	var res Result
	var err error
	if res.Removed, err = s.syncSubscriptions(ctx, remote); err != nil {
		return res, err
	}
	unread, err := s.unreadIDs(ctx, remote)
	if err != nil {
		return res, err
	}
	scanned, err := s.scanNew(ctx, remote)
	if err != nil {
		return res, err
	}
	if res.Added, err = s.fetchMissing(ctx, remote, unread, scanned); err != nil {
		return res, err
	}
	return res, s.mirrorRead(ctx, unread)
}

// syncSubscriptions overwrites feeds, tags and feed_tags with the server's
// lists and deletes what the server no longer has: feeds, tags, the articles
// of unsubscribed feeds with all their local data, and stream-level intents
// naming a stream that is gone. An empty subscription list while the library
// has feeds or articles is taken as a server fault and changes nothing:
// articles count because an import brings them without feeds. It returns the
// number of articles deleted.
func (s *Service) syncSubscriptions(ctx context.Context, remote Remote) (int, error) {
	subs, err := remote.GetSubscriptions(ctx)
	if err != nil {
		return 0, fmt.Errorf("list subscriptions: %w", err)
	}
	categories, err := remote.GetCategories(ctx)
	if err != nil {
		return 0, fmt.Errorf("list tags: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin subscriptions: %w", err)
	}
	defer tx.Rollback()

	if len(subs) == 0 {
		var feeds, articles int
		if err := tx.QueryRowContext(ctx,
			`SELECT (SELECT COUNT(*) FROM feeds), (SELECT COUNT(*) FROM articles)`).Scan(&feeds, &articles); err != nil {
			return 0, fmt.Errorf("count library: %w", err)
		}
		if feeds > 0 || articles > 0 {
			log.Printf("sync: subscription/list returned no subscriptions while the library has %d feeds and %d articles; keeping it", feeds, articles)
			return 0, nil
		}
	}

	labels := map[string]string{}
	for _, c := range categories {
		labels[c.ID] = c.Label
	}
	feedIDs := make([]string, 0, len(subs))
	for _, sub := range subs {
		feedIDs = append(feedIDs, sub.ID)
		for _, c := range sub.Categories {
			if strings.HasPrefix(c.ID, freshrss.LabelPrefix) {
				if _, ok := labels[c.ID]; !ok {
					labels[c.ID] = c.Label
				}
			}
		}
	}
	tagIDs := make([]string, 0, len(labels))
	for id, label := range labels {
		tagIDs = append(tagIDs, id)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tags (tag_id, label) VALUES (?, ?)
			 ON CONFLICT(tag_id) DO UPDATE SET label = excluded.label`, id, label); err != nil {
			return 0, fmt.Errorf("upsert tag %s: %w", id, err)
		}
	}
	for _, sub := range subs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO feeds (stream_id, title, url, site_url, icon_url) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(stream_id) DO UPDATE SET
			   title = excluded.title, url = excluded.url, site_url = excluded.site_url, icon_url = excluded.icon_url`,
			sub.ID, sub.Title, sub.URL, sub.HTMLURL, sub.IconURL); err != nil {
			return 0, fmt.Errorf("upsert feed %s: %w", sub.ID, err)
		}
		// feed_tags is a pure join table; nothing cascades from it.
		if _, err := tx.ExecContext(ctx, `DELETE FROM feed_tags WHERE stream_id = ?`, sub.ID); err != nil {
			return 0, fmt.Errorf("clear feed_tags of %s: %w", sub.ID, err)
		}
		for _, c := range sub.Categories {
			if _, ok := labels[c.ID]; !ok {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO feed_tags (stream_id, tag_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, sub.ID, c.ID); err != nil {
				return 0, fmt.Errorf("link feed %s to %s: %w", sub.ID, c.ID, err)
			}
		}
	}

	feedsJSON, err := jsonList(feedIDs)
	if err != nil {
		return 0, err
	}
	tagsJSON, err := jsonList(tagIDs)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM feeds WHERE stream_id NOT IN (SELECT value FROM json_each(?))`, feedsJSON); err != nil {
		return 0, fmt.Errorf("delete unsubscribed feeds: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tags WHERE tag_id NOT IN (SELECT value FROM json_each(?))`, tagsJSON); err != nil {
		return 0, fmt.Errorf("delete removed tags: %w", err)
	}
	// Cascades to contents, full text, translations, summaries and pending_read.
	r, err := tx.ExecContext(ctx,
		`DELETE FROM articles WHERE stream_id NOT IN (SELECT value FROM json_each(?))`, feedsJSON)
	if err != nil {
		return 0, fmt.Errorf("delete articles of unsubscribed feeds: %w", err)
	}
	removed, err := r.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted articles: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM pending_mark_all
		 WHERE stream_id <> ?
		   AND stream_id NOT IN (SELECT value FROM json_each(?))
		   AND stream_id NOT IN (SELECT value FROM json_each(?))`,
		freshrss.StreamReadingList, feedsJSON, tagsJSON); err != nil {
		return 0, fmt.Errorf("delete intents of removed streams: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit subscriptions: %w", err)
	}
	return int(removed), nil
}

// unreadIDs is the server's unread set.
func (s *Service) unreadIDs(ctx context.Context, remote Remote) (map[int64]bool, error) {
	unread := map[int64]bool{}
	q := freshrss.ItemIDQuery{Stream: freshrss.StreamReadingList, Exclude: freshrss.StreamRead}
	err := s.eachID(ctx, remote, q, func(id int64) bool {
		unread[id] = true
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("unread ids: %w", err)
	}
	return unread, nil
}

// scanNew walks the reading list from the newest entry down to the local
// high-water mark (less scanOverlap) or the retention edge, whichever comes
// first, so entries read elsewhere while this instance was off still land in
// the library.
func (s *Service) scanNew(ctx context.Context, remote Remote) ([]int64, error) {
	var highWater int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(item_id), 0) FROM articles`).Scan(&highWater); err != nil {
		return nil, fmt.Errorf("read high-water mark: %w", err)
	}
	stop := max(highWater-scanOverlap, s.retentionEdge())

	var ids []int64
	err := s.eachID(ctx, remote, freshrss.ItemIDQuery{Stream: freshrss.StreamReadingList}, func(id int64) bool {
		if id <= stop {
			return false
		}
		ids = append(ids, id)
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("scan reading list: %w", err)
	}
	return ids, nil
}

// eachID hands fn the ids of q, newest first, until fn returns false or the
// stream ends. One request normally takes them all; paging only happens past
// idPageSize, where an entry that stops matching between pages costs the next
// one (pitfall 28). For the unread set that entry mirrors as read until the
// next cycle.
func (s *Service) eachID(ctx context.Context, remote Remote, q freshrss.ItemIDQuery, fn func(int64) bool) error {
	q.Count = s.idPageSize
	for {
		page, err := remote.StreamItemIDs(ctx, q)
		if err != nil {
			return err
		}
		for _, id := range page.IDs {
			if !fn(id) {
				return nil
			}
		}
		if page.Continuation == "" || page.Continuation == q.Continuation {
			return nil
		}
		q.Continuation = page.Continuation
	}
}

// retentionEdge is the oldest item id, i.e. crawl time in microseconds, still
// inside retention.
func (s *Service) retentionEdge() int64 {
	return s.now().Add(-Retention).UnixMicro()
}

// fetchMissing downloads the entries of the unread set and the scan that the
// library lacks, read or not, in batches of contentsBatch. Entries the server
// no longer has are skipped. It returns how many entries were added.
//
// Batches go oldest first: the next scan stops at the newest stored entry, so
// a cycle cut short must only have stored entries older than every one it
// missed, or those it missed that are read would never be fetched.
func (s *Service) fetchMissing(ctx context.Context, remote Remote, unread map[int64]bool, scanned []int64) (int, error) {
	candidates := make([]int64, 0, len(unread)+len(scanned))
	for id := range unread {
		candidates = append(candidates, id)
	}
	candidates = append(candidates, scanned...)
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)

	local, err := s.existing(ctx, candidates)
	if err != nil {
		return 0, err
	}
	missing := slices.DeleteFunc(candidates, func(id int64) bool { return local[id] })

	added := 0
	for batch := range slices.Chunk(missing, s.contentsBatch) {
		items, err := remote.StreamItemContents(ctx, batch)
		if err != nil {
			return added, fmt.Errorf("fetch contents: %w", err)
		}
		if err := s.storeItems(ctx, items, unread); err != nil {
			return added, err
		}
		added += len(items)
	}
	return added, nil
}

// existing reports which of ids the library already has.
func (s *Service) existing(ctx context.Context, ids []int64) (map[int64]bool, error) {
	idsJSON, err := jsonList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT item_id FROM articles WHERE item_id IN (SELECT value FROM json_each(?))`, idsJSON)
	if err != nil {
		return nil, fmt.Errorf("find local items: %w", err)
	}
	defer rows.Close()
	have := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read local item id: %w", err)
		}
		have[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read local items: %w", err)
	}
	return have, nil
}

// storeItems writes one batch of entries and their RSS bodies. Only
// FreshRSS-owned columns are written, and by upsert: a replace would cascade
// away the article's local data (pitfall 26).
func (s *Service) storeItems(ctx context.Context, items []freshrss.Item, unread map[int64]bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin store items: %w", err)
	}
	defer tx.Rollback()
	for _, it := range items {
		read := 1
		if unread[it.ID] {
			read = 0
		}
		published := database.NormalizePublishedAt(fmt.Sprint(it.Published), it.ID)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO articles (item_id, stream_id, url, title, image_url, published_at, server_read)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(item_id) DO UPDATE SET
			   stream_id = excluded.stream_id, url = excluded.url, title = excluded.title,
			   image_url = excluded.image_url, published_at = excluded.published_at, server_read = excluded.server_read`,
			it.ID, it.StreamID, it.URL, it.Title, firstImage(it.Content), published, read); err != nil {
			return fmt.Errorf("store item %d: %w", it.ID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO article_contents (item_id, content) VALUES (?, ?)
			 ON CONFLICT(item_id) DO UPDATE SET content = excluded.content`, it.ID, it.Content); err != nil {
			return fmt.Errorf("store content %d: %w", it.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit store items: %w", err)
	}
	return nil
}

// mirrorRead sets server_read from the unread set: in it is unread, anything
// else the library holds is read. Intents are untouched; they win on display.
func (s *Service) mirrorRead(ctx context.Context, unread map[int64]bool) error {
	ids := make([]int64, 0, len(unread))
	for id := range unread {
		ids = append(ids, id)
	}
	idsJSON, err := jsonList(ids)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mirror: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE articles SET server_read = 0
		 WHERE server_read = 1 AND item_id IN (SELECT value FROM json_each(?))`, idsJSON); err != nil {
		return fmt.Errorf("mirror unread: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE articles SET server_read = 1
		 WHERE server_read = 0 AND item_id NOT IN (SELECT value FROM json_each(?))`, idsJSON); err != nil {
		return fmt.Errorf("mirror read: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mirror: %w", err)
	}
	return nil
}

// cleanup deletes articles crawled before the retention edge that show as
// read and have no pending intent, with their local data, then hands the
// space back. It returns how many articles were deleted.
func (s *Service) cleanup(ctx context.Context) (int, error) {
	r, err := s.db.ExecContext(ctx,
		`DELETE FROM articles
		 WHERE item_id < ? AND server_read = 1
		   AND NOT EXISTS (SELECT 1 FROM pending_read p WHERE p.item_id = articles.item_id)`,
		s.retentionEdge())
	if err != nil {
		return 0, fmt.Errorf("clean up past retention: %w", err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count cleaned articles: %w", err)
	}
	if n > 0 {
		if err := incrementalVacuum(ctx, s.db.DB); err != nil {
			return int(n), err
		}
	}
	return int(n), nil
}

// jsonList encodes a list for binding to json_each(?), which is how a whole
// set goes into one parameterized statement.
func jsonList[T any](v []T) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode id list: %w", err)
	}
	return string(b), nil
}

// incrementalVacuum frees the whole freelist. The pragma frees one page per
// step, so its rows are drained rather than run with Exec.
func incrementalVacuum(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA incremental_vacuum`)
	if err != nil {
		return fmt.Errorf("incremental_vacuum: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("incremental_vacuum: %w", err)
	}
	return nil
}

// firstImage is the list thumbnail: the first <img> whose src is an absolute
// http(s) URL. The body is untrusted, so anything else is passed over.
func firstImage(body string) string {
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "img" {
				continue
			}
			for hasAttr {
				var key, val []byte
				key, val, hasAttr = z.TagAttr()
				if string(key) != "src" {
					continue
				}
				if u, err := url.Parse(string(val)); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
					return u.String()
				}
			}
		}
	}
}
