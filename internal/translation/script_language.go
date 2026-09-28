package translation

import (
	"strings"
	"unicode"
)

// scriptRatioThreshold is the share of letter-class characters one script must
// hold before it settles the text's language.
//
// The threshold is low on purpose, because product names are long and Chinese
// words are short: "Node.js 22 发布" is a Chinese title whose Han share is only
// 0.25. Leaving a Chinese text untranslated costs a reader of Chinese nothing;
// translating one costs a provider call, a mangled title and a polluted cache.
// The cost of the low bar is that a mostly-English sentence behind a short
// Chinese lead-in ("英伟达 CEO: AI is the new electricity", 0.11) still counts
// as English, which is the intended reading.
const scriptRatioThreshold = 0.2

// scriptLanguage returns the language a text's Unicode script settles on its
// own, or "" when the script cannot decide.
//
// The statistical detector behind DetectLanguage is a trigram model and is
// unreliable below roughly 50 characters — exactly the length of every feed
// title. Script is not a statistical signal: an English title never contains
// Han characters, and kana never appear outside Japanese. So script is asked
// first, the way browser language detectors ask it, and the trigram model only
// settles what script leaves open (English vs Spanish vs French, and the like).
//
// Every script is weighed by share rather than by presence, because a single
// borrowed character does not change what a text is: Chinese headlines borrow
// "の" as an ornament, and one such rune must not turn a Chinese title into a
// Japanese one. Kana and Hangul are weighed before Han, since Japanese prose is
// dense with kanji and would otherwise read as Chinese.
func scriptLanguage(text string) string {
	var han, kana, hangul, letters int
	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana++
			letters++
		case unicode.In(r, unicode.Hangul):
			hangul++
			letters++
		case unicode.In(r, unicode.Han):
			han++
			letters++
		case unicode.IsLetter(r):
			letters++
		}
	}
	if letters == 0 {
		return ""
	}

	switch {
	case float64(kana)/float64(letters) >= scriptRatioThreshold:
		return "ja"
	case float64(hangul)/float64(letters) >= scriptRatioThreshold:
		return "ko"
	case float64(han)/float64(letters) >= scriptRatioThreshold:
		return "zh"
	}
	return ""
}

// sameReadingLanguage reports whether two language codes are one language to a
// reader. Simplified and Traditional Chinese are: a Traditional title is
// already Chinese to someone who asked for Chinese, and round-tripping it
// through a translation provider serves nobody.
func sameReadingLanguage(a, b string) bool {
	a = normalizeLangCode(a)
	b = normalizeLangCode(b)
	if isChineseCode(a) && isChineseCode(b) {
		return true
	}
	return a == b
}

func isChineseCode(code string) bool {
	return strings.HasPrefix(code, "zh")
}
