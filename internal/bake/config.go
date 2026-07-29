package bake

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/xilistudios/mcp2cli/internal/cache"
)

// Config mirrors a baked tool entry. JSON tags match the Python baked.json
// keys exactly for round-trip compatibility.
type Config struct {
	SourceType        string            `json:"source_type"`
	Source            string            `json:"source"`
	BaseURL           string            `json:"base_url"`
	AuthHeaders       [][2]string       `json:"auth_headers"`
	EnvVars           map[string]string `json:"env_vars"`
	CacheTTL          int               `json:"cache_ttl"`
	Transport         string            `json:"transport"`
	OAuth             bool              `json:"oauth"`
	OAuthClientID     string            `json:"oauth_client_id"`
	OAuthClientSecret string            `json:"oauth_client_secret"`
	OAuthClientName   string            `json:"oauth_client_name"`
	OAuthScope        string            `json:"oauth_scope"`
	OAuthRedirectURI  string            `json:"oauth_redirect_uri"`
	OAuthFlow         string            `json:"oauth_flow"`
	Include           []string          `json:"include"`
	Exclude           []string          `json:"exclude"`
	Methods           []string          `json:"methods"`
	Description       string            `json:"description"`
}

// LoadAll reads all baked configs from the baked.json file. Returns an
// empty map if the file is missing or corrupt.
func LoadAll() (map[string]Config, error) {
	path := cache.BakedFile()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Config{}, nil
		}
		return nil, fmt.Errorf("reading baked config: %w", err)
	}
	var all map[string]Config
	if err := json.Unmarshal(raw, &all); err != nil {
		return map[string]Config{}, nil // corrupt → empty map
	}
	if all == nil {
		all = map[string]Config{}
	}
	return all, nil
}

// Load returns a single baked config by name.
func Load(name string) (Config, bool, error) {
	all, err := LoadAll()
	if err != nil {
		return Config{}, false, err
	}
	cfg, ok := all[name]
	return cfg, ok, nil
}

// SaveAll persists all baked configs as indented JSON with a trailing newline.
func SaveAll(all map[string]Config) error {
	dir := cache.ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	raw, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling baked config: %w", err)
	}
	raw = append(raw, '\n')
	return os.WriteFile(cache.BakedFile(), raw, 0o644)
}
