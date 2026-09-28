package translation

import "testing"

// Feed titles are far shorter than the trigram model needs, so script has to
// settle them. Each case is a shape that actually appears in a feed, paired
// with the answer a reader would give it.
func TestShouldTranslateClassifiesFeedTitles(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		targetLang string
		want       bool
	}{
		{"chinese with english brand word", "OpenAI 发布新模型", "zh", false},
		{"chinese behind a long product name", "Node.js 22 发布", "zh", false},
		{"chinese with a version number", "Rust 1.75 发布", "zh", false},
		{"chinese with two english names", "GPT-4 与 Claude 3 的对比评测", "zh", false},
		{"plain chinese", "美国大选结果出炉", "zh", false},
		{"english", "OpenAI releases new model", "zh", true},
		{"english with a colon", "Show HN: I built a feed reader in Go", "zh", true},
		// A short Chinese lead-in on an English sentence reads as English: the
		// body is what the reader needs translated.
		{"english sentence behind a chinese lead-in", "英伟达 CEO: AI is the new electricity", "zh", true},
		{"chinese with a borrowed kana ornament", "米其林の秘密", "zh", false},
		{"japanese", "日本の新しいニュース記事", "zh", true},
		{"korean", "한국의 새로운 뉴스 기사", "zh", true},
		{"chinese for a non-chinese target", "美国大选结果出炉", "en", true},
		{"japanese for a japanese target", "日本の新しいニュース記事", "ja", false},
		{"korean for a korean target", "한국의 새로운 뉴스 기사", "ko", false},
	}

	ld := GetLanguageDetector()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ld.ShouldTranslate(tt.title, tt.targetLang); got != tt.want {
				t.Errorf("ShouldTranslate(%q, %q) = %v, want %v", tt.title, tt.targetLang, got, tt.want)
			}
		})
	}
}

// Script only answers when it can. Latin script covers many languages, so those
// stay with the statistical detector rather than getting a wrong confident
// answer from the script pass.
func TestScriptLanguageDeclinesWhatScriptCannotSettle(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"english", "OpenAI releases new model", ""},
		{"spanish", "El nuevo modelo de OpenAI", ""},
		{"digits and punctuation only", "1.75 — 22 (2026)", ""},
		{"empty", "", ""},
		{"chinese", "美国大选结果出炉", "zh"},
		{"japanese kanji with kana", "日本の記事", "ja"},
		{"one kana in otherwise chinese text", "米其林の秘密", "zh"},
		{"korean", "한국의 기사", "ko"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scriptLanguage(tt.text); got != tt.want {
				t.Errorf("scriptLanguage(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
