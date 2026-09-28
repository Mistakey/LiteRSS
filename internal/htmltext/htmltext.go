// Package htmltext turns untrusted HTML into the plain text a reader sees.
package htmltext

import (
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// hiddenElements hold nothing a reader sees as the article's text.
var hiddenElements = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true, "iframe": true,
	"object": true, "svg": true, "math": true, "head": true, "title": true,
	"textarea": true, "xmp": true, "noembed": true, "noframes": true, "plaintext": true,
}

// inlineElements run into the surrounding text; any other tag separates
// words.
var inlineElements = map[string]bool{
	"a": true, "abbr": true, "b": true, "bdi": true, "bdo": true, "cite": true, "code": true, "data": true,
	"dfn": true, "em": true, "i": true, "kbd": true, "mark": true, "q": true, "s": true, "samp": true,
	"small": true, "span": true, "strong": true, "sub": true, "sup": true, "time": true, "u": true, "var": true,
}

// Text turns an untrusted HTML body into its visible text: tags dropped,
// entities decoded, invisible elements skipped, whitespace collapsed, and at
// most maxRunes runes. The result is plain text, never markup.
func Text(body string, maxRunes int) string {
	var (
		text    strings.Builder
		hidden  int
		visible int // non-space runes so far; the text is full at maxRunes
	)
	z := html.NewTokenizer(strings.NewReader(body))
	for visible < maxRunes {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if hiddenElements[tag] {
				if tt == html.StartTagToken {
					hidden++
				} else if tt == html.EndTagToken && hidden > 0 {
					hidden--
				}
			}
			if !inlineElements[tag] {
				text.WriteByte(' ')
			}
		case html.TextToken:
			if hidden > 0 {
				continue
			}
			for _, r := range string(z.Text()) {
				text.WriteRune(r)
				if !unicode.IsSpace(r) {
					visible++
				}
			}
		}
	}
	runes := []rune(strings.Join(strings.Fields(text.String()), " "))
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return strings.TrimSpace(string(runes))
}

// Visible counts the non-space runes of text, the length a reader judges;
// unlike a byte count it weighs Chinese and English alike.
func Visible(text string) int {
	n := 0
	for _, r := range text {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}
