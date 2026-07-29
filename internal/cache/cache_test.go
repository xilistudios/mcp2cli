package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", dir)
	return dir
}

func setConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CONFIG_DIR", dir)
	return dir
}

// ---------------------------------------------------------------------------
// CacheKeyFor
// ---------------------------------------------------------------------------

func TestCacheKeyFor_StableAcrossCalls(t *testing.T) {
	cfg := map[string]any{
		"spec":     "https://example.com/openapi.json",
		"base_url": "https://example.com/api",
	}
	k1 := CacheKeyFor(cfg)
	k2 := CacheKeyFor(cfg)
	if k1 != k2 {
		t.Fatalf("expected stable key, got %q then %q", k1, k2)
	}
	if len(k1) != 16 {
		t.Fatalf("expected 16-char hex key, got %d chars: %q", len(k1), k1)
	}
}

func TestCacheKeyFor_DifferentWhenRelevantFieldChanges(t *testing.T) {
	cfg1 := map[string]any{"spec": "https://a.example.com"}
	cfg2 := map[string]any{"spec": "https://b.example.com"}
	if CacheKeyFor(cfg1) == CacheKeyFor(cfg2) {
		t.Fatal("different specs should produce different keys")
	}
}

func TestCacheKeyFor_SameWhenOnlyExcludedFieldsChange(t *testing.T) {
	base := map[string]any{"spec": "https://example.com"}
	modified := map[string]any{
		"spec":        "https://example.com",
		"cache_ttl":   9999,
		"description": "changed",
		"include":     []string{"a"},
		"exclude":     []string{"b"},
		"methods":     []string{"GET"},
	}
	if CacheKeyFor(base) != CacheKeyFor(modified) {
		t.Fatal("changing only excluded fields should not change the key")
	}
}

func TestCacheKeyFor_AuthHeadersOrderIndependent(t *testing.T) {
	cfg1 := map[string]any{
		"spec": "https://example.com",
		"auth_headers": []any{
			[]any{"Authorization", "Bearer abc"},
			[]any{"X-Custom", "val"},
		},
	}
	cfg2 := map[string]any{
		"spec": "https://example.com",
		"auth_headers": []any{
			[]any{"X-Custom", "val"},
			[]any{"Authorization", "Bearer abc"},
		},
	}
	if CacheKeyFor(cfg1) != CacheKeyFor(cfg2) {
		t.Fatal("auth_headers order should not affect the key")
	}
}

func TestCacheKeyFor_AuthHeadersTypedSlices(t *testing.T) {
	cfg1 := map[string]any{
		"spec":         "https://example.com",
		"auth_headers": [][2]string{{"Authorization", "Bearer abc"}, {"X-Custom", "val"}},
	}
	cfg2 := map[string]any{
		"spec":         "https://example.com",
		"auth_headers": [][2]string{{"X-Custom", "val"}, {"Authorization", "Bearer abc"}},
	}
	if CacheKeyFor(cfg1) != CacheKeyFor(cfg2) {
		t.Fatal("[][2]string auth_headers order should not affect the key")
	}
}

// ---------------------------------------------------------------------------
// SaveCache / LoadCached
// ---------------------------------------------------------------------------

func TestSaveLoadCached_Roundtrip(t *testing.T) {
	setCacheDir(t)
	setConfigDir(t)

	data := map[string]any{"tools": []string{"a", "b"}}
	if err := SaveCache("testkey", data); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	got, ok := LoadCached("testkey", DefaultCacheTTL)
	if !ok {
		t.Fatal("LoadCached should return true for a fresh cache")
	}
	gotMap, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if gotMap["tools"] == nil {
		t.Fatal("expected 'tools' key in cached data")
	}
}

func TestLoadCached_MissingFile(t *testing.T) {
	setCacheDir(t)
	_, ok := LoadCached("nonexistent", DefaultCacheTTL)
	if ok {
		t.Fatal("LoadCached should return false for missing file")
	}
}

func TestLoadCached_Expired(t *testing.T) {
	dir := setCacheDir(t)

	data := map[string]any{"x": 1}
	if err := SaveCache("expired", data); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	// Set mtime to 2 hours ago so ttl=0 means expired.
	path := filepath.Join(dir, "expired.json")
	old := time.Now().Add(-7200 * time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("os.Chtimes: %v", err)
	}

	_, ok := LoadCached("expired", 0)
	if ok {
		t.Fatal("LoadCached should return false for expired cache (ttl=0)")
	}

	// But with a very large TTL it should still work.
	got, ok := LoadCached("expired", 99999)
	if !ok {
		t.Fatal("LoadCached should return true with large TTL")
	}
	if got == nil {
		t.Fatal("expected non-nil data")
	}
}
