package bake

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/xilistudios/mcp2cli/internal/types"
)

// helper: set MCP2CLI_CONFIG_DIR to a temp dir and return it.
func withTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CONFIG_DIR", dir)
	return dir
}

// ---------------------------------------------------------------------------
// globMatch
// ---------------------------------------------------------------------------

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"*", "anything", true},
		{"*", "", true},
		{"get-*", "get-users", true},
		{"get-*", "list-users", false},
		{"?et", "get", true},
		{"?et", "set", true},
		{"?et", "get-extra", false},
		{"[ab]oo", "boo", true},
		{"[ab]oo", "aoo", true},
		{"[ab]oo", "coo", false},
		{"exact", "exact", true},
		{"exact", "Exact", false},
		{"*", "GET", true},
	}
	for _, tt := range tests {
		got := globMatch(tt.pattern, tt.name)
		if got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FilterCommands
// ---------------------------------------------------------------------------

func TestFilterCommands_Methods(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "get-users", Method: "get"},
		{Name: "create-user", Method: "post"},
		{Name: "mcp-tool", Method: ""}, // MCP — always passes
	}
	got := FilterCommands(cmds, nil, nil, []string{"GET"})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	names := []string{got[0].Name, got[1].Name}
	sort.Strings(names)
	if names[0] != "get-users" || names[1] != "mcp-tool" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestFilterCommands_Include(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "get-users", Method: "get"},
		{Name: "list-users", Method: "get"},
		{Name: "create-user", Method: "post"},
	}
	got := FilterCommands(cmds, []string{"get-*", "create-*"}, nil, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestFilterCommands_Exclude(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "get-users", Method: "get"},
		{Name: "internal-debug", Method: "get"},
	}
	got := FilterCommands(cmds, nil, []string{"internal-*"}, nil)
	if len(got) != 1 || got[0].Name != "get-users" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestFilterCommands_Combined(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "get-users", Method: "get"},
		{Name: "get-posts", Method: "get"},
		{Name: "delete-users", Method: "delete"},
		{Name: "internal-list", Method: "get"},
	}
	// methods=get, include=get-*, exclude=*users*
	got := FilterCommands(cmds, []string{"get-*"}, []string{"*users*"}, []string{"GET"})
	if len(got) != 1 || got[0].Name != "get-posts" {
		t.Errorf("unexpected result: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Config roundtrip
// ---------------------------------------------------------------------------

func TestConfigRoundtrip(t *testing.T) {
	withTempConfig(t)

	orig := map[string]Config{
		"my-api": {
			SourceType:  "spec",
			Source:      "https://example.com/openapi.json",
			BaseURL:     "https://api.example.com",
			AuthHeaders: [][2]string{{"Authorization", "Bearer tok123"}},
			EnvVars:     map[string]string{"FOO": "bar"},
			CacheTTL:    3600,
			Transport:   "sse",
			OAuth:       true,
			Include:     []string{"get-*"},
			Exclude:     []string{"internal-*"},
			Methods:     []string{"GET", "POST"},
			Description: "My API",
		},
	}
	if err := SaveAll(orig); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}

	loaded, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded))
	}
	cfg := loaded["my-api"]
	if cfg.SourceType != "spec" {
		t.Errorf("SourceType = %q", cfg.SourceType)
	}
	if cfg.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if len(cfg.AuthHeaders) != 1 || cfg.AuthHeaders[0][0] != "Authorization" {
		t.Errorf("AuthHeaders = %v", cfg.AuthHeaders)
	}
	if cfg.CacheTTL != 3600 {
		t.Errorf("CacheTTL = %d", cfg.CacheTTL)
	}
	if !cfg.OAuth {
		t.Error("OAuth should be true")
	}

	// Load single.
	single, ok, err := Load("my-api")
	if err != nil || !ok {
		t.Fatalf("Load: %v, %v", err, ok)
	}
	if single.Source != "https://example.com/openapi.json" {
		t.Errorf("Source = %q", single.Source)
	}

	// Missing.
	_, ok, err = Load("nonexistent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ok {
		t.Error("expected !ok for missing")
	}
}

// ---------------------------------------------------------------------------
// BakedToArgv
// ---------------------------------------------------------------------------

func TestBakedToArgv_Full(t *testing.T) {
	c := Config{
		SourceType:        "mcp",
		Source:            "http://localhost:3000",
		BaseURL:           "https://api.example.com",
		AuthHeaders:       [][2]string{{"X-API-Key", "secret123"}},
		EnvVars:           map[string]string{"FOO": "bar"},
		CacheTTL:          3600,
		Transport:         "streamable",
		OAuth:             true,
		OAuthClientID:     "cid",
		OAuthClientSecret: "csec",
		OAuthClientName:   "custom-app",
		OAuthScope:        "read write",
		OAuthRedirectURI:  "https://example.com/callback",
		OAuthFlow:         "authorization_code",
	}
	got := BakedToArgv(c)
	want := []string{
		"--mcp", "http://localhost:3000",
		"--base-url", "https://api.example.com",
		"--auth-header", "X-API-Key:secret123",
		"--env", "FOO=bar",
		"--cache-ttl", "3600",
		"--transport", "streamable",
		"--oauth",
		"--oauth-client-id", "cid",
		"--oauth-client-secret", "csec",
		"--oauth-client-name", "custom-app",
		"--oauth-scope", "read write",
		"--oauth-redirect-uri", "https://example.com/callback",
		"--oauth-flow", "authorization_code",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BakedToArgv mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestBakedToArgv_Minimal(t *testing.T) {
	c := Config{SourceType: "spec", Source: "spec.json"}
	got := BakedToArgv(c)
	want := []string{"--spec", "spec.json"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBakedToArgv_SkipsAutoTransport(t *testing.T) {
	c := Config{SourceType: "spec", Source: "s", Transport: "auto"}
	got := BakedToArgv(c)
	for _, a := range got {
		if a == "--transport" {
			t.Error("--transport auto should be skipped")
		}
	}
}

func TestBakedToArgv_SkipsDefaultOAuthName(t *testing.T) {
	c := Config{SourceType: "spec", Source: "s", OAuthClientName: "mcp2cli"}
	got := BakedToArgv(c)
	for _, a := range got {
		if a == "--oauth-client-name" {
			t.Error("--oauth-client-name mcp2cli should be skipped")
		}
	}
}

func TestBakedToArgv_McpStdio(t *testing.T) {
	c := Config{SourceType: "mcp_stdio", Source: "/usr/bin/server"}
	got := BakedToArgv(c)
	want := []string{"--mcp-stdio", "/usr/bin/server"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestCreate_BadName(t *testing.T) {
	withTempConfig(t)
	err := Create("BadName", Config{}, false)
	if err == nil {
		t.Fatal("expected error for bad name")
	}
}

func TestCreate_OK(t *testing.T) {
	withTempConfig(t)
	err := Create("my-api", Config{SourceType: "spec", Source: "x"}, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	all, _ := LoadAll()
	if _, ok := all["my-api"]; !ok {
		t.Error("my-api not found after create")
	}
}

func TestCreate_ExistsNoForce(t *testing.T) {
	withTempConfig(t)
	Create("my-api", Config{SourceType: "spec", Source: "x"}, false)
	err := Create("my-api", Config{SourceType: "spec", Source: "y"}, false)
	if err == nil {
		t.Fatal("expected error for existing without force")
	}
}

func TestCreate_ExistsForce(t *testing.T) {
	withTempConfig(t)
	Create("my-api", Config{SourceType: "spec", Source: "x"}, false)
	err := Create("my-api", Config{SourceType: "spec", Source: "y"}, true)
	if err != nil {
		t.Fatalf("Create with force: %v", err)
	}
	all, _ := LoadAll()
	if all["my-api"].Source != "y" {
		t.Error("source not updated with force")
	}
}

// ---------------------------------------------------------------------------
// Remove
// ---------------------------------------------------------------------------

func TestRemove_Missing(t *testing.T) {
	withTempConfig(t)
	err := Remove("nope")
	if err == nil {
		t.Fatal("expected error for missing")
	}
}

func TestRemove_Existing(t *testing.T) {
	withTempConfig(t)
	Create("my-api", Config{SourceType: "spec"}, false)
	err := Remove("my-api")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	all, _ := LoadAll()
	if _, ok := all["my-api"]; ok {
		t.Error("still present after remove")
	}
}

func TestRemove_AlsoRemovesWrapper(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	withTempConfig(t)

	// Pre-create a fake wrapper.
	binDir := filepath.Join(tmpHome, ".local", "bin")
	os.MkdirAll(binDir, 0o755)
	wrapper := filepath.Join(binDir, "my-api")
	os.WriteFile(wrapper, []byte("#!/bin/sh\n"), 0o755)

	Create("my-api", Config{SourceType: "spec"}, false)
	if err := Remove("my-api"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(wrapper); !os.IsNotExist(err) {
		t.Error("wrapper should have been removed")
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestUpdate_Partial(t *testing.T) {
	withTempConfig(t)
	Create("api", Config{
		SourceType:  "spec",
		Source:      "spec.json",
		CacheTTL:    100,
		Description: "old",
	}, false)

	newTTL := 999
	newMethods := []string{"get", "post"}
	err := Update("api", UpdateOptions{
		CacheTTL: &newTTL,
		Methods:  &newMethods,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	all, _ := LoadAll()
	cfg := all["api"]
	if cfg.CacheTTL != 999 {
		t.Errorf("CacheTTL = %d", cfg.CacheTTL)
	}
	if len(cfg.Methods) != 2 || cfg.Methods[0] != "GET" || cfg.Methods[1] != "POST" {
		t.Errorf("Methods = %v (should be uppercased)", cfg.Methods)
	}
	if cfg.Description != "old" {
		t.Error("Description should not have changed")
	}
	if cfg.Source != "spec.json" {
		t.Error("Source should not have changed")
	}
}

func TestUpdate_Missing(t *testing.T) {
	withTempConfig(t)
	err := Update("nope", UpdateOptions{})
	if err == nil {
		t.Fatal("expected error for missing")
	}
}

// ---------------------------------------------------------------------------
// Show
// ---------------------------------------------------------------------------

func TestShow_MasksSecrets(t *testing.T) {
	withTempConfig(t)
	Create("api", Config{
		SourceType:  "spec",
		Source:      "spec.json",
		AuthHeaders: [][2]string{{"Authorization", "Bearer sk-abc123xyz"}},
	}, false)

	display, err := Show("api")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	headers, ok := display["auth_headers"].([]any)
	if !ok || len(headers) != 1 {
		t.Fatalf("unexpected auth_headers: %v", display["auth_headers"])
	}
	pair, ok := headers[0].([]any)
	if !ok || len(pair) < 2 {
		t.Fatalf("unexpected pair: %v", headers[0])
	}
	masked, _ := pair[1].(string)
	if masked != "Bear****" {
		t.Errorf("masked value = %q, want %q", masked, "Bear****")
	}
}

func TestShow_KeepsEnvRef(t *testing.T) {
	withTempConfig(t)
	Create("api", Config{
		SourceType:  "spec",
		Source:      "spec.json",
		AuthHeaders: [][2]string{{"Authorization", "env:MY_TOKEN"}},
	}, false)

	display, _ := Show("api")
	headers := display["auth_headers"].([]any)
	pair := headers[0].([]any)
	val, _ := pair[1].(string)
	if val != "env:MY_TOKEN" {
		t.Errorf("env: ref should not be masked, got %q", val)
	}
}

func TestShow_KeepsFileRef(t *testing.T) {
	withTempConfig(t)
	Create("api", Config{
		SourceType:  "spec",
		Source:      "spec.json",
		AuthHeaders: [][2]string{{"Authorization", "file:/tmp/secret"}},
	}, false)

	display, _ := Show("api")
	headers := display["auth_headers"].([]any)
	pair := headers[0].([]any)
	val, _ := pair[1].(string)
	if val != "file:/tmp/secret" {
		t.Errorf("file: ref should not be masked, got %q", val)
	}
}

func TestShow_ShortSecret(t *testing.T) {
	withTempConfig(t)
	Create("api", Config{
		SourceType:  "spec",
		Source:      "spec.json",
		AuthHeaders: [][2]string{{"X-Key", "ab"}},
	}, false)

	display, _ := Show("api")
	headers := display["auth_headers"].([]any)
	pair := headers[0].([]any)
	val, _ := pair[1].(string)
	if val != "****" {
		t.Errorf("short secret masked to %q, want %q", val, "****")
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestList_Empty(t *testing.T) {
	withTempConfig(t)
	var buf bytes.Buffer
	List(&buf)
	if buf.String() != "No baked tools.\n" {
		t.Errorf("unexpected: %q", buf.String())
	}
}

func TestList_WithEntries(t *testing.T) {
	withTempConfig(t)
	Create("alpha", Config{SourceType: "spec", Source: "spec.json"}, false)
	Create("beta", Config{SourceType: "mcp", Source: "http://localhost:3000"}, false)
	var buf bytes.Buffer
	List(&buf)
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("alpha")) || !bytes.Contains(buf.Bytes(), []byte("beta")) {
		t.Errorf("missing entries in output:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Install
// ---------------------------------------------------------------------------

func TestInstall(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	withTempConfig(t)

	Create("my-tool", Config{SourceType: "spec", Source: "spec.json"}, false)

	dir := t.TempDir()
	path, err := Install("my-tool", dir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	expected := filepath.Join(dir, "my-tool")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(raw)
	if !bytes.HasPrefix(raw, []byte("#!/bin/sh\n")) {
		t.Error("wrapper should start with #!/bin/sh")
	}
	if !bytes.Contains(raw, []byte("@my-tool")) {
		t.Error("wrapper should contain @my-tool")
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm()&0o111 == 0 {
		t.Error("wrapper should be executable")
	}

	_ = content // suppress unused
}

func TestInstall_Missing(t *testing.T) {
	withTempConfig(t)
	_, err := Install("nope", t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

// ---------------------------------------------------------------------------
// SaveAll / LoadAll: missing file → empty map
// ---------------------------------------------------------------------------

func TestLoadAll_MissingFile(t *testing.T) {
	withTempConfig(t)
	all, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected empty, got %d", len(all))
	}
}

func TestLoadAll_CorruptFile(t *testing.T) {
	dir := withTempConfig(t)
	os.WriteFile(filepath.Join(dir, "baked.json"), []byte("NOT JSON{{{"), 0o644)
	all, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on corrupt: %v", err)
	}
	if len(all) != 0 {
		t.Error("expected empty map for corrupt file")
	}
}

// Ensure JSON round-trip preserves all fields.
func TestJSONRoundTrip(t *testing.T) {
	c := Config{
		SourceType:  "spec",
		Source:      "spec.json",
		AuthHeaders: [][2]string{{"A", "B"}, {"C", "D"}},
		EnvVars:     map[string]string{"X": "1"},
	}
	raw, _ := json.Marshal(c)
	var c2 Config
	json.Unmarshal(raw, &c2)
	if !reflect.DeepEqual(c, c2) {
		t.Errorf("JSON round-trip mismatch:\n orig: %+v\n  got: %+v", c, c2)
	}
}
