// Package config persists the CLI's API endpoint and key under ~/.agentdomains/config.json.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const DefaultAPIURL = "https://api.agentdomains.co"

type Config struct {
	APIURL    string `json:"api_url"`
	APIKey    string `json:"api_key"`
	AccountID string `json:"account_id,omitempty"`
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agentdomains"), nil
}

func path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load reads config from disk, then applies env overrides (handy for agents/CI).
// The AGENTDOMAINS_* env vars are preferred; the older AGENTDNS_* names are still
// honored so existing setups keep working.
func Load() Config {
	c := Config{APIURL: DefaultAPIURL}
	if p, err := path(); err == nil {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &c)
		}
	}
	if c.APIURL == "" {
		c.APIURL = DefaultAPIURL
	}
	if v := firstEnv("AGENTDOMAINS_API_URL", "AGENTDNS_API_URL"); v != "" {
		c.APIURL = v
	}
	if v := firstEnv("AGENTDOMAINS_API_KEY", "AGENTDNS_API_KEY"); v != "" {
		c.APIKey = v
	}
	return c
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func Save(c Config) error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	p, err := path()
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, b, 0o600) // contains the API key — keep it private
}
