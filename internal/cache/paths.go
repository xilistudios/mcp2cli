// Package cache provides caching, usage tracking, and path helpers for
// mcp2cli. All path functions read environment variables at call time so
// that tests can isolate via t.Setenv.
package cache

import (
	"os"
	"path/filepath"
)

// DefaultCacheTTL is the default time-to-live for cached entries (seconds).
const DefaultCacheTTL = 3600

// CacheDir returns the cache directory. It reads MCP2CLI_CACHE_DIR first,
// falling back to ~/.cache/mcp2cli.
func CacheDir() string {
	if v := os.Getenv("MCP2CLI_CACHE_DIR"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), ".cache", "mcp2cli")
}

// ConfigDir returns the configuration directory. It reads MCP2CLI_CONFIG_DIR
// first, falling back to ~/.config/mcp2cli.
func ConfigDir() string {
	if v := os.Getenv("MCP2CLI_CONFIG_DIR"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), ".config", "mcp2cli")
}

// UsageFile returns the path to usage.json inside the cache directory.
func UsageFile() string {
	return filepath.Join(CacheDir(), "usage.json")
}

// BakedFile returns the path to baked.json inside the config directory.
func BakedFile() string {
	return filepath.Join(ConfigDir(), "baked.json")
}

// homeDir returns the user's home directory, or "/" if it cannot be
// determined (mimics os.UserHomeDir behaviour without the error).
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}

// OAuthDir returns the directory holding OAuth credentials for a server source
// hash. Credentials belong in the config tree, not the cache tree: caches are
// disposable and get wiped by cache cleaners, tmpfiles rules and ephemeral
// containers, which used to throw away refresh tokens and force a browser
// login. Reads MCP2CLI_CONFIG_DIR at call time.
func OAuthDir(srcHash string) string {
	return filepath.Join(ConfigDir(), "oauth", srcHash)
}

// LegacyOAuthDir returns the pre-keyring location under the cache directory.
// mcp2cli reads it once to migrate existing tokens and then deletes it.
func LegacyOAuthDir(srcHash string) string {
	return filepath.Join(CacheDir(), "oauth", srcHash)
}
