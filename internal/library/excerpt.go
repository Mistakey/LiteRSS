package library

import "LiteRSS/internal/htmltext"

// excerptRunes is how long an excerpt gets; the list shows one line of it.
const excerptRunes = 200

// Excerpt turns an untrusted HTML body into the start of its visible text,
// at most excerptRunes runes. The result is plain text, never markup
// (spec D13).
func Excerpt(body string) string {
	return htmltext.Text(body, excerptRunes)
}
