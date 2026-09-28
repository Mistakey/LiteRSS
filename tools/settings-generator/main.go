// Settings Code Generator - generates the settings code from the schema
// Usage: go run tools/settings-generator/main.go
package main

import (
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// writeGoFile gofmt-formats generated Go source before writing it. Without this the
// generator's hand-built struct alignment drifts from gofmt's and `gofmt -l .` reports
// the generated files forever.
func writeGoFile(path, content string) error {
	formatted, err := format.Source([]byte(content))
	if err != nil {
		return fmt.Errorf("gofmt %s: %w", path, err)
	}
	return os.WriteFile(path, formatted, 0644)
}

// SettingsSchema defines the structure of settings schema
type SettingsSchema struct {
	Meta     Meta                  `json:"_meta"`
	Settings map[string]SettingDef `json:"settings"`
}

type Meta struct {
	Version     string `json:"version"`
	Description string `json:"description"`
}

type SettingDef struct {
	Type      string      `json:"type"` // int, string, bool
	Default   interface{} `json:"default"`
	Category  string      `json:"category"`
	Encrypted bool        `json:"encrypted"`
}

// categoryInternal marks settings only the backend writes (window state).
const categoryInternal = "internal"

func main() {
	schemaData, err := os.ReadFile("internal/config/settings_schema.json")
	if err != nil {
		fmt.Printf("Error reading schema: %v\n", err)
		os.Exit(1)
	}

	var schema SettingsSchema
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		fmt.Printf("Error parsing schema: %v\n", err)
		os.Exit(1)
	}
	if err := validateSchema(&schema); err != nil {
		fmt.Printf("Invalid schema: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generating code from schema with %d settings...\n", len(schema.Settings))

	steps := []struct {
		path string
		gen  func(*SettingsSchema, string) error
	}{
		{"internal/config/defaults.json", generateDefaultsJSON},
		{"internal/config/config.go", generateConfigGo},
		{"internal/config/settings_keys.go", generateSettingsKeysGo},
		{"frontend/src/types/settings.generated.ts", generateFrontendTypes},
	}
	for _, s := range steps {
		if err := s.gen(&schema, s.path); err != nil {
			fmt.Printf("Error generating %s: %v\n", s.path, err)
			os.Exit(1)
		}
		fmt.Printf("Generated %s\n", s.path)
	}
}

// validateSchema rejects a default that does not match its type and an
// encrypted setting that is not a string.
func validateSchema(schema *SettingsSchema) error {
	for _, key := range sortedKeys(schema) {
		def := schema.Settings[key]
		var ok bool
		switch def.Type {
		case "string":
			_, ok = def.Default.(string)
		case "bool":
			_, ok = def.Default.(bool)
		case "int":
			f, isNum := def.Default.(float64)
			ok = isNum && f == float64(int(f))
		default:
			return fmt.Errorf("%s: unknown type %q", key, def.Type)
		}
		if !ok {
			return fmt.Errorf("%s: default %v is not a %s", key, def.Default, def.Type)
		}
		if def.Encrypted && def.Type != "string" {
			return fmt.Errorf("%s: only string settings can be encrypted", key)
		}
		if def.Category == "" {
			return fmt.Errorf("%s: missing category", key)
		}
	}
	return nil
}

func sortedKeys(schema *SettingsSchema) []string {
	keys := make([]string, 0, len(schema.Settings))
	for key := range schema.Settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func generateDefaultsJSON(schema *SettingsSchema, path string) error {
	defaults := make(map[string]interface{})
	for key, def := range schema.Settings {
		defaults[key] = def.Default
	}

	// json.MarshalIndent sorts map keys.
	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func generateConfigGo(schema *SettingsSchema, path string) error {
	var structFields []string
	var switchCases []string

	for _, key := range sortedKeys(schema) {
		def := schema.Settings[key]
		goKey := toGoFieldName(key)
		structFields = append(structFields, fmt.Sprintf("\t%s %s `json:\"%s\"`", goKey, def.Type, key))

		var returnValue string
		switch def.Type {
		case "int":
			returnValue = fmt.Sprintf("strconv.Itoa(defaults.%s)", goKey)
		case "bool":
			returnValue = fmt.Sprintf("strconv.FormatBool(defaults.%s)", goKey)
		case "string":
			returnValue = fmt.Sprintf("defaults.%s", goKey)
		}
		switchCases = append(switchCases, fmt.Sprintf("\tcase %q:", key), "\t\treturn "+returnValue)
	}

	tmpl := `// Package config holds the settings schema: default values and per-key
// metadata, generated from settings_schema.json.
//
// CODE GENERATED - DO NOT EDIT MANUALLY
// To change settings, edit internal/config/settings_schema.json and run: go run tools/settings-generator/main.go
package config

import (
	_ "embed"
	"encoding/json"
	"strconv"
)

//go:embed defaults.json
var defaultsJSON []byte

// Defaults holds all default settings values
type Defaults struct {
%s
}

var defaults Defaults

func init() {
	if err := json.Unmarshal(defaultsJSON, &defaults); err != nil {
		panic("failed to parse defaults.json: " + err.Error())
	}
}

// Get returns the loaded defaults
func Get() Defaults {
	return defaults
}

// GetString returns a setting default as a string, or "" for an unknown key.
func GetString(key string) string {
	switch key {
%s
	default:
		return ""
	}
}
`

	content := fmt.Sprintf(tmpl,
		strings.Join(structFields, "\n"),
		strings.Join(switchCases, "\n"))
	return writeGoFile(path, content)
}

func generateSettingsKeysGo(schema *SettingsSchema, path string) error {
	var entries []string
	for _, key := range sortedKeys(schema) {
		def := schema.Settings[key]
		entries = append(entries, fmt.Sprintf("\t%q: {Type: Type%s, Category: %q, Encrypted: %t},",
			key, strings.ToUpper(def.Type[:1])+def.Type[1:], def.Category, def.Encrypted))
	}

	tmpl := `// CODE GENERATED - DO NOT EDIT MANUALLY
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
	return s.Category == %q
}

var settings = map[string]Setting{
%s
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
`

	content := fmt.Sprintf(tmpl, categoryInternal, strings.Join(entries, "\n"))
	return writeGoFile(path, content)
}

// generateFrontendTypes writes the settings the frontend edits: every key but
// the internal ones. The output is already in the repository's prettier style,
// so a rerun leaves no diff.
func generateFrontendTypes(schema *SettingsSchema, path string) error {
	var fields []string
	var defaults []string
	for _, key := range sortedKeys(schema) {
		def := schema.Settings[key]
		if def.Category == categoryInternal {
			continue
		}
		fields = append(fields, fmt.Sprintf("  %s: %s;", key, toTSType(def.Type)))
		defaults = append(defaults, fmt.Sprintf("  %s: %s,", key, toTSLiteral(def.Default)))
	}

	tmpl := `// CODE GENERATED - DO NOT EDIT MANUALLY
// To change settings, edit internal/config/settings_schema.json and run: go run tools/settings-generator/main.go

export interface SettingsData {
%s
}

export const settingsDefaults: SettingsData = {
%s
};
`

	content := fmt.Sprintf(tmpl, strings.Join(fields, "\n"), strings.Join(defaults, "\n"))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func toGoFieldName(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		switch {
		case p == "freshrss":
			parts[i] = "FreshRSS"
		case p == "api" || p == "llm" || p == "url" || p == "id":
			parts[i] = strings.ToUpper(p)
		case p != "":
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func toTSType(typ string) string {
	switch typ {
	case "int":
		return "number"
	case "bool":
		return "boolean"
	default:
		return "string"
	}
}

// toTSLiteral writes a default as prettier would: single-quoted strings.
func toTSLiteral(v interface{}) string {
	switch v := v.(type) {
	case string:
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`).Replace(v) + "'"
	case float64:
		return strconv.Itoa(int(v))
	default:
		return fmt.Sprint(v)
	}
}
