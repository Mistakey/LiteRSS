package freshrsstest

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"time"
)

var (
	feedLabels = []string{"科技", "Tech", "新闻", "Engineering/Go"}
	enTitles   = []string{
		"Go %d.%d released with faster builds",
		"How we cut our SQLite write latency in half (part %d.%d)",
		"A field guide to HTTP caching, chapter %d.%d",
		"Why the scheduler stalls under load: notes %d.%d",
	}
	zhTitles = []string{
		"开源项目 %d.%d 版本发布",
		"关于本地优先软件的第 %d 篇笔记（%d）",
		"数据库迁移实践 %d-%d",
		"周报：第 %d 周第 %d 期",
	}
)

// Generate builds a reproducible account for a development instance: feeds
// spread over a few labels, and per feed itemsPerFeed entries crawled over the
// last 100 days, so both sides of the 90-day retention are present. Titles mix
// English and Chinese; older entries are mostly read; every tenth entry reuses
// the previous entry's URL, as a republished article does; a few entries have
// no published time. Odd feeds have an icon, even ones only FreshRSS's
// placeholder.
func Generate(seed int64, feeds, itemsPerFeed int, now time.Time) ([]Feed, []Item) {
	rng := rand.New(rand.NewSource(seed))
	window := int64(100 * 24 * time.Hour / time.Microsecond)
	nowID := now.UnixMicro()

	var outFeeds []Feed
	var outItems []Item
	used := map[int64]bool{}
	for f := 1; f <= feeds; f++ {
		feed := Feed{
			ID:      f,
			Title:   fmt.Sprintf("示例订阅 %d / Sample Feed %d", f, f),
			URL:     fmt.Sprintf("https://feed%d.example.com/rss.xml", f),
			HTMLURL: fmt.Sprintf("https://feed%d.example.com/", f),
			Labels:  []string{feedLabels[(f-1)%len(feedLabels)]},
		}
		if f%2 == 1 {
			feed.Icon = iconPNG(f)
		}
		outFeeds = append(outFeeds, feed)

		prevURL := ""
		for n := 1; n <= itemsPerFeed; n++ {
			id := nowID - rng.Int63n(window)
			for used[id] {
				id--
			}
			used[id] = true

			var title string
			if rng.Intn(2) == 0 {
				title = fmt.Sprintf(enTitles[rng.Intn(len(enTitles))], f, n)
			} else {
				title = fmt.Sprintf(zhTitles[rng.Intn(len(zhTitles))], f, n)
			}
			url := fmt.Sprintf("https://feed%d.example.com/posts/%d", f, n)
			if n%10 == 0 && prevURL != "" {
				url = prevURL
			}
			prevURL = url

			crawled := time.UnixMicro(id)
			published := crawled.Add(-time.Duration(rng.Intn(6*60)) * time.Minute)
			if n%17 == 0 {
				published = time.Time{}
			}
			age := now.Sub(crawled)
			outItems = append(outItems, Item{
				ID:        id,
				FeedID:    f,
				Title:     title,
				URL:       url,
				Author:    fmt.Sprintf("author%d", rng.Intn(5)+1),
				Content:   fmt.Sprintf("<p>%s</p><p>Generated body for entry %d of feed %d.</p>", title, n, f),
				Published: published,
				Read:      age > 3*24*time.Hour && rng.Intn(10) < 7,
			})
		}
	}
	return outFeeds, outItems
}

// iconPNG draws a 32×32 icon, a light square on a colour that differs per
// feed, so a screenshot tells the feeds apart.
func iconPNG(f int) []byte {
	bg := color.RGBA{R: uint8(40 * f % 256), G: uint8(90 + 50*f%160), B: uint8(200 - 30*f%200), A: 255}
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			c := bg
			if x >= 10 && x < 22 && y >= 10 && y < 22 {
				c = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
