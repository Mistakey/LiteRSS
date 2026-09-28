// Package freshrsstest is a stand-in for the FreshRSS Google Reader API
// (p/api/greader.php). It is the test seam for sync (spec D6): Go tests mount it
// on httptest, and tools/fake-freshrss serves it to a development instance.
//
// It reproduces the server behaviour sync depends on, including the parts that
// surprise a client: item ids come back in two formats, n has no upper bound,
// a continuation is an inclusive bound whose first result is always dropped,
// edit-tag answers OK for any i, mark-all-as-read cuts off at ts compared with
// item ids, and an expired session is answered with 401. Anything beyond that
// (unread counts, stream/contents, subscription editing) is deliberately absent.
package freshrsstest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Stream ids and item id format, as greader.php spells them.
const (
	StreamReadingList = "user/-/state/com.google/reading-list"
	StreamRead        = "user/-/state/com.google/read"
	StreamStarred     = "user/-/state/com.google/starred"
	LabelPrefix       = "user/-/label/"
	FeedPrefix        = "feed/"
	LongIDPrefix      = "tag:google.com,2005:reader/item/"
)

// APIPrefix is the path the Google Reader API lives under.
const APIPrefix = "/api/greader.php"

// Feed is one subscription. Its stream id is "feed/<ID>", as in FreshRSS.
type Feed struct {
	ID      int
	Title   string
	URL     string
	HTMLURL string
	Labels  []string
}

// StreamID returns the feed's stream id.
func (f Feed) StreamID() string { return FeedPrefix + strconv.Itoa(f.ID) }

// Item is one entry. ID is the crawl time in microseconds, which is how
// FreshRSS mints entry ids; items sharing a URL are distinct entries.
type Item struct {
	ID        int64
	FeedID    int
	Title     string
	URL       string
	Author    string
	Content   string
	Published time.Time
	Read      bool
	Starred   bool
}

// EditTag records one edit-tag request as the server parsed it: an i the
// server cannot read shows up as id 0, just as FreshRSS turns it into 0.
type EditTag struct {
	Add    string
	Remove string
	IDs    []int64
}

// MarkAll records one mark-all-as-read request.
type MarkAll struct {
	Stream string
	TS     int64
}

// Server is the fake. Its zero value is not usable; call New.
type Server struct {
	mu       sync.Mutex
	username string
	password string
	feeds    map[int]Feed
	items    map[int64]*Item
	// sessions maps an auth token to its write token.
	sessions map[string]string
	seq      int
	logins   int
	editTags []EditTag
	markAlls []MarkAll
	rejected map[int64]bool
	now      func() time.Time
}

// New returns an empty server that accepts the given account.
func New(username, password string) *Server {
	return &Server{
		username: username,
		password: password,
		feeds:    map[int]Feed{},
		items:    map[int64]*Item{},
		sessions: map[string]string{},
		rejected: map[int64]bool{},
		now:      time.Now,
	}
}

// AddFeeds adds or replaces subscriptions.
func (s *Server) AddFeeds(feeds ...Feed) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range feeds {
		s.feeds[f.ID] = f
	}
}

// RemoveFeed unsubscribes a feed. FreshRSS deletes its entries with it.
func (s *Server) RemoveFeed(feedID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.feeds, feedID)
	for id, it := range s.items {
		if it.FeedID == feedID {
			delete(s.items, id)
		}
	}
}

// AddItems adds or replaces entries.
func (s *Server) AddItems(items ...Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range items {
		it := it
		s.items[it.ID] = &it
	}
}

// NextID returns an unused entry id no earlier than now, the way a fresh crawl
// would mint one.
func (s *Server) NextID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.now().UnixMicro()
	for _, it := range s.items {
		if it.ID >= id {
			id = it.ID + 1
		}
	}
	return id
}

// Item returns a copy of one entry.
func (s *Server) Item(id int64) (Item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok {
		return Item{}, false
	}
	return *it, true
}

// Items returns copies of all entries, newest first.
func (s *Server) Items() []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Item, 0, len(s.items))
	for _, it := range s.items {
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// SetRead changes an entry's read state as another device would.
func (s *Server) SetRead(id int64, read bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if ok {
		it.Read = read
	}
	return ok
}

// ExpireSessions invalidates every auth and write token; the next request
// carrying one is answered with 401.
func (s *Server) ExpireSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = map[string]string{}
}

// Logins counts successful ClientLogin calls.
func (s *Server) Logins() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logins
}

// EditTags returns every accepted edit-tag request in arrival order.
func (s *Server) EditTags() []EditTag {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]EditTag(nil), s.editTags...)
}

// MarkAlls returns every accepted mark-all-as-read request in arrival order.
func (s *Server) MarkAlls() []MarkAll {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]MarkAll(nil), s.markAlls...)
}

// RejectItem is fault injection, not FreshRSS behaviour: every later edit-tag
// naming this id is answered with 400 and changes nothing, so a caller can
// exercise its handling of a refused batch.
func (s *Server) RejectItem(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejected[id] = true
}

// ServeHTTP answers the Google Reader API under APIPrefix.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Routed by hand, not http.ServeMux: stream ids are not path-clean.
	path, ok := strings.CutPrefix(r.URL.Path, APIPrefix)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if path == "/accounts/ClientLogin" {
		s.clientLogin(w, r)
		return
	}
	writeToken, ok := s.session(r)
	if !ok {
		unauthorized(w)
		return
	}
	switch path {
	case "/reader/api/0/token":
		// FreshRSS ends the token with a newline.
		_, _ = fmt.Fprint(w, writeToken, "\n")
	case "/reader/api/0/subscription/list":
		s.subscriptionList(w)
	case "/reader/api/0/tag/list":
		s.tagList(w)
	case "/reader/api/0/stream/items/ids":
		s.itemIDs(w, r)
	case "/reader/api/0/stream/items/contents":
		s.itemContents(w, r)
	case "/reader/api/0/edit-tag":
		if !s.checkWrite(w, r, writeToken) {
			return
		}
		s.editTag(w, r)
	case "/reader/api/0/mark-all-as-read":
		if !s.checkWrite(w, r, writeToken) {
			return
		}
		s.markAllAsRead(w, r)
	default:
		http.NotFound(w, r)
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Header().Set("Google-Bad-Token", "true")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("Unauthorized!"))
}

func badRequest(w http.ResponseWriter) {
	http.Error(w, "Bad Request!", http.StatusBadRequest)
}

func (s *Server) clientLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Form.Get("Email") != s.username || r.Form.Get("Passwd") != s.password {
		unauthorized(w)
		return
	}
	s.seq++
	s.logins++
	auth := fmt.Sprintf("%s/%040x", s.username, s.seq)
	s.sessions[auth] = fmt.Sprintf("%056xZ", s.seq)
	_, _ = fmt.Fprintf(w, "SID=%s\nLSID=null\nAuth=%s\n", auth, auth)
}

func (s *Server) session(r *http.Request) (string, bool) {
	auth, ok := strings.CutPrefix(r.Header.Get("Authorization"), "GoogleLogin auth=")
	if !ok {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	write, ok := s.sessions[auth]
	return write, ok
}

// checkWrite enforces POST and the write token T, as checkToken does.
func (s *Server) checkWrite(w http.ResponseWriter, r *http.Request, writeToken string) bool {
	if r.Method != http.MethodPost {
		badRequest(w)
		return false
	}
	_ = r.ParseForm()
	if r.PostForm.Get("T") != writeToken {
		unauthorized(w)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) subscriptionList(w http.ResponseWriter) {
	s.mu.Lock()
	feeds := s.sortedFeeds()
	s.mu.Unlock()

	type category struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	type subscription struct {
		ID         string     `json:"id"`
		Title      string     `json:"title"`
		Categories []category `json:"categories"`
		URL        string     `json:"url"`
		HTMLURL    string     `json:"htmlUrl"`
		IconURL    string     `json:"iconUrl"`
	}
	subs := make([]subscription, 0, len(feeds))
	for _, f := range feeds {
		cats := make([]category, 0, len(f.Labels))
		for _, l := range f.Labels {
			cats = append(cats, category{ID: LabelPrefix + l, Label: l})
		}
		subs = append(subs, subscription{ID: f.StreamID(), Title: f.Title, Categories: cats, URL: f.URL, HTMLURL: f.HTMLURL})
	}
	writeJSON(w, map[string]any{"subscriptions": subs})
}

func (s *Server) tagList(w http.ResponseWriter) {
	s.mu.Lock()
	seen := map[string]bool{}
	for _, f := range s.feeds {
		for _, l := range f.Labels {
			seen[l] = true
		}
	}
	s.mu.Unlock()

	labels := make([]string, 0, len(seen))
	for l := range seen {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	tags := []map[string]string{{"id": StreamStarred}}
	for _, l := range labels {
		tags = append(tags, map[string]string{"id": LabelPrefix + l, "type": "folder"})
	}
	writeJSON(w, map[string]any{"tags": tags})
}

func (s *Server) sortedFeeds() []Feed {
	feeds := make([]Feed, 0, len(s.feeds))
	for _, f := range s.feeds {
		feeds = append(feeds, f)
	}
	sort.Slice(feeds, func(i, j int) bool { return feeds[i].ID < feeds[j].ID })
	return feeds
}

// inStream reports whether an entry belongs to a stream id; used for s, it
// and xt alike. Callers hold s.mu.
func (s *Server) inStream(it *Item, stream string) bool {
	feed, subscribed := s.feeds[it.FeedID]
	if !subscribed {
		return false
	}
	switch {
	case stream == StreamReadingList:
		return true
	case stream == StreamRead:
		return it.Read
	case stream == StreamStarred:
		return it.Starred
	case strings.HasPrefix(stream, FeedPrefix):
		return strings.TrimPrefix(stream, FeedPrefix) == strconv.Itoa(it.FeedID)
	case strings.HasPrefix(stream, LabelPrefix):
		label := strings.TrimPrefix(stream, LabelPrefix)
		for _, l := range feed.Labels {
			if l == label {
				return true
			}
		}
	}
	return false
}

// itemIDs is stream/items/ids. n defaults to 20 and has no upper bound. A
// continuation c is the last id of the previous page, used as an inclusive
// bound: the server fetches one extra entry and drops the first result
// unconditionally, so an entry that stopped matching between pages costs the
// caller one entry it never sees.
func (s *Server) itemIDs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	stream, include, exclude := q.Get("s"), q.Get("it"), q.Get("xt")
	count := 20
	if n := q.Get("n"); n != "" {
		count, _ = strconv.Atoi(n) // PHP (int) cast: garbage reads as 0
	}
	ascending := q.Get("r") == "o"
	continuation := q.Get("c")
	continued := continuation != "" && continuation != "0"
	var bound int64
	if continued {
		bound, _ = strconv.ParseInt(continuation, 10, 64)
		count++
	}

	s.mu.Lock()
	ids := make([]int64, 0)
	for _, it := range s.items {
		if !s.inStream(it, stream) ||
			(include != "" && !s.inStream(it, include)) ||
			(exclude != "" && s.inStream(it, exclude)) {
			continue
		}
		if continued && ((!ascending && it.ID > bound) || (ascending && it.ID < bound)) {
			continue
		}
		ids = append(ids, it.ID)
	}
	s.mu.Unlock()

	sort.Slice(ids, func(i, j int) bool {
		if ascending {
			return ids[i] < ids[j]
		}
		return ids[i] > ids[j]
	})
	if count < 0 {
		count = 0
	}
	if len(ids) > count {
		ids = ids[:count]
	}
	if continued {
		if len(ids) > 0 {
			ids = ids[1:]
		}
		count--
	}

	refs := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, map[string]string{"id": strconv.FormatInt(id, 10)})
	}
	response := map[string]any{"itemRefs": refs}
	if len(ids) > 0 && len(ids) >= count {
		response["continuation"] = strconv.FormatInt(ids[len(ids)-1], 10)
	}
	writeJSON(w, response)
}

// parseEntryID reads an i the way greader.php does: a decimal without a
// leading zero is taken as is; anything else is the hex basename of the long
// form, and what hex cannot read becomes 0.
func parseEntryID(s string) int64 {
	if isDigits(s) && s[0] != '0' {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	base := s[strings.LastIndex(s, "/")+1:]
	v, err := strconv.ParseUint(base, 16, 63)
	if err != nil {
		return 0
	}
	return int64(v)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (s *Server) itemContents(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.mu.Lock()
	var found []*Item
	seen := map[int64]bool{}
	for _, raw := range r.Form["i"] {
		id := parseEntryID(raw)
		if it, ok := s.items[id]; ok && !seen[id] && s.inStream(it, StreamReadingList) {
			seen[id] = true
			found = append(found, it)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ID > found[j].ID })
	items := make([]map[string]any, 0, len(found))
	for _, it := range found {
		items = append(items, s.toGReader(it))
	}
	s.mu.Unlock()

	writeJSON(w, map[string]any{
		"id":      StreamReadingList,
		"updated": s.now().Unix(),
		"items":   items,
	})
}

// toGReader renders an entry as greader.php does, with the long-form id.
// Callers hold s.mu.
func (s *Server) toGReader(it *Item) map[string]any {
	feed := s.feeds[it.FeedID]
	categories := []string{StreamReadingList}
	for _, l := range feed.Labels {
		categories = append(categories, LabelPrefix+l)
	}
	if it.Read {
		categories = append(categories, StreamRead)
	}
	if it.Starred {
		categories = append(categories, StreamStarred)
	}
	var published int64
	if !it.Published.IsZero() {
		published = it.Published.Unix()
	}
	return map[string]any{
		"id":            fmt.Sprintf("%s%016x", LongIDPrefix, it.ID),
		"crawlTimeMsec": strconv.FormatInt(it.ID/1000, 10),
		"timestampUsec": strconv.FormatInt(it.ID, 10),
		"published":     published,
		"title":         it.Title,
		"canonical":     []map[string]string{{"href": it.URL}},
		"alternate":     []map[string]string{{"href": it.URL}},
		"categories":    categories,
		"origin":        map[string]string{"streamId": feed.StreamID(), "title": feed.Title, "htmlUrl": feed.HTMLURL},
		"summary":       map[string]string{"content": it.Content},
		"author":        it.Author,
	}
}

// editTag answers OK whatever i names: an id the server cannot read, or one it
// does not have, changes nothing and is not reported.
func (s *Server) editTag(w http.ResponseWriter, r *http.Request) {
	add, remove := r.PostForm.Get("a"), r.PostForm.Get("r")
	ids := make([]int64, 0, len(r.PostForm["i"]))
	for _, raw := range r.PostForm["i"] {
		ids = append(ids, parseEntryID(raw))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if s.rejected[id] {
			http.Error(w, "rejected by fault injection", http.StatusBadRequest)
			return
		}
	}
	s.editTags = append(s.editTags, EditTag{Add: add, Remove: remove, IDs: ids})
	for _, id := range ids {
		it, ok := s.items[id]
		if !ok {
			continue
		}
		switch add {
		case StreamRead:
			it.Read = true
		case StreamStarred:
			it.Starred = true
		}
		switch remove {
		case StreamRead:
			it.Read = false
		case StreamStarred:
			it.Starred = false
		}
	}
	_, _ = w.Write([]byte("OK"))
}

// markAllAsRead marks the stream's entries with id <= ts as read: ts is
// compared with entry ids, so it is a crawl time in microseconds. ts 0 (or
// absent) means now.
func (s *Server) markAllAsRead(w http.ResponseWriter, r *http.Request) {
	stream := strings.TrimSpace(r.PostForm.Get("s"))
	raw := strings.TrimSpace(r.PostForm.Get("ts"))
	if raw == "" {
		raw = "0"
	}
	if !isDigits(raw) {
		badRequest(w)
		return
	}
	if rest, ok := strings.CutPrefix(stream, FeedPrefix); ok && !isDigits(rest) {
		badRequest(w)
		return
	}
	ts, _ := strconv.ParseInt(raw, 10, 64)
	if ts == 0 {
		ts = s.now().UnixMicro()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.markAlls = append(s.markAlls, MarkAll{Stream: stream, TS: ts})
	for _, it := range s.items {
		if it.ID <= ts && s.inStream(it, stream) {
			it.Read = true
		}
	}
	_, _ = w.Write([]byte("OK"))
}
