package graphql

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIntrospectionQueryContainsSchema(t *testing.T) {
	if len(IntrospectionQuery) < 100 {
		t.Fatal("IntrospectionQuery is too short")
	}
	// Must contain the expected fragments
	for _, want := range []string{
		"query IntrospectionQuery",
		"__schema",
		"queryType { name }",
		"mutationType { name }",
		"fragment TypeRef on __Type",
		"kind",
		"ofType",
	} {
		if !contains(IntrospectionQuery, want) {
			t.Errorf("IntrospectionQuery missing %q", want)
		}
	}
}

func TestLoadSchema_CacheHit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", dir)

	saved := map[string]any{"queryType": map[string]any{"name": "Query"}}
	raw, _ := json.Marshal(saved)
	os.WriteFile(filepath.Join(dir, "testkey.json"), raw, 0o644)

	schema, err := LoadSchema("http://unused", nil, "testkey", 3600, false, nil)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if schema["queryType"] == nil {
		t.Error("expected queryType in cached schema")
	}
}

func TestLoadSchema_CacheMiss_Refresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", dir)

	// Write a "stale" cache file that should be ignored with refresh=true
	stale := map[string]any{"stale": true}
	raw, _ := json.Marshal(stale)
	os.WriteFile(filepath.Join(dir, "refkey.json"), raw, 0o644)

	schemaData := map[string]any{
		"queryType":    map[string]any{"name": "Query"},
		"mutationType": map[string]any{"name": "Mutation"},
		"types":        []any{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's a POST with the introspection query
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		json.Unmarshal(body, &req)
		if !contains(req["query"], "IntrospectionQuery") {
			t.Error("expected introspection query in body")
		}

		resp := map[string]any{"data": map[string]any{"__schema": schemaData}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	schema, err := LoadSchema(srv.URL, nil, "refkey", 3600, true, nil)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if schema["stale"] != nil {
		t.Error("should not have returned stale cache")
	}
	if schema["queryType"] == nil {
		t.Error("expected fresh schema data")
	}
}

func TestLoadSchema_AuthHeaders(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", dir)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		resp := map[string]any{"data": map[string]any{"__schema": map[string]any{"queryType": map[string]any{"name": "Q"}}}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	_, err := LoadSchema(srv.URL, [][2]string{{"Authorization", "Bearer tok123"}}, "", 3600, true, nil)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("expected auth header, got %q", gotAuth)
	}
}

func TestLoadSchema_HTTPError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := LoadSchema(srv.URL, nil, "", 3600, true, nil)
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
}

func TestLoadSchema_GraphQLErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"errors": []any{
				map[string]any{"message": "Not authorized"},
				map[string]any{"message": "Field not found"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	_, err := LoadSchema(srv.URL, nil, "", 3600, true, nil)
	if err == nil {
		t.Fatal("expected error for GraphQL errors")
	}
	if !contains(err.Error(), "Not authorized") {
		t.Errorf("expected error message, got: %v", err)
	}
}

func TestLoadSchema_PartialErrorsWithData(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data":   map[string]any{"__schema": map[string]any{"queryType": map[string]any{"name": "Q"}}},
			"errors": []any{map[string]any{"message": "partial"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	schema, err := LoadSchema(srv.URL, nil, "", 3600, true, nil)
	if err != nil {
		t.Fatalf("should succeed with partial errors: %v", err)
	}
	if schema["queryType"] == nil {
		t.Error("expected schema data")
	}
}

func TestLoadSchema_NoSchema(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"data": map[string]any{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	_, err := LoadSchema(srv.URL, nil, "", 3600, true, nil)
	if err == nil {
		t.Fatal("expected error for missing schema")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
