// Command fake-freshrss serves internal/freshrss/freshrsstest on a loopback
// port with a generated account, for a development instance to sync against.
// It never talks to a real FreshRSS. Usage is in docs/TESTING.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"LiteRSS/internal/freshrss/freshrsstest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:1240", "loopback address to listen on")
	user := flag.String("user", "dev", "account user name")
	pass := flag.String("pass", "dev", "account password")
	feeds := flag.Int("feeds", 6, "number of generated feeds")
	items := flag.Int("items", 40, "generated items per feed")
	seed := flag.Int64("seed", 1, "generator seed; the same seed gives the same titles and ids relative to now")
	flag.Parse()

	if err := checkLoopback(*addr); err != nil {
		log.Fatal(err)
	}

	fake := freshrsstest.New(*user, *pass)
	generatedFeeds, generatedItems := freshrsstest.Generate(*seed, *feeds, *items, time.Now())
	fake.AddFeeds(generatedFeeds...)
	fake.AddItems(generatedItems...)

	mux := http.NewServeMux()
	mux.Handle(freshrsstest.APIPrefix+"/", fake)
	control(mux, fake)

	log.Printf("fake FreshRSS on http://%s (API %s), user %q, %d feeds, %d items",
		*addr, freshrsstest.APIPrefix, *user, len(generatedFeeds), len(generatedItems))
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errors.New("-addr must be a loopback IP such as 127.0.0.1")
	}
	return nil
}

// control mounts POST endpoints under /_fake/ that play what other devices and
// the server do between syncs.
func control(mux *http.ServeMux, fake *freshrsstest.Server) {
	// Crawl n new unread items into a feed: /_fake/add?feed=1&n=3
	mux.HandleFunc("POST /_fake/add", func(w http.ResponseWriter, r *http.Request) {
		feed, _ := strconv.Atoi(r.URL.Query().Get("feed"))
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if feed <= 0 {
			feed = 1
		}
		n = max(n, 1)
		for range n {
			id := fake.NextID()
			fake.AddItems(freshrsstest.Item{
				ID:        id,
				FeedID:    feed,
				Title:     fmt.Sprintf("New item %d", id),
				URL:       fmt.Sprintf("https://feed%d.example.com/new/%d", feed, id),
				Content:   "<p>Freshly crawled.</p>",
				Published: time.Now(),
			})
			_, _ = fmt.Fprintln(w, id)
		}
	})
	// Read or unread an item elsewhere: /_fake/read?i=<id>&read=0
	mux.HandleFunc("POST /_fake/read", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.URL.Query().Get("i"), 10, 64)
		if !fake.SetRead(id, r.URL.Query().Get("read") != "0") {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintln(w, "OK")
	})
	// Unsubscribe a feed; its items go with it: /_fake/unsubscribe?feed=1
	mux.HandleFunc("POST /_fake/unsubscribe", func(w http.ResponseWriter, r *http.Request) {
		feed, _ := strconv.Atoi(r.URL.Query().Get("feed"))
		fake.RemoveFeed(feed)
		_, _ = fmt.Fprintln(w, "OK")
	})
	// Expire every session; the next API call gets 401.
	mux.HandleFunc("POST /_fake/expire", func(w http.ResponseWriter, _ *http.Request) {
		fake.ExpireSessions()
		_, _ = fmt.Fprintln(w, "OK")
	})
}
