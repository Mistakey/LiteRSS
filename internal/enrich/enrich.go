// Package enrich holds the content actions on articles the reader opens:
// fetching the full text, translating titles, summarizing (spec D11) and
// translating the body (spec D21). Their results live in the local tables
// fulltext_cache, title_translations, summaries and article_translations,
// which sync never writes.
package enrich

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"LiteRSS/internal/fulltext"
	"LiteRSS/internal/htmltext"
	"LiteRSS/internal/library"
	"LiteRSS/internal/summary"
	"LiteRSS/internal/translation"
)

// Settings is where the Baidu and model configuration is read, on every
// call, so a change applies at once; *settings.Store implements it.
type Settings interface {
	Load(ctx context.Context) (map[string]string, error)
}

// Clients carry the outbound requests: Web fetches article pages, API talks
// to Baidu and the model (pitfall 8).
type Clients struct {
	Web *http.Client
	API *http.Client
}

// Service runs the content actions against the local library.
type Service struct {
	db       *sql.DB
	settings Settings
	fetcher  *fulltext.Fetcher
	api      *http.Client

	// BaiduEndpoint overrides translation.BaiduEndpoint in tests.
	BaiduEndpoint string
	// BaiduGap spaces the requests of one translation call; Baidu's
	// standard tier allows one query per second.
	BaiduGap time.Duration

	mu      sync.Mutex
	fetches map[int64]*fetch // full-text fetches in flight, by item id

	titleSem chan struct{} // one title translation at a time: no id is sent twice
}

// New returns the service over db.
func New(db *sql.DB, settings Settings, clients Clients) *Service {
	return &Service{
		db:       db,
		settings: settings,
		fetcher:  fulltext.NewFetcher(clients.Web),
		api:      clients.API,
		BaiduGap: 1100 * time.Millisecond,
		fetches:  map[int64]*fetch{},
		titleSem: make(chan struct{}, 1),
	}
}

// FullText is the answer to a full-text fetch. Content is the extracted
// article, untrusted HTML the frontend sanitizes (spec D16); Message is the
// Chinese reason for a failure, empty on success.
type FullText struct {
	Outcome fulltext.Outcome `json:"outcome"`
	Content string           `json:"content"`
	Message string           `json:"message"`
}

type fetch struct {
	done chan struct{}
	res  FullText
	err  error
}

// FullText returns an article's full text: the cached one, else a fresh
// fetch of its link, which only the reader's button asks for (spec D11).
// Only a success is cached (pitfall 21), and it drops the article's summary
// and translation, which were made from the RSS body. Concurrent calls for one article share a
// fetch.
func (s *Service) FullText(ctx context.Context, id int64) (FullText, error) {
	var content string
	err := s.db.QueryRowContext(ctx, `SELECT content FROM fulltext_cache WHERE item_id = ?`, id).Scan(&content)
	if err == nil {
		return FullText{Outcome: fulltext.OutcomeSuccess, Content: content}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return FullText{}, fmt.Errorf("read full text: %w", err)
	}
	var link string
	err = s.db.QueryRowContext(ctx, `SELECT url FROM articles WHERE item_id = ?`, id).Scan(&link)
	if errors.Is(err, sql.ErrNoRows) {
		return FullText{}, fmt.Errorf("article %d: %w", id, library.ErrNotFound)
	}
	if err != nil {
		return FullText{}, fmt.Errorf("read article link: %w", err)
	}
	if !fetchable(link) {
		return failed(fulltext.OutcomeNoLink), nil
	}

	s.mu.Lock()
	f := s.fetches[id]
	if f == nil {
		f = &fetch{done: make(chan struct{})}
		s.fetches[id] = f
		go s.runFetch(id, link, f)
	}
	s.mu.Unlock()
	select {
	case <-f.done:
		return f.res, f.err
	case <-ctx.Done():
		return FullText{}, ctx.Err()
	}
}

// runFetch fetches outside any request's context: a caller that leaves does
// not cancel the fetch another caller waits on. The web client's timeout
// bounds it.
func (s *Service) runFetch(id int64, link string, f *fetch) {
	defer func() {
		s.mu.Lock()
		delete(s.fetches, id)
		s.mu.Unlock()
		close(f.done)
	}()
	ctx := context.Background()
	r, err := s.fetcher.Fetch(ctx, link)
	if err != nil {
		// fetchable already vetted the link, so this is a program fault.
		f.err = fmt.Errorf("fetch full text: %w", err)
		return
	}
	if r.Outcome != fulltext.OutcomeSuccess {
		log.Printf("Full text for %d (%s): %s (%s)", id, link, r.Outcome, r.Detail)
		f.res = failed(r.Outcome)
		return
	}
	if err := s.cacheFullText(ctx, id, r.Content); err != nil {
		f.err = err
		return
	}
	f.res = FullText{Outcome: fulltext.OutcomeSuccess, Content: r.Content}
}

// cacheFullText stores a fetched full text and drops the summary and the
// translation made before it, so the next ones are made from what the reader
// now shows. The article may be gone meanwhile (retention cleanup); then
// nothing is cached.
func (s *Service) cacheFullText(ctx context.Context, id int64, content string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cache full text: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO fulltext_cache (item_id, content, cached_at)
		 SELECT ?, ?, ? WHERE EXISTS (SELECT 1 FROM articles WHERE item_id = ?)
		 ON CONFLICT (item_id) DO UPDATE SET content = excluded.content, cached_at = excluded.cached_at`,
		id, content, time.Now().Unix(), id); err != nil {
		return fmt.Errorf("cache full text: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM summaries WHERE item_id = ?`, id); err != nil {
		return fmt.Errorf("drop the RSS summary: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM article_translations WHERE item_id = ?`, id); err != nil {
		return fmt.Errorf("drop the RSS translation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cache full text: %w", err)
	}
	return nil
}

func failed(o fulltext.Outcome) FullText {
	return FullText{Outcome: o, Message: o.Message()}
}

// fetchable reports whether link is an absolute http(s) URL with a host.
func fetchable(link string) bool {
	u, err := url.Parse(link)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Title is one article's decided title: a Chinese translation, or the title
// itself when it was judged already Chinese (pitfall 12).
type Title struct {
	ID              int64  `json:"id"`
	TranslatedTitle string `json:"translated_title"`
}

// Titles answers a title translation. Articles left undecided are absent;
// Message then says why in Chinese.
type Titles struct {
	Titles  []Title `json:"titles"`
	Message string  `json:"message"`
}

// TranslateTitles decides the titles of the given articles, at most
// library.MaxCards: a title already decided is answered as stored; one whose
// script says Chinese is stored as itself; the rest go to Baidu (spec D11).
// Articles no longer in the library, and empty titles, are skipped.
func (s *Service) TranslateTitles(ctx context.Context, ids []int64) (Titles, error) {
	if len(ids) > library.MaxCards {
		return Titles{}, fmt.Errorf("at most %d ids: %w", library.MaxCards, library.ErrBadRequest)
	}
	out := Titles{Titles: []Title{}}
	if len(ids) == 0 {
		return out, nil
	}
	select {
	case s.titleSem <- struct{}{}:
		defer func() { <-s.titleSem }()
	case <-ctx.Done():
		return Titles{}, ctx.Err()
	}

	type row struct {
		title, decided string
	}
	rows := map[int64]row{}
	q := `SELECT a.item_id, a.title, COALESCE(t.translated_title, '') FROM articles a
		LEFT JOIN title_translations t ON t.item_id = a.item_id
		WHERE a.item_id IN (?` + strings.Repeat(",?", len(ids)-1) + `)`
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rs, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return Titles{}, fmt.Errorf("read titles: %w", err)
	}
	for rs.Next() {
		var id int64
		var r row
		if err := rs.Scan(&id, &r.title, &r.decided); err != nil {
			rs.Close()
			return Titles{}, fmt.Errorf("read titles: %w", err)
		}
		rows[id] = r
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return Titles{}, fmt.Errorf("read titles: %w", err)
	}

	decided := map[int64]string{}
	var pending []int64 // in request order
	var lines []string
	detector := translation.GetLanguageDetector()
	for _, id := range ids {
		r, ok := rows[id]
		if !ok || r.decided != "" {
			continue
		}
		line := strings.Join(strings.Fields(r.title), " ")
		switch {
		case line == "":
		case !detector.ShouldTranslate(line, "zh"):
			decided[id] = r.title
		default:
			if !slices.Contains(pending, id) {
				pending = append(pending, id)
				lines = append(lines, line)
			}
		}
	}

	if len(pending) > 0 {
		translated, msg := s.baidu(ctx, lines)
		out.Message = msg
		for i, dst := range translated {
			if dst == "" {
				continue
			}
			if dst == lines[i] {
				// Baidu kept it: the title is decided as it is.
				dst = rows[pending[i]].title
			}
			decided[pending[i]] = dst
		}
	}
	if err := s.storeTitles(ctx, decided); err != nil {
		return Titles{}, err
	}

	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		t := rows[id].decided
		if t == "" {
			t = decided[id]
		}
		if t != "" {
			out.Titles = append(out.Titles, Title{ID: id, TranslatedTitle: t})
		}
	}
	return out, nil
}

// baidu translates lines through Baidu in requests of at most
// translation.BaiduMaxQueryBytes, stopping at the first failure. A line
// left untranslated stays ""; the message says why in Chinese.
func (s *Service) baidu(ctx context.Context, lines []string) ([]string, string) {
	out := make([]string, len(lines))
	values, err := s.settings.Load(ctx)
	if err != nil {
		log.Printf("Title translation: read settings: %v", err)
		return out, "读取设置失败，标题暂时显示原文。"
	}
	appID, secret := values["baidu_app_id"], values["baidu_secret_key"]
	if appID == "" || secret == "" {
		return out, "还没有配置百度翻译，英文标题暂时显示原文。"
	}
	b := &translation.Baidu{AppID: appID, SecretKey: secret, Endpoint: s.BaiduEndpoint, Client: s.api}

	for start := 0; start < len(lines); {
		end, size := start, 0
		for end < len(lines) && (end == start || size+len(lines[end])+1 <= translation.BaiduMaxQueryBytes) {
			size += len(lines[end]) + 1
			end++
		}
		if start > 0 {
			select {
			case <-time.After(s.BaiduGap):
			case <-ctx.Done():
				return out, "标题翻译被取消。"
			}
		}
		dst, err := b.TranslateLines(ctx, lines[start:end])
		if err != nil {
			log.Printf("Title translation: %v", err)
			return out, baiduMessage(err)
		}
		copy(out[start:end], dst)
		start = end
	}
	return out, ""
}

// baiduMessage is the Chinese reason for a failed Baidu request.
func baiduMessage(err error) string {
	var be *translation.BaiduError
	if !errors.As(err, &be) {
		return "连不上百度翻译，请检查网络或代理设置。"
	}
	switch be.Code {
	case "52003", "54001":
		return "百度翻译的 APP ID 或密钥不对，请在设置里检查。"
	case "54003":
		return "百度翻译访问太频繁，稍后再试。"
	case "54004":
		return "百度翻译账户余额不足。"
	default:
		return fmt.Sprintf("百度翻译返回错误 %s，标题暂时显示原文。", be.Code)
	}
}

func (s *Service) storeTitles(ctx context.Context, decided map[int64]string) error {
	if len(decided) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store titles: %w", err)
	}
	defer tx.Rollback()
	for id, t := range decided {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO title_translations (item_id, translated_title)
			 SELECT ?, ? WHERE EXISTS (SELECT 1 FROM articles WHERE item_id = ?)
			 ON CONFLICT (item_id) DO UPDATE SET translated_title = excluded.translated_title`,
			id, t, id); err != nil {
			return fmt.Errorf("store title %d: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store titles: %w", err)
	}
	return nil
}

const (
	// minSummaryRunes is how much visible text makes a body worth
	// summarizing (spec D11: too short is refused with the reason).
	minSummaryRunes = 300
	// maxSummaryRunes bounds the text sent to the model.
	maxSummaryRunes = 20000
)

// Summary answers a summary request. HTML is the rendered summary, untrusted
// until the frontend sanitizes it (spec D16), empty when none was made; Note
// is Chinese: why there is none, or what the summary is based on when it is
// not the full text.
type Summary struct {
	HTML string `json:"html"`
	Note string `json:"note"`
}

// Summarize returns an article's summary: the stored one, else a new one
// from what the reader shows, the cached full text or else the RSS body. It
// never fetches the page itself (spec D11).
func (s *Service) Summarize(ctx context.Context, id int64) (Summary, error) {
	var md, note string
	err := s.db.QueryRowContext(ctx, `SELECT summary, note FROM summaries WHERE item_id = ?`, id).Scan(&md, &note)
	if err == nil {
		return Summary{HTML: summary.RenderHTML(md), Note: note}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Summary{}, fmt.Errorf("read summary: %w", err)
	}
	var title, body string
	var full sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT a.title, COALESCE(c.content, ''), f.content FROM articles a
		 LEFT JOIN article_contents c ON c.item_id = a.item_id
		 LEFT JOIN fulltext_cache f ON f.item_id = a.item_id WHERE a.item_id = ?`, id).Scan(&title, &body, &full)
	if errors.Is(err, sql.ErrNoRows) {
		return Summary{}, fmt.Errorf("article %d: %w", id, library.ErrNotFound)
	}
	if err != nil {
		return Summary{}, fmt.Errorf("read article: %w", err)
	}

	model, ok, err := s.model(ctx)
	if err != nil {
		return Summary{}, err
	}
	if !ok {
		return Summary{Note: noModel}, nil
	}

	fromRSS := !full.Valid
	source := full.String
	if fromRSS {
		source = body
		note = "摘要基于 RSS 正文。"
	}
	text := htmltext.Text(source, maxSummaryRunes)
	if htmltext.Visible(text) < minSummaryRunes {
		if fromRSS {
			return Summary{Note: "RSS 正文太短，无法生成摘要，可以先抓取全文。"}, nil
		}
		return Summary{Note: "正文太短，无法生成摘要。"}, nil
	}

	md, err = model.Summarize(ctx, title, text)
	if err != nil {
		if ctx.Err() != nil {
			return Summary{}, ctx.Err()
		}
		log.Printf("Summary for %d: %v", id, err)
		return Summary{Note: "摘要生成失败，请检查设置里的大模型，或稍后再试。"}, nil
	}
	// A made summary is kept even when the reader has left meanwhile, but one
	// from the RSS body not once a full text has arrived: that one replaces it.
	if _, err := s.db.ExecContext(context.WithoutCancel(ctx),
		`INSERT INTO summaries (item_id, summary, note, created_at)
		 SELECT ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM articles WHERE item_id = ?)
		   AND NOT (? AND EXISTS (SELECT 1 FROM fulltext_cache WHERE item_id = ?))
		 ON CONFLICT (item_id) DO UPDATE SET summary = excluded.summary, note = excluded.note, created_at = excluded.created_at`,
		id, md, note, time.Now().Unix(), id, fromRSS, id); err != nil {
		return Summary{}, fmt.Errorf("store summary: %w", err)
	}
	return Summary{HTML: summary.RenderHTML(md), Note: note}, nil
}

// noModel is the Chinese hint when the model is not configured.
const noModel = "还没有配置大模型，请在设置里填写。"

// model reads the one model configuration (spec D10); ok is false when it
// is not configured, its endpoint or model name empty.
func (s *Service) model(ctx context.Context) (m summary.Model, ok bool, err error) {
	values, err := s.settings.Load(ctx)
	if err != nil {
		return summary.Model{}, false, fmt.Errorf("read the model settings: %w", err)
	}
	m = summary.Model{Endpoint: values["llm_endpoint"], Name: values["llm_model"], APIKey: values["llm_api_key"], HTTP: s.api}
	return m, m.Endpoint != "" && m.Name != "", nil
}
