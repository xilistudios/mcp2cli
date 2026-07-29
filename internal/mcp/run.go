package mcp

import (
	"context"
	"fmt"

	"github.com/xilistudios/mcp2cli/internal/cache"
)

// FetchTools connects to an MCP server, lists tools, and closes the
// connection. Used for caching the tool list.
func FetchTools(
	ctx context.Context,
	source string,
	isStdio bool,
	authHeaders [][2]string,
	envVars map[string]string,
	transport string,
) ([]map[string]any, error) {
	c, err := Connect(ctx, source, isStdio, authHeaders, envVars, transport)
	if err != nil {
		return nil, fmt.Errorf("connecting to MCP server: %w", err)
	}
	defer c.Close()

	tools, err := c.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing tools: %w", err)
	}
	return tools, nil
}

// FetchToolsCached checks the cache first (key = cacheKey+"_tools"),
// falling back to FetchTools + SaveCache on a miss or refresh.
func FetchToolsCached(
	ctx context.Context,
	cacheKey string,
	ttl int,
	refresh bool,
	source string,
	isStdio bool,
	authHeaders [][2]string,
	envVars map[string]string,
	transport string,
) ([]map[string]any, error) {
	fullKey := cacheKey + "_tools"

	if !refresh {
		if cached, ok := cache.LoadCached(fullKey, ttl); ok {
			if tools, ok := toToolMaps(cached); ok {
				return tools, nil
			}
		}
	}

	tools, err := FetchTools(ctx, source, isStdio, authHeaders, envVars, transport)
	if err != nil {
		return nil, err
	}

	if saveErr := cache.SaveCache(fullKey, tools); saveErr != nil {
		// Non-fatal: log but don't fail.
		_ = saveErr
	}
	return tools, nil
}

// toToolMaps converts the cached any value back to []map[string]any.
func toToolMaps(v any) ([]map[string]any, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		out = append(out, m)
	}
	return out, true
}
