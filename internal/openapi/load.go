package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xilistudios/mcp2cli/internal/cache"
	"gopkg.in/yaml.v3"
)

// LoadSpec loads an OpenAPI spec from a URL or local file path.
// URL sources are cached using the provided key and TTL (seconds).
// authHeaders are applied as HTTP headers for URL sources.
// If client is nil a default 30 s timeout client is used.
func LoadSpec(
	source string,
	authHeaders [][2]string,
	cacheKey string,
	ttl int,
	refresh bool,
	client *http.Client,
) (map[string]any, error) {
	isURL := strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")

	if isURL {
		key := cacheKey
		if key == "" {
			key = cache.CacheKeyFor(map[string]any{
				"source":       source,
				"auth_headers": authHeaders,
			})
		}
		if !refresh {
			if cached, ok := cache.LoadCached(key, ttl); ok {
				if m, ok2 := cached.(map[string]any); ok2 {
					return m, nil
				}
			}
		}

		req, err := http.NewRequest("GET", source, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		for _, h := range authHeaders {
			req.Header.Set(h[0], h[1])
		}

		if client == nil {
			client = &http.Client{Timeout: 30 * time.Second}
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching spec: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("fetching spec: HTTP %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		spec, err := parseSpec(body)
		if err != nil {
			return nil, err
		}

		spec = ResolveRefs(spec)
		cache.SaveCache(key, spec)
		return spec, nil
	}

	// Local file.
	raw, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("reading spec file: %w", err)
	}

	spec, err := parseSpec(raw)
	if err != nil {
		return nil, err
	}

	spec = ResolveRefs(spec)
	return spec, nil
}

// parseSpec attempts to unmarshal raw bytes as JSON, falling back to YAML.
func parseSpec(raw []byte) (map[string]any, error) {
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		if err := yaml.Unmarshal(raw, &spec); err != nil {
			return nil, fmt.Errorf("parsing spec: neither valid JSON nor YAML")
		}
	}
	if spec == nil {
		return nil, fmt.Errorf("spec must contain 'paths'")
	}
	if _, ok := spec["paths"]; !ok {
		return nil, fmt.Errorf("spec must contain 'paths'")
	}
	return spec, nil
}
