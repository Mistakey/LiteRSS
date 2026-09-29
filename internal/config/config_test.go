package config

import (
	"strings"
	"testing"
)

func TestGetString_KnownKeys(t *testing.T) {
	for key, want := range map[string]string{
		"freshrss_auto_sync_interval": "30",
		"close_to_tray":               "true",
		"proxy_mode":                  "system",
		"llm_endpoint":                "",
	} {
		if got := GetString(key); got != want {
			t.Errorf("GetString(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestGetString_UnknownKey(t *testing.T) {
	if v := GetString("this_key_does_not_exist"); v != "" {
		t.Fatalf("expected empty string for unknown key, got %q", v)
	}
}

// The schema is the fixed list of spec D10; removed features must not creep back.
func TestSchemaHasNoRemovedKeys(t *testing.T) {
	removed := []string{"language", "theme", "translation_enabled", "summary_enabled", "shortcuts",
		"content_font_size", "freshrss_last_sync_time", "freshrss_sync_on_startup", "freshrss_enabled"}
	for _, key := range SettingsKeys() {
		if strings.HasPrefix(key, "ai_") {
			t.Errorf("schema still has %q; model settings are llm_*", key)
		}
	}
	for _, key := range removed {
		if _, ok := Lookup(key); ok {
			t.Errorf("schema still has removed key %q", key)
		}
	}
	if got := len(SettingsKeys()); got != 24 {
		t.Errorf("schema has %d keys, want the 18 panel + 1 Bionic Reading (spec D22) + 5 window keys of spec D10", got)
	}
}

func TestLookupMarksCredentialsEncrypted(t *testing.T) {
	for _, key := range []string{"freshrss_api_password", "baidu_secret_key", "llm_api_key", "proxy_username", "proxy_password"} {
		s, ok := Lookup(key)
		if !ok || !s.Encrypted {
			t.Errorf("Lookup(%q) = %+v, %v; want an encrypted setting", key, s, ok)
		}
	}
	if s, _ := Lookup("window_x"); !s.Internal() {
		t.Errorf("window_x is not internal")
	}
}
