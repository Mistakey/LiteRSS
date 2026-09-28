// CODE GENERATED - DO NOT EDIT MANUALLY
// To change settings, edit internal/config/settings_schema.json and run: go run tools/settings-generator/main.go
package config

import "sort"

// Type is how a setting's stored string is parsed.
type Type string

const (
	TypeString Type = "string"
	TypeInt    Type = "int"
	TypeBool   Type = "bool"
)

// Setting describes one key of the schema.
type Setting struct {
	Type      Type
	Category  string
	Encrypted bool
}

// Internal reports whether only the backend writes this setting.
func (s Setting) Internal() bool {
	return s.Category == "internal"
}

var settings = map[string]Setting{
	"baidu_app_id":                {Type: TypeString, Category: "translation", Encrypted: false},
	"baidu_secret_key":            {Type: TypeString, Category: "translation", Encrypted: true},
	"close_to_tray":               {Type: TypeBool, Category: "app", Encrypted: false},
	"freshrss_api_password":       {Type: TypeString, Category: "freshrss", Encrypted: true},
	"freshrss_auto_sync_interval": {Type: TypeInt, Category: "freshrss", Encrypted: false},
	"freshrss_server_url":         {Type: TypeString, Category: "freshrss", Encrypted: false},
	"freshrss_username":           {Type: TypeString, Category: "freshrss", Encrypted: false},
	"llm_api_key":                 {Type: TypeString, Category: "llm", Encrypted: true},
	"llm_endpoint":                {Type: TypeString, Category: "llm", Encrypted: false},
	"llm_model":                   {Type: TypeString, Category: "llm", Encrypted: false},
	"proxy_host":                  {Type: TypeString, Category: "network", Encrypted: false},
	"proxy_mode":                  {Type: TypeString, Category: "network", Encrypted: false},
	"proxy_password":              {Type: TypeString, Category: "network", Encrypted: true},
	"proxy_port":                  {Type: TypeString, Category: "network", Encrypted: false},
	"proxy_type":                  {Type: TypeString, Category: "network", Encrypted: false},
	"proxy_username":              {Type: TypeString, Category: "network", Encrypted: true},
	"startup_on_boot":             {Type: TypeBool, Category: "app", Encrypted: false},
	"update_check_enabled":        {Type: TypeBool, Category: "app", Encrypted: false},
	"window_height":               {Type: TypeInt, Category: "internal", Encrypted: false},
	"window_maximized":            {Type: TypeBool, Category: "internal", Encrypted: false},
	"window_width":                {Type: TypeInt, Category: "internal", Encrypted: false},
	"window_x":                    {Type: TypeInt, Category: "internal", Encrypted: false},
	"window_y":                    {Type: TypeInt, Category: "internal", Encrypted: false},
}

// Lookup returns the schema entry for key; ok is false for a key outside the schema.
func Lookup(key string) (s Setting, ok bool) {
	s, ok = settings[key]
	return s, ok
}

// SettingsKeys returns every key of the schema, sorted.
func SettingsKeys() []string {
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
