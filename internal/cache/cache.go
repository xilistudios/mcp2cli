package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CacheKeyFor generates a deterministic cache key from a configuration map.
//
// It excludes keys that do not affect which tools/specs are returned
// (cache_ttl, description, include, exclude, methods). If auth_headers is
// present, its entries are sorted before hashing so that header order does
// not affect the key.
//
// NOTE: exact hash parity with the Python implementation is NOT guaranteed;
// determinism and stability across calls IS guaranteed.
func CacheKeyFor(config map[string]any) string {
	// Build a copy excluding irrelevant keys.
	exclude := map[string]bool{
		"cache_ttl":   true,
		"description": true,
		"include":     true,
		"exclude":     true,
		"methods":     true,
	}
	filtered := make(map[string]any, len(config))
	for k, v := range config {
		if exclude[k] {
			continue
		}
		filtered[k] = v
	}

	// Normalise auth_headers for stable hashing.
	if ah, ok := filtered["auth_headers"]; ok && ah != nil {
		filtered["auth_headers"] = normaliseAuthHeaders(ah)
	}

	// encoding/json marshals map keys in sorted order, so the JSON is
	// deterministic as long as the values are deterministic.
	raw, _ := json.Marshal(filtered)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:16]
}

// normaliseAuthHeaders converts auth_headers from any of the supported
// representations ([][2]string, [][]string, []any) into a sorted
// [][]string for stable hashing.
func normaliseAuthHeaders(v any) [][]string {
	var pairs [][]string

	switch val := v.(type) {
	case [][2]string:
		for _, p := range val {
			pairs = append(pairs, []string{p[0], p[1]})
		}
	case [][]string:
		pairs = val
	case []any:
		for _, item := range val {
			switch p := item.(type) {
			case [2]string:
				pairs = append(pairs, []string{p[0], p[1]})
			case []string:
				pairs = append(pairs, p)
			case []any:
				if len(p) >= 2 {
					a, _ := p[0].(string)
					b, _ := p[1].(string)
					pairs = append(pairs, []string{a, b})
				}
			}
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	return pairs
}

// LoadCached loads a cached JSON file identified by key. It returns
// (data, true) if the file exists, is younger than ttl seconds, and
// decodes successfully. Otherwise it returns (nil, false).
//
// ttl <= 0 means the cache is always expired.
func LoadCached(key string, ttl int) (any, bool) {
	path := filepath.Join(CacheDir(), key+".json")
	fi, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	age := time.Since(fi.ModTime()).Seconds()
	if int(age) >= ttl {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, false
	}
	return data, true
}

// SaveCache persists data as pretty-printed JSON under
// CacheDir()/key+".json". It creates the cache directory if needed.
func SaveCache(key string, data any) error {
	dir := CacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, key+".json"), raw, 0o644)
}
