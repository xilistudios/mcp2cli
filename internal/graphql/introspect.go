// Package graphql provides GraphQL introspection, schema loading, command
// extraction, document building, and query execution for mcp2cli.
package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xilistudios/mcp2cli/internal/cache"
)

// IntrospectionQuery is the standard GraphQL introspection query used to
// discover the schema (types, fields, arguments, enums).
const IntrospectionQuery = `query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    types {
      kind
      name
      fields(includeDeprecated: false) {
        name
        description
        args {
          name
          description
          type {
            ...TypeRef
          }
          defaultValue
        }
        type {
          ...TypeRef
        }
      }
      inputFields {
        name
        description
        type {
          ...TypeRef
        }
        defaultValue
      }
      enumValues(includeDeprecated: false) {
        name
        description
      }
    }
  }
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
        }
      }
    }
  }
}
`

// LoadSchema fetches a GraphQL schema via introspection with caching support.
//
// If cacheKey is empty a deterministic key is derived from url + authHeaders.
// When refresh is false the cache is checked first; on a cache miss the
// introspection query is POSTed to url. The resulting __schema map is cached
// and returned.
//
// If client is nil a default http.Client with a 30-second timeout is used.
func LoadSchema(url string, authHeaders [][2]string, cacheKey string, ttl int, refresh bool, client *http.Client) (map[string]any, error) {
	key := cacheKey
	if key == "" {
		key = cache.CacheKeyFor(map[string]any{
			"source":       "graphql:" + url,
			"auth_headers": authHeaders,
		})
	}

	if !refresh {
		if cached, ok := cache.LoadCached(key, ttl); ok {
			if m, ok := cached.(map[string]any); ok {
				return m, nil
			}
		}
	}

	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	body, err := json.Marshal(map[string]string{"query": IntrospectionQuery})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal introspection query: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Apply auth headers; set default Content-Type.
	for _, h := range authHeaders {
		req.Header.Set(h[0], h[1])
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspection request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("introspection returned HTTP %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read introspection response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse introspection response: %w", err)
	}

	if errs, hasErrors := result["errors"]; hasErrors {
		if _, hasData := result["data"]; !hasData {
			msgs := extractErrorMessages(errs)
			return nil, fmt.Errorf("GraphQL introspection failed: %s", strings.Join(msgs, "; "))
		}
	}

	dataRaw, ok := result["data"]
	if !ok {
		return nil, fmt.Errorf("introspection returned no schema")
	}
	data, ok := dataRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("introspection returned no schema")
	}
	schemaRaw, ok := data["__schema"]
	if !ok {
		return nil, fmt.Errorf("introspection returned no schema")
	}
	schema, ok := schemaRaw.(map[string]any)
	if !ok || len(schema) == 0 {
		return nil, fmt.Errorf("introspection returned no schema")
	}

	if err := cache.SaveCache(key, schema); err != nil {
		// Non-fatal; log but don't fail.
		_ = err
	}

	return schema, nil
}

// extractErrorMessages pulls "message" strings from a GraphQL errors array.
func extractErrorMessages(errs any) []string {
	var msgs []string
	switch e := errs.(type) {
	case []any:
		for _, item := range e {
			if m, ok := item.(map[string]any); ok {
				if msg, ok := m["message"].(string); ok {
					msgs = append(msgs, msg)
				}
			}
		}
	}
	if len(msgs) == 0 {
		msgs = append(msgs, "unknown error")
	}
	return msgs
}
