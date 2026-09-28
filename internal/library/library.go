// Package library holds the read paths of the local library the list,
// sidebar and detail pane show: view snapshots, cards, bodies, unread counts
// and the subscription tree (spec D8, D13). It never writes; the user's
// changes go through internal/syncer's intents.
package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"LiteRSS/internal/syncer"
)

// MaxCards is how many cards one request may ask for.
const MaxCards = 200

var (
	// ErrBadRequest reports input the caller got wrong.
	ErrBadRequest = errors.New("bad request")
	// ErrNotFound reports an article that is not in the library.
	ErrNotFound = errors.New("not found")
)

// Library reads the local library.
type Library struct {
	db *sql.DB
}

// New returns the reader of db.
func New(db *sql.DB) *Library {
	return &Library{db: db}
}

// View is what a list shows: the articles of a stream (a feed, a label or
// the reading list), all of them or those shown unread.
type View struct {
	Stream     string
	UnreadOnly bool
}

// Snapshot is a view's members as the view was entered: its item IDs in
// list order. The list holds all of them, so "this and below" and
// mark-all cover the whole view, loaded or not (spec D7, D8).
type Snapshot struct {
	IDs []int64 `json:"ids"`
	// Newest is the largest item ID among IDs, the fetch time of the newest
	// member: the ts a mark-all of this view is cut at. 0 for an empty view.
	Newest int64 `json:"newest"`
}

// Snapshot returns the view's item IDs ordered by (published_at DESC,
// item_id DESC), all of them in one answer: members are fixed once taken,
// so items read afterwards turn grey rather than drop out, and items synced
// afterwards reach the list only through a new snapshot.
func (l *Library) Snapshot(ctx context.Context, view View) (Snapshot, error) {
	if !syncer.ValidStream(view.Stream) {
		return Snapshot{}, fmt.Errorf("%w: stream %q", ErrBadRequest, view.Stream)
	}
	unread := 0
	if view.UnreadOnly {
		unread = 1
	}
	rows, err := l.db.QueryContext(ctx,
		`SELECT a.item_id FROM articles a
		 WHERE `+syncer.InStream+` AND (@unread = 0 OR `+syncer.DisplayRead+` = 0)
		 ORDER BY a.published_at DESC, a.item_id DESC`,
		sql.Named("stream", view.Stream), sql.Named("unread", unread))
	if err != nil {
		return Snapshot{}, fmt.Errorf("snapshot: %w", err)
	}
	defer rows.Close()
	s := Snapshot{IDs: []int64{}}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return Snapshot{}, fmt.Errorf("snapshot row: %w", err)
		}
		s.IDs = append(s.IDs, id)
		s.Newest = max(s.Newest, id)
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("snapshot rows: %w", err)
	}
	return s, nil
}

// Card is what a list row shows of an article.
type Card struct {
	ID     int64  `json:"id"`
	Stream string `json:"stream_id"`
	// FeedTitle is empty for an article whose feed the library does not
	// know yet (imported before the first sync).
	FeedTitle string `json:"feed_title"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	// TranslatedTitle is empty while the title is undecided; equal to Title
	// it means the title was judged Chinese (pitfall 12).
	TranslatedTitle string `json:"translated_title"`
	ImageURL        string `json:"image_url"`
	PublishedAt     int64  `json:"published_at"`
	Read            bool   `json:"read"`
	// Excerpt is plain text from the RSS body, to be shown as text.
	Excerpt string `json:"excerpt"`
}

// Cards returns the cards of ids in the order asked, skipping IDs no longer
// in the library: the retention cleanup may have taken the oldest items of a
// snapshot.
func (l *Library) Cards(ctx context.Context, ids []int64) ([]Card, error) {
	if len(ids) > MaxCards {
		return nil, fmt.Errorf("%w: %d cards asked, at most %d", ErrBadRequest, len(ids), MaxCards)
	}
	cards := []Card{}
	if len(ids) == 0 {
		return cards, nil
	}
	idsJSON := "[" + joinIDs(ids) + "]"
	rows, err := l.db.QueryContext(ctx,
		`SELECT a.item_id, a.stream_id, COALESCE(f.title, ''), a.url, a.title, COALESCE(t.translated_title, ''),
		        a.image_url, a.published_at, `+syncer.DisplayRead+`, COALESCE(c.content, '')
		 FROM articles a
		 LEFT JOIN feeds f ON f.stream_id = a.stream_id
		 LEFT JOIN title_translations t ON t.item_id = a.item_id
		 LEFT JOIN article_contents c ON c.item_id = a.item_id
		 WHERE a.item_id IN (SELECT value FROM json_each(?))`, idsJSON)
	if err != nil {
		return nil, fmt.Errorf("cards: %w", err)
	}
	defer rows.Close()
	found := make(map[int64]Card, len(ids))
	for rows.Next() {
		var (
			c       Card
			content string
		)
		if err := rows.Scan(&c.ID, &c.Stream, &c.FeedTitle, &c.URL, &c.Title, &c.TranslatedTitle,
			&c.ImageURL, &c.PublishedAt, &c.Read, &content); err != nil {
			return nil, fmt.Errorf("card row: %w", err)
		}
		c.Excerpt = Excerpt(content)
		found[c.ID] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("card rows: %w", err)
	}
	for _, id := range ids {
		if c, ok := found[id]; ok {
			cards = append(cards, c)
		}
	}
	return cards, nil
}

// Content returns an article's RSS body, untrusted HTML the frontend
// sanitizes (spec D16); empty when the feed sent none.
func (l *Library) Content(ctx context.Context, id int64) (string, error) {
	var content string
	err := l.db.QueryRowContext(ctx,
		`SELECT COALESCE(c.content, '') FROM articles a
		 LEFT JOIN article_contents c ON c.item_id = a.item_id WHERE a.item_id = ?`, id).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("article %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("content: %w", err)
	}
	return content, nil
}

// Counts are the sidebar's live unread counts: articles shown unread, an URL
// group counted once per scope (spec D8).
type Counts struct {
	// Total covers the reading list, including articles whose feed the
	// library does not know yet.
	Total int            `json:"total"`
	Feeds map[string]int `json:"feeds"`
	Tags  map[string]int `json:"tags"`
}

// UnreadCounts counts unread articles for the reading list, each feed and
// each label, leaving out scopes with none. It is one statement, so the
// three agree with each other.
func (l *Library) UnreadCounts(ctx context.Context) (Counts, error) {
	rows, err := l.db.QueryContext(ctx,
		`WITH u AS (
		   SELECT a.stream_id, CASE WHEN a.url <> '' THEN 'u:' || a.url ELSE 'i:' || a.item_id END AS k
		   FROM articles a WHERE `+syncer.DisplayRead+` = 0)
		 SELECT 'total', '', COUNT(DISTINCT k) FROM u
		 UNION ALL
		 SELECT 'feed', stream_id, COUNT(DISTINCT k) FROM u GROUP BY stream_id
		 UNION ALL
		 SELECT 'tag', ft.tag_id, COUNT(DISTINCT u.k) FROM u JOIN feed_tags ft ON ft.stream_id = u.stream_id GROUP BY ft.tag_id`)
	if err != nil {
		return Counts{}, fmt.Errorf("unread counts: %w", err)
	}
	defer rows.Close()
	counts := Counts{Feeds: map[string]int{}, Tags: map[string]int{}}
	for rows.Next() {
		var (
			kind, scope string
			n           int
		)
		if err := rows.Scan(&kind, &scope, &n); err != nil {
			return Counts{}, fmt.Errorf("unread count row: %w", err)
		}
		switch kind {
		case "total":
			counts.Total = n
		case "feed":
			counts.Feeds[scope] = n
		case "tag":
			counts.Tags[scope] = n
		}
	}
	if err := rows.Err(); err != nil {
		return Counts{}, fmt.Errorf("unread count rows: %w", err)
	}
	return counts, nil
}

// Feed is a subscription in the sidebar.
type Feed struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	SiteURL string `json:"site_url"`
	IconURL string `json:"icon_url"`
}

// Category is a FreshRSS label and its feeds. A feed with several labels is
// under each.
type Category struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Feeds []Feed `json:"feeds"`
}

// Tree is the sidebar under "all subscriptions": the labels, then the feeds
// in no label.
type Tree struct {
	Categories []Category `json:"categories"`
	Feeds      []Feed     `json:"feeds"`
}

// Tree returns the subscription tree as the last sync left it, labels and
// feeds sorted by name. A label holding no feed is left out: FreshRSS keeps
// its default category even when it is empty.
func (l *Library) Tree(ctx context.Context) (Tree, error) {
	tree := Tree{Categories: []Category{}, Feeds: []Feed{}}
	rows, err := l.db.QueryContext(ctx,
		`WITH tree (tag_id, label, stream_id, title, url, site_url, icon_url) AS (
		   SELECT t.tag_id, t.label, f.stream_id, f.title, f.url, f.site_url, f.icon_url
		   FROM tags t
		   JOIN feed_tags ft ON ft.tag_id = t.tag_id
		   JOIN feeds f ON f.stream_id = ft.stream_id
		   UNION ALL
		   SELECT NULL, NULL, f.stream_id, f.title, f.url, f.site_url, f.icon_url FROM feeds f
		   WHERE NOT EXISTS (SELECT 1 FROM feed_tags ft WHERE ft.stream_id = f.stream_id)
		 )
		 SELECT * FROM tree ORDER BY label IS NULL, label COLLATE NOCASE, tag_id, title COLLATE NOCASE, stream_id`)
	if err != nil {
		return Tree{}, fmt.Errorf("subscription tree: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			tagID, label                     sql.NullString
			id, title, url, siteURL, iconURL sql.NullString
		)
		if err := rows.Scan(&tagID, &label, &id, &title, &url, &siteURL, &iconURL); err != nil {
			return Tree{}, fmt.Errorf("subscription tree row: %w", err)
		}
		feed := Feed{ID: id.String, Title: title.String, URL: url.String, SiteURL: siteURL.String, IconURL: iconURL.String}
		if !tagID.Valid {
			tree.Feeds = append(tree.Feeds, feed)
			continue
		}
		n := len(tree.Categories)
		if n == 0 || tree.Categories[n-1].ID != tagID.String {
			tree.Categories = append(tree.Categories, Category{ID: tagID.String, Label: label.String, Feeds: []Feed{}})
			n++
		}
		if id.Valid {
			tree.Categories[n-1].Feeds = append(tree.Categories[n-1].Feeds, feed)
		}
	}
	if err := rows.Err(); err != nil {
		return Tree{}, fmt.Errorf("subscription tree rows: %w", err)
	}
	return tree, nil
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}
