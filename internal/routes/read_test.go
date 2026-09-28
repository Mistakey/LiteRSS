package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"LiteRSS/internal/library"
)

func (a *testAPI) send(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	a.handler.ServeHTTP(rec, req)
	return rec
}

// readOf returns the read state the cards show for each id.
func (a *testAPI) readOf(t *testing.T, ids string) map[int64]bool {
	t.Helper()
	var cards []library.Card
	a.getJSON(t, "/api/articles/cards?ids="+ids, &cards)
	got := map[int64]bool{}
	for _, c := range cards {
		got[c.ID] = c.Read
	}
	return got
}

func (a *testAPI) seedFeed(t *testing.T) {
	t.Helper()
	a.exec(t, `INSERT INTO feeds (stream_id, title) VALUES ('feed/1', 'One'), ('feed/2', 'Two')`)
	a.exec(t, `INSERT INTO articles (item_id, stream_id, url, published_at) VALUES
		(100, 'feed/1', 'https://x/1', 1), (200, 'feed/1', 'https://x/2', 2), (300, 'feed/1', 'https://x/3', 3),
		(400, 'feed/2', 'https://x/1', 4)`)
	a.exec(t, `INSERT INTO articles (item_id, stream_id, url, published_at, server_read) VALUES (500, 'feed/1', 'https://x/5', 5, 1)`)
}

func TestSetArticleRead(t *testing.T) {
	api := newTestAPI(t)
	api.seedFeed(t)

	if rec := api.send(t, http.MethodPost, "/api/articles/200/read", `{"read":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("mark read: %d %s", rec.Code, rec.Body)
	}
	if got := api.readOf(t, "200,300"); !got[200] || got[300] {
		t.Fatalf("after marking 200 read: %v", got)
	}
	// Same URL: 100 and 400 are one article.
	if rec := api.send(t, http.MethodPost, "/api/articles/100/read", `{"read":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("mark read: %d %s", rec.Code, rec.Body)
	}
	if got := api.readOf(t, "400"); !got[400] {
		t.Fatalf("copy of 100 stays unread: %v", got)
	}
	if rec := api.send(t, http.MethodPost, "/api/articles/500/read", `{"read":false}`); rec.Code != http.StatusNoContent {
		t.Fatalf("mark unread: %d %s", rec.Code, rec.Body)
	}
	if got := api.readOf(t, "500"); got[500] {
		t.Fatalf("500 still read: %v", got)
	}

	for _, c := range []struct{ target, body string }{
		{"/api/articles/x/read", `{"read":true}`},
		{"/api/articles/200/read", `{}`},
		{"/api/articles/200/read", `{"read":"yes"}`},
		{"/api/articles/200/read", `{"read":true,"extra":1}`},
		{"/api/articles/200/read", `not json`},
	} {
		if rec := api.send(t, http.MethodPost, c.target, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s %s: %d, want 400", c.target, c.body, rec.Code)
		}
	}
}

type batchResponse struct {
	Token string `json:"token"`
	Count int    `json:"count"`
}

func (a *testAPI) batch(t *testing.T, target, body string) batchResponse {
	t.Helper()
	rec := a.send(t, http.MethodPost, target, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s: %d %s", target, rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("POST %s: Cache-Control %q", target, got)
	}
	var b batchResponse
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

// TestMarkItemsReadAndUndo marks a range of the snapshot read, gets a token
// back, and undoes it: the articles show unread again (spec D7).
func TestMarkItemsReadAndUndo(t *testing.T) {
	api := newTestAPI(t)
	api.seedFeed(t)
	api.send(t, http.MethodPost, "/api/articles/300/read", `{"read":true}`)

	// "This and below" from 300: 300 is already read, 100 has a copy in 400.
	b := api.batch(t, "/api/articles/read", `{"ids":[300,200,100]}`)
	if b.Token == "" || b.Count != 3 {
		t.Fatalf("batch = %+v, want a token and 3 changed (200, 100, 400)", b)
	}
	if got := api.readOf(t, "100,200,300,400"); !got[100] || !got[200] || !got[300] || !got[400] {
		t.Fatalf("after the batch: %v", got)
	}

	if rec := api.send(t, http.MethodPost, "/api/undo", `{"token":"`+b.Token+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body)
	}
	// 300 was read before the batch and stays read.
	if got := api.readOf(t, "100,200,300,400"); got[100] || got[200] || !got[300] || got[400] {
		t.Fatalf("after undo: %v", got)
	}
	if rec := api.send(t, http.MethodPost, "/api/undo", `{"token":"`+b.Token+`"}`); rec.Code != http.StatusGone {
		t.Fatalf("second undo: %d, want 410", rec.Code)
	}

	empty := api.batch(t, "/api/articles/read", `{"ids":[]}`)
	if empty.Count != 0 {
		t.Fatalf("empty batch = %+v", empty)
	}
	for _, body := range []string{`{}`, `{"ids":["1"]}`, `{"ids":[1],"x":1}`} {
		if rec := api.send(t, http.MethodPost, "/api/articles/read", body); rec.Code != http.StatusBadRequest {
			t.Errorf("POST articles/read %s: %d, want 400", body, rec.Code)
		}
	}
	for _, body := range []string{`{}`, `{"token":""}`} {
		if rec := api.send(t, http.MethodPost, "/api/undo", body); rec.Code != http.StatusBadRequest {
			t.Errorf("POST undo %s: %d, want 400", body, rec.Code)
		}
	}
	if rec := api.send(t, http.MethodPost, "/api/undo", `{"token":"nope"}`); rec.Code != http.StatusGone {
		t.Errorf("unknown token: %d, want 410", rec.Code)
	}
}

// TestMarkStreamReadAndUndo marks a feed read up to the snapshot's newest,
// then a label with its feeds, and undoes the first.
func TestMarkStreamReadAndUndo(t *testing.T) {
	api := newTestAPI(t)
	api.seedFeed(t)
	api.exec(t, `INSERT INTO tags (tag_id, label) VALUES ('user/-/label/News', 'News')`)
	api.exec(t, `INSERT INTO feed_tags (stream_id, tag_id) VALUES ('feed/2', 'user/-/label/News')`)

	// ts 200: 300 was fetched after the snapshot and stays unread.
	b := api.batch(t, "/api/streams/read", `{"stream":"feed/1","ts":200}`)
	if b.Token == "" || b.Count != 3 {
		t.Fatalf("feed batch = %+v, want 100, 200 and the copy 400", b)
	}
	if got := api.readOf(t, "100,200,300,400"); !got[100] || !got[200] || got[300] || !got[400] {
		t.Fatalf("after feed/1 up to 200: %v", got)
	}
	if rec := api.send(t, http.MethodPost, "/api/undo", `{"token":"`+b.Token+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body)
	}
	if got := api.readOf(t, "100,200,300,400"); got[100] || got[200] || got[300] || got[400] {
		t.Fatalf("after undo: %v", got)
	}

	// A label covers its feeds; no ts takes the newest local item.
	lb := api.batch(t, "/api/streams/read", `{"stream":"user/-/label/News"}`)
	if lb.Count != 2 {
		t.Fatalf("label batch = %+v, want 400 and its copy 100", lb)
	}
	if got := api.readOf(t, "100,200,400"); !got[100] || got[200] || !got[400] {
		t.Fatalf("after the label: %v", got)
	}

	for _, body := range []string{`{}`, `{"stream":"user/-/state/com.google/starred"}`, `{"stream":"feed/1","ts":-1}`} {
		if rec := api.send(t, http.MethodPost, "/api/streams/read", body); rec.Code != http.StatusBadRequest {
			t.Errorf("POST streams/read %s: %d, want 400", body, rec.Code)
		}
	}
}
