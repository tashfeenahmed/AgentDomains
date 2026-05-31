// Package config persists the CLI's API endpoint and key under ~/.agentdns/config.json.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const DefaultAPIURL = "https://api.makes.fyi"

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
	return filepath.Join(home, ".agentdns"), nil
}

func path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load reads config from disk, then applies env overrides (handy for agents/CI).
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
	if v := os.Getenv("AGENTDNS_API_URL"); v != "" {
		c.APIURL = v
	}
	if v := os.Getenv("AGENTDNS_API_KEY"); v != "" {
		c.APIKey = v
	}
	return c
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
