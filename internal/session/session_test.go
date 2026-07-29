package session

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/xilistudios/mcp2cli/internal/mcp"
)

// ---------------------------------------------------------------------------
// IsAlive tests
// ---------------------------------------------------------------------------

func TestIsAlive_Self(t *testing.T) {
	if !IsAlive(os.Getpid()) {
		t.Error("expected current PID to be alive")
	}
}

func TestIsAlive_Nonexistent(t *testing.T) {
	if IsAlive(99999999) {
		t.Error("expected very large PID to not be alive")
	}
}

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestList_Empty(t *testing.T) {
	t.Setenv("MCP2CLI_CACHE_DIR", t.TempDir())
	sessions, err := List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestList_WithMeta(t *testing.T) {
	t.Setenv("MCP2CLI_CACHE_DIR", t.TempDir())
	dir := SessionsDir()
	os.MkdirAll(dir, 0755)

	meta := Meta{
		Pid:       os.Getpid(),
		Source:    "http://example.com",
		Transport: "http",
		CreatedAt: 1234567890.0,
	}
	raw, _ := json.Marshal(meta)
	os.WriteFile(metaPath("foo"), raw, 0644)

	sessions, err := List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.Name != "foo" {
		t.Errorf("Name = %q, want %q", s.Name, "foo")
	}
	if !s.Alive {
		t.Error("expected session to be alive (own PID)")
	}
	if s.Transport != "http" {
		t.Errorf("Transport = %q, want %q", s.Transport, "http")
	}
	if s.Source != "http://example.com" {
		t.Errorf("Source = %q, want %q", s.Source, "http://example.com")
	}
}

// ---------------------------------------------------------------------------
// Stop tests
// ---------------------------------------------------------------------------

func TestStop_RemovesFiles(t *testing.T) {
	t.Setenv("MCP2CLI_CACHE_DIR", t.TempDir())
	dir := SessionsDir()
	os.MkdirAll(dir, 0755)

	// Write meta with a dead PID.
	meta := Meta{Pid: 99999999, Source: "test", Transport: "stdio", CreatedAt: 1.0}
	raw, _ := json.Marshal(meta)
	mp := metaPath("bar")
	sp := sockPath("bar")
	lp := logPath("bar")
	os.WriteFile(mp, raw, 0644)
	os.WriteFile(sp, []byte{}, 0644)
	os.WriteFile(lp, []byte{}, 0644)

	if err := Stop("bar"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range []string{mp, sp, lp} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", p)
		}
	}
}

// ---------------------------------------------------------------------------
// Start tests
// ---------------------------------------------------------------------------

func TestStart_AlreadyRunning(t *testing.T) {
	t.Setenv("MCP2CLI_CACHE_DIR", t.TempDir())
	dir := SessionsDir()
	os.MkdirAll(dir, 0755)

	// Write meta with our own PID (alive).
	meta := Meta{Pid: os.Getpid(), Source: "test", Transport: "stdio", CreatedAt: 1.0}
	raw, _ := json.Marshal(meta)
	os.WriteFile(metaPath("baz"), raw, 0644)

	err := Start("baz", "test", true, nil, nil, "auto")
	if err == nil {
		t.Fatal("expected error for already running session")
	}
	if got := err.Error(); !contains(got, "already running") {
		t.Errorf("error %q does not contain 'already running'", got)
	}
}

// ---------------------------------------------------------------------------
// HandleRequest tests (with fake MCPOps)
// ---------------------------------------------------------------------------

// fakeMCPOps implements MCPOps for testing.
type fakeMCPOps struct {
	tools        []map[string]any
	resources    []map[string]any
	prompts      []map[string]any
	toolResult   *mcp.CallResult
	resourceText string
	promptResult map[string]any
}

func (f *fakeMCPOps) ListTools(_ context.Context) ([]map[string]any, error) {
	return f.tools, nil
}

func (f *fakeMCPOps) CallTool(_ context.Context, name string, arguments map[string]any) (*mcp.CallResult, error) {
	return f.toolResult, nil
}

func (f *fakeMCPOps) ListResources(_ context.Context) ([]map[string]any, error) {
	return f.resources, nil
}

func (f *fakeMCPOps) ReadResource(_ context.Context, uri string) (string, error) {
	return f.resourceText, nil
}

func (f *fakeMCPOps) ListResourceTemplates(_ context.Context) ([]map[string]any, error) {
	return nil, nil
}

func (f *fakeMCPOps) ListPrompts(_ context.Context) ([]map[string]any, error) {
	return f.prompts, nil
}

func (f *fakeMCPOps) GetPrompt(_ context.Context, name string, arguments map[string]any) (map[string]any, error) {
	return f.promptResult, nil
}

func TestHandleRequest_ListTools(t *testing.T) {
	ops := &fakeMCPOps{
		tools: []map[string]any{
			{"name": "echo", "description": "echo tool"},
		},
	}
	result, err := HandleRequest(context.Background(), ops, "list_tools", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(tools) != 1 || tools[0]["name"] != "echo" {
		t.Errorf("unexpected tools: %v", tools)
	}
}

func TestHandleRequest_CallTool(t *testing.T) {
	ops := &fakeMCPOps{
		toolResult: &mcp.CallResult{
			Content: []mcp.ContentPart{{Type: "text", Text: "hi"}},
		},
	}
	result, err := HandleRequest(context.Background(), ops, "call_tool", map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"message": "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if text != "hi" {
		t.Errorf("text = %q, want %q", text, "hi")
	}
}

func TestHandleRequest_ReadResource(t *testing.T) {
	ops := &fakeMCPOps{resourceText: "resource-data"}
	result, err := HandleRequest(context.Background(), ops, "read_resource", map[string]any{"uri": "test://uri"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "resource-data" {
		t.Errorf("result = %q, want %q", result, "resource-data")
	}
}

func TestHandleRequest_GetPrompt(t *testing.T) {
	ops := &fakeMCPOps{
		promptResult: map[string]any{"description": "test prompt"},
	}
	result, err := HandleRequest(context.Background(), ops, "get_prompt", map[string]any{"name": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["description"] != "test prompt" {
		t.Errorf("description = %v, want %q", m["description"], "test prompt")
	}
}

func TestHandleRequest_Unknown(t *testing.T) {
	ops := &fakeMCPOps{}
	_, err := HandleRequest(context.Background(), ops, "bogus_method", nil)
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
	if got := err.Error(); !contains(got, "unknown method") {
		t.Errorf("error %q does not contain 'unknown method'", got)
	}
}

// ---------------------------------------------------------------------------
// ServeConn round-trip test
// ---------------------------------------------------------------------------

func TestServeConn_RoundTrip(t *testing.T) {
	ops := &fakeMCPOps{
		tools: []map[string]any{
			{"name": "tool1", "description": "first tool"},
			{"name": "tool2", "description": "second tool"},
		},
	}

	tmp := t.TempDir()
	sockFile := filepath.Join(tmp, "test.sock")
	ln, err := net.Listen("unix", sockFile)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	ctx := context.Background()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		ServeConn(ctx, ops, conn)
	}()

	// Dial and send request.
	conn, err := net.Dial("unix", sockFile)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	reqJSON := `{"id":1,"method":"list_tools","params":{}}` + "\n"
	if _, err := conn.Write([]byte(reqJSON)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if tc, ok := conn.(*net.UnixConn); ok {
		tc.CloseWrite()
	}

	// Read response.
	var buf []byte
	tmp2 := make([]byte, 4096)
	for {
		n, readErr := conn.Read(tmp2)
		if n > 0 {
			buf = append(buf, tmp2[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	conn.Close()

	var resp map[string]any
	if err := json.Unmarshal(buf, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nraw: %s", err, string(buf))
	}
	if _, ok := resp["error"]; ok {
		t.Fatalf("response has error: %v", resp["error"])
	}
	result, ok := resp["result"].([]any)
	if !ok {
		t.Fatalf("result is not array: %T %v", resp["result"], resp["result"])
	}
	if len(result) != 2 {
		t.Errorf("expected 2 tools, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// Request_NotFound test
// ---------------------------------------------------------------------------

func TestRequest_NotFound(t *testing.T) {
	t.Setenv("MCP2CLI_CACHE_DIR", t.TempDir())
	_, err := Request("nonexistent", "list_tools", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if got := err.Error(); !contains(got, "not found") {
		t.Errorf("error %q does not contain 'not found'", got)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
