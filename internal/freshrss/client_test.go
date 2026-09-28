package freshrss

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"LiteRSS/internal/freshrss/freshrsstest"
)

// newFake mounts a fake FreshRSS and returns it with a client logged into it.
func newFake(t *testing.T) (*freshrsstest.Server, *Client) {
	t.Helper()
	fake := freshrsstest.New("user", "secret")
	fake.AddFeeds(
		freshrsstest.Feed{ID: 1, Title: "One", URL: "https://one.example/rss", Labels: []string{"Tech"}},
		freshrsstest.Feed{ID: 2, Title: "Two", URL: "https://two.example/rss", Labels: []string{"News"}},
	)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, NewClient(srv.URL, "user", "secret")
}

// addItems adds items 1..n (ids 1000, 2000, ...) to feed 1.
func addItems(fake *freshrsstest.Server, n int) {
	for i := 1; i <= n; i++ {
		fake.AddItems(freshrsstest.Item{ID: int64(i) * 1000, FeedID: 1, Title: "t"})
	}
}

func ids(t *testing.T, c *Client, q ItemIDQuery) ItemIDPage {
	t.Helper()
	page, err := c.StreamItemIDs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestStreamItemIDsIsDecimalAndCountHasNoUpperBound(t *testing.T) {
	fake, c := newFake(t)
	addItems(fake, 1500)

	page := ids(t, c, ItemIDQuery{Stream: StreamReadingList, Count: 100000})
	if len(page.IDs) != 1500 || page.Continuation != "" {
		t.Fatalf("got %d ids, continuation %q; want all 1500 in one page", len(page.IDs), page.Continuation)
	}
	if page.IDs[0] != 1500000 || page.IDs[1499] != 1000 {
		t.Fatalf("want newest first, got %d..%d", page.IDs[0], page.IDs[1499])
	}
}

func TestContinuationIsInclusiveAndDropsFirst(t *testing.T) {
	fake, c := newFake(t)
	addItems(fake, 5)

	var pages [][]int64
	q := ItemIDQuery{Stream: StreamReadingList, Count: 2}
	for {
		page := ids(t, c, q)
		pages = append(pages, page.IDs)
		if page.Continuation == "" {
			break
		}
		q.Continuation = page.Continuation
	}
	want := [][]int64{{5000, 4000}, {3000, 2000}, {1000}}
	if !slices.EqualFunc(pages, want, slices.Equal) {
		t.Fatalf("pages = %v, want %v", pages, want)
	}

	// The boundary entry is read elsewhere between pages: the server still drops
	// the first result, so an unread entry is skipped (read-retention.md, 新发现 1).
	unread := ItemIDQuery{Stream: StreamReadingList, Exclude: StreamRead, Count: 2}
	first := ids(t, c, unread)
	if !slices.Equal(first.IDs, []int64{5000, 4000}) || first.Continuation != "4000" {
		t.Fatalf("first page = %+v", first)
	}
	fake.SetRead(4000, true)
	unread.Continuation = first.Continuation
	second := ids(t, c, unread)
	if !slices.Equal(second.IDs, []int64{2000, 1000}) {
		t.Fatalf("second page = %v, want 3000 lost to the dropped first result", second.IDs)
	}
}

func TestStreamItemIDsFilters(t *testing.T) {
	fake, c := newFake(t)
	fake.AddItems(
		freshrsstest.Item{ID: 10, FeedID: 1},
		freshrsstest.Item{ID: 20, FeedID: 1, Read: true},
		freshrsstest.Item{ID: 30, FeedID: 2, Starred: true},
		freshrsstest.Item{ID: 40, FeedID: 2, Read: true},
	)
	cases := []struct {
		name string
		q    ItemIDQuery
		want []int64
	}{
		{"unread via xt", ItemIDQuery{Stream: StreamReadingList, Exclude: StreamRead}, []int64{30, 10}},
		{"read via it", ItemIDQuery{Stream: StreamReadingList, Include: StreamRead}, []int64{40, 20}},
		{"read as stream", ItemIDQuery{Stream: StreamRead}, []int64{40, 20}},
		{"starred", ItemIDQuery{Stream: StreamStarred}, []int64{30}},
		{"feed", ItemIDQuery{Stream: "feed/1"}, []int64{20, 10}},
		{"label unread", ItemIDQuery{Stream: LabelPrefix + "News", Exclude: StreamRead}, []int64{30}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.q.Count = 1000
			if got := ids(t, c, tc.q).IDs; !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStreamItemContentsParsesLongFormIDs(t *testing.T) {
	fake, c := newFake(t)
	id := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC).UnixMicro()
	published := time.Date(2026, 9, 1, 7, 30, 0, 0, time.UTC)
	fake.AddItems(freshrsstest.Item{
		ID: id, FeedID: 2, Title: "Hello", URL: "https://two.example/p/1",
		Content: "<p>body</p>", Author: "a", Published: published, Read: true,
	})

	items, err := c.StreamItemContents(context.Background(), []int64{id, 42})
	if err != nil {
		t.Fatal(err)
	}
	want := Item{
		ID: id, StreamID: "feed/2", Title: "Hello", URL: "https://two.example/p/1",
		Author: "a", Content: "<p>body</p>", Published: published.Unix(), Read: true,
	}
	if len(items) != 1 || items[0] != want {
		t.Fatalf("items = %+v, want [%+v]", items, want)
	}
}

func TestParseItemID(t *testing.T) {
	for in, want := range map[string]int64{
		"1758960000000000": 1758960000000000,
		"tag:google.com,2005:reader/item/00063f9c4a1b2c00": 0x63f9c4a1b2c00,
	} {
		if got, err := ParseItemID(in); err != nil || got != want {
			t.Errorf("ParseItemID(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "0", "-5", "https://example.com/a", "tag:google.com,2005:reader/item/zz"} {
		if _, err := ParseItemID(bad); err == nil {
			t.Errorf("ParseItemID(%q) succeeded", bad)
		}
	}
}

func TestEditTagChangesStateAndIsOKForUnknownItems(t *testing.T) {
	fake, c := newFake(t)
	fake.AddItems(freshrsstest.Item{ID: 10, FeedID: 1}, freshrsstest.Item{ID: 20, FeedID: 1, Read: true})
	ctx := context.Background()

	if err := c.MarkAsRead(ctx, []int64{10, 999}); err != nil {
		t.Fatalf("MarkAsRead with an unknown id: %v", err)
	}
	if err := c.MarkAsUnread(ctx, []int64{20}); err != nil {
		t.Fatal(err)
	}
	if it, _ := fake.Item(10); !it.Read {
		t.Fatal("item 10 not read")
	}
	if it, _ := fake.Item(20); it.Read {
		t.Fatal("item 20 still read")
	}
}

func TestMarkAllAsReadCutsOffAtItemID(t *testing.T) {
	cases := []struct {
		stream string
		want   map[int64]bool // item id -> read afterwards
	}{
		{"feed/1", map[int64]bool{100: true, 200: true, 300: false, 150: false}},
		{LabelPrefix + "News", map[int64]bool{100: false, 200: false, 300: false, 150: true}},
		{StreamReadingList, map[int64]bool{100: true, 200: true, 300: false, 150: true}},
	}
	for _, tc := range cases {
		t.Run(tc.stream, func(t *testing.T) {
			fake, c := newFake(t)
			fake.AddItems(
				freshrsstest.Item{ID: 100, FeedID: 1},
				freshrsstest.Item{ID: 200, FeedID: 1},
				freshrsstest.Item{ID: 300, FeedID: 1},
				freshrsstest.Item{ID: 150, FeedID: 2},
			)
			if err := c.MarkAllAsRead(context.Background(), tc.stream, 200); err != nil {
				t.Fatal(err)
			}
			for id, want := range tc.want {
				if it, _ := fake.Item(id); it.Read != want {
					t.Errorf("item %d read = %v, want %v", id, it.Read, want)
				}
			}
		})
	}
}

func TestMarkAllAsReadRefusesZeroTS(t *testing.T) {
	fake, c := newFake(t)
	fake.AddItems(freshrsstest.Item{ID: 100, FeedID: 1})
	if err := c.MarkAllAsRead(context.Background(), StreamReadingList, 0); err == nil {
		t.Fatal("ts 0 accepted; the server would read it as now")
	}
	if len(fake.MarkAlls()) != 0 {
		t.Fatal("request reached the server")
	}
}

func TestExpiredSessionLogsInAgain(t *testing.T) {
	fake, c := newFake(t)
	fake.AddItems(freshrsstest.Item{ID: 10, FeedID: 1})
	ctx := context.Background()

	if _, err := c.GetSubscriptions(ctx); err != nil {
		t.Fatal(err)
	}
	fake.ExpireSessions()
	subs, err := c.GetSubscriptions(ctx)
	if err != nil || len(subs) != 2 {
		t.Fatalf("after expiry: %v, %d subscriptions", err, len(subs))
	}
	fake.ExpireSessions()
	if err := c.MarkAsRead(ctx, []int64{10}); err != nil {
		t.Fatalf("write after expiry: %v", err)
	}
	if it, _ := fake.Item(10); !it.Read {
		t.Fatal("write after expiry did not land")
	}
	if got := fake.Logins(); got != 3 {
		t.Fatalf("logins = %d, want one per session", got)
	}
}

func TestWrongPasswordIsAnAPIErrorWithoutRetryLoop(t *testing.T) {
	fake, _ := newFake(t)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL+"/", "user", "wrong")

	_, err := c.GetSubscriptions(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized || apiErr.RejectsItem() {
		t.Fatalf("err = %v, want a 401 APIError that blames no item", err)
	}
}

func TestClientsReuseOneSessionPerConfiguration(t *testing.T) {
	fake := freshrsstest.New("user", "secret")
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ctx := context.Background()

	var pool Clients
	a := pool.For(srv.URL, "user", "secret")
	for range 3 {
		if _, err := pool.For(srv.URL, "user", "secret").GetCategories(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if pool.For(srv.URL, "user", "secret") != a || fake.Logins() != 1 {
		t.Fatalf("same configuration: logins = %d, want one shared client and one login", fake.Logins())
	}
	if pool.For(srv.URL, "user", "other") == a {
		t.Fatal("changed password reused the old client")
	}
}
