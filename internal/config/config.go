// Package config holds the settings schema: default values and per-key
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
	BaiduAppID               string `json:"baidu_app_id"`
	BaiduSecretKey           string `json:"baidu_secret_key"`
	BionicReading            bool   `json:"bionic_reading"`
	CloseToTray              bool   `json:"close_to_tray"`
	FreshRSSAPIPassword      string `json:"freshrss_api_password"`
	FreshRSSAutoSyncInterval int    `json:"freshrss_auto_sync_interval"`
	FreshRSSServerURL        string `json:"freshrss_server_url"`
	FreshRSSUsername         string `json:"freshrss_username"`
	LLMAPIKey                string `json:"llm_api_key"`
	LLMEndpoint              string `json:"llm_endpoint"`
	LLMModel                 string `json:"llm_model"`
	ProxyHost                string `json:"proxy_host"`
	ProxyMode                string `json:"proxy_mode"`
	ProxyPassword            string `json:"proxy_password"`
	ProxyPort                string `json:"proxy_port"`
	ProxyType                string `json:"proxy_type"`
	ProxyUsername            string `json:"proxy_username"`
	StartupOnBoot            bool   `json:"startup_on_boot"`
	UpdateCheckEnabled       bool   `json:"update_check_enabled"`
	WindowHeight             int    `json:"window_height"`
	WindowMaximized          bool   `json:"window_maximized"`
	WindowWidth              int    `json:"window_width"`
	WindowX                  int    `json:"window_x"`
	WindowY                  int    `json:"window_y"`
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
	case "baidu_app_id":
		return defaults.BaiduAppID
	case "baidu_secret_key":
		return defaults.BaiduSecretKey
	case "bionic_reading":
		return strconv.FormatBool(defaults.BionicReading)
	case "close_to_tray":
		return strconv.FormatBool(defaults.CloseToTray)
	case "freshrss_api_password":
		return defaults.FreshRSSAPIPassword
	case "freshrss_auto_sync_interval":
		return strconv.Itoa(defaults.FreshRSSAutoSyncInterval)
	case "freshrss_server_url":
		return defaults.FreshRSSServerURL
	case "freshrss_username":
		return defaults.FreshRSSUsername
	case "llm_api_key":
		return defaults.LLMAPIKey
	case "llm_endpoint":
		return defaults.LLMEndpoint
	case "llm_model":
		return defaults.LLMModel
	case "proxy_host":
		return defaults.ProxyHost
	case "proxy_mode":
		return defaults.ProxyMode
	case "proxy_password":
		return defaults.ProxyPassword
	case "proxy_port":
		return defaults.ProxyPort
	case "proxy_type":
		return defaults.ProxyType
	case "proxy_username":
		return defaults.ProxyUsername
	case "startup_on_boot":
		return strconv.FormatBool(defaults.StartupOnBoot)
	case "update_check_enabled":
		return strconv.FormatBool(defaults.UpdateCheckEnabled)
	case "window_height":
		return strconv.Itoa(defaults.WindowHeight)
	case "window_maximized":
		return strconv.FormatBool(defaults.WindowMaximized)
	case "window_width":
		return strconv.Itoa(defaults.WindowWidth)
	case "window_x":
		return strconv.Itoa(defaults.WindowX)
	case "window_y":
		return strconv.Itoa(defaults.WindowY)
	default:
		return ""
	}
}
