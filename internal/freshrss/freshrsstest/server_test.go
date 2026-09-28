package freshrsstest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// session logs in over raw HTTP and returns the auth and write tokens.
func session(t *testing.T, base string) (string, string) {
	t.Helper()
	resp, err := http.PostForm(base+APIPrefix+"/accounts/ClientLogin", url.Values{"Email": {"u"}, "Passwd": {"p"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var auth string
	for _, line := range strings.Split(string(body), "\n") {
		if v, ok := strings.CutPrefix(line, "Auth="); ok {
			auth = v
		}
	}
	if auth == "" {
		t.Fatalf("no Auth in %q", body)
	}
	status, write := do(t, http.MethodGet, base+APIPrefix+"/reader/api/0/token", auth, nil)
	if status != http.StatusOK || !strings.HasSuffix(write, "\n") {
		t.Fatalf("token: %d %q", status, write)
	}
	return auth, strings.TrimSpace(write)
}

func do(t *testing.T, method, target, auth string, form url.Values) (int, string) {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, _ := http.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", "GoogleLogin auth="+auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func newFake(t *testing.T) (*Server, string) {
	t.Helper()
	fake := New("u", "p")
	fake.AddFeeds(Feed{ID: 1, Title: "One", Labels: []string{"Tech"}}, Feed{ID: 2, Title: "Two"})
	fake.AddItems(
		Item{ID: 100, FeedID: 1, Title: "a"},
		Item{ID: 200, FeedID: 1, Title: "b", Read: true},
		Item{ID: 300, FeedID: 2, Title: "c"},
	)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv.URL
}

func TestItemContentsAcceptsBothIDFormatsAndAnswersLongForm(t *testing.T) {
	_, base := newFake(t)
	auth, _ := session(t, base)

	form := url.Values{"i": {"100", LongIDPrefix + "000000000000012c", "999"}}
	status, body := do(t, http.MethodPost, base+APIPrefix+"/reader/api/0/stream/items/contents", auth, form)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	var got struct {
		Items []struct {
			ID     string `json:"id"`
			Origin struct {
				StreamID string `json:"streamId"`
			} `json:"origin"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	want := []string{LongIDPrefix + "000000000000012c", LongIDPrefix + "0000000000000064"}
	if len(got.Items) != 2 || got.Items[0].ID != want[0] || got.Items[1].ID != want[1] {
		t.Fatalf("items = %+v, want ids %v (newest first, unknown id dropped)", got.Items, want)
	}
	if got.Items[0].Origin.StreamID != "feed/2" {
		t.Fatalf("origin = %q", got.Items[0].Origin.StreamID)
	}
}

func TestEditTagAnswersOKForAnyItem(t *testing.T) {
	fake, base := newFake(t)
	auth, write := session(t, base)

	form := url.Values{"T": {write}, "a": {StreamRead}, "i": {"https://example.com/post", "bogus", "999"}}
	status, body := do(t, http.MethodPost, base+APIPrefix+"/reader/api/0/edit-tag", auth, form)
	if status != http.StatusOK || body != "OK" {
		t.Fatalf("edit-tag = %d %q, want 200 OK", status, body)
	}
	for _, it := range fake.Items() {
		if it.ID != 200 && it.Read {
			t.Fatalf("item %d changed by an edit-tag naming no real item", it.ID)
		}
	}
	if calls := fake.EditTags(); len(calls) != 1 || calls[0].IDs[0] != 0 || calls[0].IDs[1] != 0 || calls[0].IDs[2] != 999 {
		t.Fatalf("recorded %+v, want unreadable ids parsed to 0", calls)
	}
}

func TestWritesNeedWriteToken(t *testing.T) {
	_, base := newFake(t)
	auth, _ := session(t, base)
	form := url.Values{"T": {"stale"}, "a": {StreamRead}, "i": {"100"}}
	if status, _ := do(t, http.MethodPost, base+APIPrefix+"/reader/api/0/edit-tag", auth, form); status != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", status)
	}
}

func TestMarkAllAsReadRejectsNonDigitTS(t *testing.T) {
	_, base := newFake(t)
	auth, write := session(t, base)
	form := url.Values{"T": {write}, "s": {StreamReadingList}, "ts": {"2026-01-01"}}
	if status, _ := do(t, http.MethodPost, base+APIPrefix+"/reader/api/0/mark-all-as-read", auth, form); status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", status)
	}
}

func TestExpiredSessionIsUnauthorized(t *testing.T) {
	fake, base := newFake(t)
	auth, _ := session(t, base)
	fake.ExpireSessions()

	req, _ := http.NewRequest(http.MethodGet, base+APIPrefix+"/reader/api/0/subscription/list?output=json", nil)
	req.Header.Set("Authorization", "GoogleLogin auth="+auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("Google-Bad-Token") != "true" {
		t.Fatalf("status %d header %q, want 401 with Google-Bad-Token", resp.StatusCode, resp.Header.Get("Google-Bad-Token"))
	}
}

func TestGenerateIsReproducibleWithUniqueIDs(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	feeds, items := Generate(7, 4, 50, now)
	_, again := Generate(7, 4, 50, now)
	if len(feeds) != 4 || len(items) != 200 {
		t.Fatalf("got %d feeds, %d items", len(feeds), len(items))
	}
	seen := map[int64]bool{}
	oldest := now.UnixMicro()
	for i, it := range items {
		if seen[it.ID] {
			t.Fatalf("duplicate id %d", it.ID)
		}
		seen[it.ID] = true
		if it != again[i] {
			t.Fatalf("item %d differs between runs", i)
		}
		oldest = min(oldest, it.ID)
	}
	if now.Sub(time.UnixMicro(oldest)) < 90*24*time.Hour {
		t.Fatalf("oldest item is younger than the 90-day retention; want both sides covered")
	}
}
