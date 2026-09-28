package library

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"LiteRSS/internal/database"
	"LiteRSS/internal/freshrss"
)

const labelTech = freshrss.LabelPrefix + "Tech"

// article is one row to seed; ID is the fetch time in microseconds.
type article struct {
	ID        int64
	Stream    string
	URL       string
	Title     string
	Published int64
	Read      bool
	Content   string
}

func open(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func exec(t *testing.T, db *database.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func seed(t *testing.T, db *database.DB, articles ...article) {
	t.Helper()
	for _, a := range articles {
		read := 0
		if a.Read {
			read = 1
		}
		exec(t, db, `INSERT INTO articles (item_id, stream_id, url, title, published_at, server_read) VALUES (?, ?, ?, ?, ?, ?)`,
			a.ID, a.Stream, a.URL, a.Title, a.Published, read)
		if a.Content != "" {
			exec(t, db, `INSERT INTO article_contents (item_id, content) VALUES (?, ?)`, a.ID, a.Content)
		}
	}
}

// seedFeeds adds feed/1 and feed/2 under labelTech, and feed/3 in no label.
func seedFeeds(t *testing.T, db *database.DB) {
	t.Helper()
	exec(t, db, `INSERT INTO feeds (stream_id, title, url) VALUES
		('feed/1', 'Beta', 'https://b.example/rss'), ('feed/2', 'alpha', 'https://a.example/rss'), ('feed/3', 'Gamma', '')`)
	exec(t, db, `INSERT INTO tags (tag_id, label) VALUES (?, 'Tech'), (?, 'Empty')`, labelTech, freshrss.LabelPrefix+"Empty")
	exec(t, db, `INSERT INTO feed_tags (stream_id, tag_id) VALUES ('feed/1', ?), ('feed/2', ?)`, labelTech, labelTech)
}

func snapshot(t *testing.T, lib *Library, view View) []int64 {
	t.Helper()
	s, err := lib.Snapshot(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	return s.IDs
}

func TestSnapshotOrdersByPublishedThenItemID(t *testing.T) {
	db := open(t)
	seedFeeds(t, db)
	seed(t, db,
		article{ID: 100, Stream: "feed/1", Published: 50},
		article{ID: 200, Stream: "feed/2", Published: 70},
		article{ID: 300, Stream: "feed/3", Published: 50, Read: true},
		article{ID: 400, Stream: "feed/1", Published: 60},
	)
	lib := New(db.DB)

	all := snapshot(t, lib, View{Stream: freshrss.StreamReadingList})
	if want := []int64{200, 400, 300, 100}; !slices.Equal(all, want) {
		t.Fatalf("all = %v, want %v", all, want)
	}
	unread := snapshot(t, lib, View{Stream: freshrss.StreamReadingList, UnreadOnly: true})
	if want := []int64{200, 400, 100}; !slices.Equal(unread, want) {
		t.Fatalf("unread = %v, want %v", unread, want)
	}
	feed := snapshot(t, lib, View{Stream: "feed/1"})
	if want := []int64{400, 100}; !slices.Equal(feed, want) {
		t.Fatalf("feed/1 = %v, want %v", feed, want)
	}
	label := snapshot(t, lib, View{Stream: labelTech})
	if want := []int64{200, 400, 100}; !slices.Equal(label, want) {
		t.Fatalf("label = %v, want %v", label, want)
	}
}

func TestSnapshotUnreadFollowsTheDisplayedState(t *testing.T) {
	db := open(t)
	seedFeeds(t, db)
	seed(t, db,
		article{ID: 100, Stream: "feed/1", Published: 10},
		article{ID: 200, Stream: "feed/1", Published: 20, Read: true},
		article{ID: 300, Stream: "feed/3", Published: 30},
		article{ID: 400, Stream: "feed/3", Published: 40},
	)
	exec(t, db, `INSERT INTO pending_read (item_id, value, seq) VALUES (100, 1, 1), (200, 0, 2)`)
	exec(t, db, `INSERT INTO pending_mark_all (stream_id, ts, seq) VALUES ('feed/3', 300, 3)`)

	got := snapshot(t, New(db.DB), View{Stream: freshrss.StreamReadingList, UnreadOnly: true})
	if want := []int64{400, 200}; !slices.Equal(got, want) {
		t.Fatalf("unread = %v, want %v", got, want)
	}
}

// TestSnapshotHoldsTheWholeViewAndItsNewest returns every member in one
// answer, however long the view, with the newest fetch time for a mark-all.
func TestSnapshotHoldsTheWholeViewAndItsNewest(t *testing.T) {
	db := open(t)
	seedFeeds(t, db)
	exec(t, db, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 6000)
		INSERT INTO articles (item_id, stream_id, published_at) SELECT i * 100, 'feed/1', i * 10 FROM n`)
	var want []int64
	for i := int64(6000); i >= 1; i-- {
		want = append(want, i*100)
	}
	// Fetched last, published first: the newest is not at the top.
	seed(t, db, article{ID: 999_999, Stream: "feed/1", Published: 1})
	want = append(want, 999_999)

	s, err := New(db.DB).Snapshot(context.Background(), View{Stream: "feed/1", UnreadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.IDs, want) || s.Newest != 999_999 {
		t.Fatalf("snapshot: %d ids, newest %d; want %d ids, newest 999999", len(s.IDs), s.Newest, len(want))
	}
	empty, err := New(db.DB).Snapshot(context.Background(), View{Stream: "feed/2"})
	if err != nil || empty.IDs == nil || len(empty.IDs) != 0 || empty.Newest != 0 {
		t.Fatalf("empty view = %+v, %v", empty, err)
	}
}

func TestSnapshotRejectsAnUnknownStream(t *testing.T) {
	if _, err := New(open(t).DB).Snapshot(context.Background(), View{Stream: "state/x"}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("unknown stream: %v", err)
	}
}

func TestCardsKeepTheAskedOrderAndSkipMissingIDs(t *testing.T) {
	db := open(t)
	seedFeeds(t, db)
	seed(t, db,
		article{ID: 100, Stream: "feed/1", URL: "https://b.example/1", Title: "Hello", Published: 10,
			Content: `<p>First&nbsp;<b>bold</b> &amp; more</p><script>alert(1)</script><style>p{}</style><p>second
				paragraph</p>`},
		article{ID: 200, Stream: "feed/9", Title: "中文标题", Published: 20, Read: true},
	)
	exec(t, db, `UPDATE articles SET image_url = 'https://b.example/i.png' WHERE item_id = 100`)
	exec(t, db, `INSERT INTO title_translations (item_id, translated_title) VALUES (100, '你好'), (200, '中文标题')`)
	exec(t, db, `INSERT INTO pending_read (item_id, value, seq) VALUES (100, 1, 1)`)

	cards, err := New(db.DB).Cards(context.Background(), []int64{200, 999, 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 || cards[0].ID != 200 || cards[1].ID != 100 {
		t.Fatalf("cards = %+v", cards)
	}
	c := cards[1]
	want := Card{ID: 100, Stream: "feed/1", FeedTitle: "Beta", URL: "https://b.example/1", Title: "Hello",
		TranslatedTitle: "你好", ImageURL: "https://b.example/i.png", PublishedAt: 10, Read: true,
		Excerpt: "First bold & more second paragraph"}
	if c != want {
		t.Fatalf("card = %+v\nwant   %+v", c, want)
	}
	if o := cards[0]; o.FeedTitle != "" || !o.Read || o.Excerpt != "" || o.TranslatedTitle != "中文标题" {
		t.Fatalf("card without feed or content = %+v", o)
	}
}

func TestCardsRefuseTooManyIDs(t *testing.T) {
	ids := make([]int64, MaxCards+1)
	if _, err := New(open(t).DB).Cards(context.Background(), ids); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestExcerpt(t *testing.T) {
	long := ""
	for range excerptRunes + 50 {
		long += "字"
	}
	tests := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"plain text", "just text", "just text"},
		{"entities and tags", `<div>a &lt;b&gt; <i>c</i></div>`, "a <b> c"},
		{"hidden elements", `<noscript>no</noscript><template>t</template><iframe>f</iframe>x`, "x"},
		{"comments", `<!-- c -->y`, "y"},
		{"broken html", `<p>open <b>never closed`, "open never closed"},
		{"long", long, string([]rune(long)[:excerptRunes])},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Excerpt(tt.in); got != tt.want {
				t.Fatalf("Excerpt(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestContent(t *testing.T) {
	db := open(t)
	seed(t, db,
		article{ID: 100, Stream: "feed/1", Published: 1, Content: "<p>body</p>"},
		article{ID: 200, Stream: "feed/1", Published: 1},
	)
	lib := New(db.DB)
	ctx := context.Background()
	if got, err := lib.Content(ctx, 100); err != nil || got != "<p>body</p>" {
		t.Fatalf("content = %q, %v", got, err)
	}
	if got, err := lib.Content(ctx, 200); err != nil || got != "" {
		t.Fatalf("no body = %q, %v", got, err)
	}
	if _, err := lib.Content(ctx, 300); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing article: %v", err)
	}
}

// TestUnreadCountsFoldTheSameURL counts a URL group once per scope, while
// the snapshot keeps each item (spec D8).
func TestUnreadCountsFoldTheSameURL(t *testing.T) {
	db := open(t)
	seedFeeds(t, db)
	seed(t, db,
		article{ID: 100, Stream: "feed/1", URL: "https://x/1", Published: 1},
		article{ID: 101, Stream: "feed/2", URL: "https://x/1", Published: 1},
		article{ID: 102, Stream: "feed/1", URL: "https://x/1", Published: 1},
		article{ID: 200, Stream: "feed/1", URL: "", Published: 1},
		article{ID: 201, Stream: "feed/1", URL: "", Published: 1},
		article{ID: 300, Stream: "feed/3", URL: "https://x/3", Published: 1},
		article{ID: 301, Stream: "feed/3", URL: "https://x/4", Published: 1, Read: true},
		article{ID: 400, Stream: "feed/9", URL: "https://x/9", Published: 1},
	)
	exec(t, db, `INSERT INTO pending_read (item_id, value, seq) VALUES (301, 0, 1)`)
	exec(t, db, `INSERT INTO pending_mark_all (stream_id, ts, seq) VALUES ('feed/9', 400, 2)`)

	got, err := New(db.DB).UnreadCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// feed/1: x/1 once plus two items without a URL; feed/9 is covered by
	// its mark-all; 301 is unread by intent.
	if got.Total != 5 {
		t.Fatalf("total = %d, want 5 (%+v)", got.Total, got)
	}
	wantFeeds := map[string]int{"feed/1": 3, "feed/2": 1, "feed/3": 2}
	if len(got.Feeds) != len(wantFeeds) {
		t.Fatalf("feeds = %v, want %v", got.Feeds, wantFeeds)
	}
	for k, v := range wantFeeds {
		if got.Feeds[k] != v {
			t.Fatalf("feeds = %v, want %v", got.Feeds, wantFeeds)
		}
	}
	if len(got.Tags) != 1 || got.Tags[labelTech] != 3 {
		t.Fatalf("tags = %v, want %s: 3", got.Tags, labelTech)
	}
}

func TestTree(t *testing.T) {
	db := open(t)
	seedFeeds(t, db)
	got, err := New(db.DB).Tree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 1 {
		t.Fatalf("categories = %+v, want the empty label left out", got.Categories)
	}
	tech := got.Categories[0]
	if tech.ID != labelTech || tech.Label != "Tech" || len(tech.Feeds) != 2 ||
		tech.Feeds[0].ID != "feed/2" || tech.Feeds[1].ID != "feed/1" || tech.Feeds[1].URL != "https://b.example/rss" {
		t.Fatalf("tech = %+v, want alpha then Beta", tech)
	}
	if len(got.Feeds) != 1 || got.Feeds[0].ID != "feed/3" || got.Feeds[0].Title != "Gamma" {
		t.Fatalf("feeds outside a label = %+v", got.Feeds)
	}
}
