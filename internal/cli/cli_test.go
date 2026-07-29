package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/xilistudios/mcp2cli/internal/cache"
	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// ---------------------------------------------------------------------------
// TruncateDescription
// ---------------------------------------------------------------------------

func TestTruncateDescription_Short(t *testing.T) {
	got := TruncateDescription("hello", 10)
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestTruncateDescription_Long(t *testing.T) {
	got := TruncateDescription("this is a long description that should be truncated", 20)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected suffix '...', got %q", got)
	}
	if len(got) > 23 { // maxLen + "..."
		t.Errorf("got too long: %q (%d chars)", got, len(got))
	}
	// Should cut at a space boundary.
	if strings.Contains(got, "that") {
		t.Errorf("expected to be truncated before 'that', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// WrapDescription
// ---------------------------------------------------------------------------

func TestWrapDescription_Basic(t *testing.T) {
	got := WrapDescription("one two three four five", 4, 12)
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Errorf("expected multiple lines, got %q", got)
	}
	// Continuation lines should be indented.
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l, "    ") {
			t.Errorf("continuation line missing indent: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// FilterCommandsSearch
// ---------------------------------------------------------------------------

func TestFilterCommandsSearch(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "get-users", Description: "List all users"},
		{Name: "create-user", Description: "Create a new user"},
		{Name: "delete-post", Description: "Remove a post"},
	}

	got := FilterCommandsSearch(cmds, "user")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}

	got = FilterCommandsSearch(cmds, "POST")
	if len(got) != 1 || got[0].Name != "delete-post" {
		t.Errorf("expected [delete-post], got %v", namesOf(got))
	}

	got = FilterCommandsSearch(cmds, "nonexistent")
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// ApplyListOptions
// ---------------------------------------------------------------------------

func TestApplyListOptions(t *testing.T) {
	t.Setenv("MCP2CLI_CACHE_DIR", t.TempDir())

	cmds := []types.CommandDef{
		{Name: "beta"},
		{Name: "alpha"},
		{Name: "gamma"},
	}

	got := ApplyListOptions(cmds, "testhash", "alpha", 0)
	if got[0].Name != "alpha" || got[1].Name != "beta" || got[2].Name != "gamma" {
		t.Errorf("expected alpha,beta,gamma order, got %v", namesOf(got))
	}

	got = ApplyListOptions(cmds, "testhash", "alpha", 2)
	if len(got) != 2 {
		t.Errorf("expected top 2, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// ListOpenAPI
// ---------------------------------------------------------------------------

func TestListOpenAPI_ContainsGroupHeaderAndNames(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "get-users", Method: "get", Description: "List users"},
		{Name: "create-user", Method: "post", Description: "Create user"},
	}
	var buf bytes.Buffer
	o := ListOptions{Out: &buf, SourceHash: "test"}
	ListOpenAPI(cmds, o)
	out := buf.String()
	if !strings.Contains(out, "get-users") {
		t.Errorf("output missing get-users:\n%s", out)
	}
	if !strings.Contains(out, "create-user") {
		t.Errorf("output missing create-user:\n%s", out)
	}
	// Should have a group header like "get:" or "create:".
	if !strings.Contains(out, ":") {
		t.Errorf("output missing group header:\n%s", out)
	}
}

func TestListOpenAPI_Compact(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "a-cmd"},
		{Name: "b-cmd"},
	}
	var buf bytes.Buffer
	o := ListOptions{Out: &buf, Compact: true, SourceHash: "test"}
	ListOpenAPI(cmds, o)
	out := strings.TrimSpace(buf.String())
	if out != "a-cmd b-cmd" {
		t.Errorf("expected 'a-cmd b-cmd', got %q", out)
	}
}

func TestListOpenAPI_JSON(t *testing.T) {
	// Redirect util.Out to capture JSON output.
	old := util.Out
	defer func() { util.Out = old }()
	var buf bytes.Buffer
	util.Out = &buf

	cmds := []types.CommandDef{
		{Name: "x", Method: "get"},
	}
	o := ListOptions{JSONOutput: true, SourceHash: "test"}
	ListOpenAPI(cmds, o)

	var arr []any
	if err := json.Unmarshal(buf.Bytes(), &arr); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if len(arr) != 1 {
		t.Errorf("expected 1 item, got %d", len(arr))
	}
}

// ---------------------------------------------------------------------------
// ListMCP
// ---------------------------------------------------------------------------

func TestListMCP_ContainsNames(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "tool-a", Description: "A tool"},
		{Name: "tool-b", Description: "B tool"},
	}
	var buf bytes.Buffer
	o := ListOptions{Out: &buf, SourceHash: "test"}
	ListMCP(cmds, o)
	out := buf.String()
	if !strings.Contains(out, "tool-a") || !strings.Contains(out, "tool-b") {
		t.Errorf("expected both tool names, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// ListGraphQL
// ---------------------------------------------------------------------------

func TestListGraphQL_GroupsByType(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "find-user", GraphQLOperationType: "query", Description: "Find a user"},
		{Name: "update-user", GraphQLOperationType: "mutation", Description: "Update a user"},
	}
	var buf bytes.Buffer
	o := ListOptions{Out: &buf, SourceHash: "test"}
	ListGraphQL(cmds, o)
	out := buf.String()
	if !strings.Contains(out, "queries:") {
		t.Errorf("expected 'queries:' header, got:\n%s", out)
	}
	if !strings.Contains(out, "mutations:") {
		t.Errorf("expected 'mutations:' header, got:\n%s", out)
	}
	if !strings.Contains(out, "find-user") || !strings.Contains(out, "update-user") {
		t.Errorf("expected both names, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// SplitAtSubcommand
// ---------------------------------------------------------------------------

func TestSplitAtSubcommand(t *testing.T) {
	vo := map[string]bool{"--spec": true, "--cache-ttl": true}
	bo := map[string]bool{"--list": true, "--refresh": true}

	tests := []struct {
		name       string
		argv       []string
		wantGlobal []string
		wantTool   []string
	}{
		{
			name:       "mixed",
			argv:       []string{"--spec", "x.json", "--list", "get-users", "--limit", "5"},
			wantGlobal: []string{"--spec", "x.json", "--list"},
			wantTool:   []string{"get-users", "--limit", "5"},
		},
		{
			name:       "double-dash",
			argv:       []string{"--", "--spec", "x"},
			wantGlobal: []string{},
			wantTool:   []string{"--spec", "x"},
		},
		{
			name:       "equals",
			argv:       []string{"--spec=x.json", "cmd"},
			wantGlobal: []string{"--spec=x.json"},
			wantTool:   []string{"cmd"},
		},
		{
			name:       "tool-arg-not-consumed",
			argv:       []string{"--refresh", "cmd", "--env", "FOO=bar"},
			wantGlobal: []string{"--refresh"},
			wantTool:   []string{"cmd", "--env", "FOO=bar"},
		},
		{
			name:       "no-positional",
			argv:       []string{"--spec", "s", "--list"},
			wantGlobal: []string{"--spec", "s", "--list"},
			wantTool:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global, tool := SplitAtSubcommand(tt.argv, vo, bo)
			if !strSliceEq(global, tt.wantGlobal) {
				t.Errorf("global: got %v, want %v", global, tt.wantGlobal)
			}
			if !strSliceEq(tool, tt.wantTool) {
				t.Errorf("tool: got %v, want %v", tool, tt.wantTool)
			}
		})
	}
}

func strSliceEq(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// NewGlobalFlagSet
// ---------------------------------------------------------------------------

func TestNewGlobalFlagSet(t *testing.T) {
	var g GlobalFlags
	fs, valueOpts, boolOpts := NewGlobalFlagSet(&g)

	err := fs.Parse([]string{
		"--spec", "s",
		"--auth-header", "A:b",
		"--auth-header", "C:d",
		"--cache-ttl", "60",
		"--list",
		"--json",
	})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if g.Spec != "s" {
		t.Errorf("Spec = %q, want %q", g.Spec, "s")
	}
	if len(g.AuthHeaders) != 2 || g.AuthHeaders[0] != "A:b" || g.AuthHeaders[1] != "C:d" {
		t.Errorf("AuthHeaders = %v, want [A:b C:d]", g.AuthHeaders)
	}
	if g.CacheTTL != 60 {
		t.Errorf("CacheTTL = %d, want 60", g.CacheTTL)
	}
	if !g.ListCommands {
		t.Errorf("ListCommands = false, want true")
	}
	if !g.JSONOutput {
		t.Errorf("JSONOutput = false, want true")
	}

	// Defaults
	if g.OAuthClientName != "mcp2cli" {
		t.Errorf("OAuthClientName = %q, want %q", g.OAuthClientName, "mcp2cli")
	}
	if g.Transport != "auto" {
		t.Errorf("Transport = %q, want %q", g.Transport, "auto")
	}
	if g.OAuthFlow != "auto" {
		t.Errorf("OAuthFlow = %q, want %q", g.OAuthFlow, "auto")
	}
	if g.CacheTTL != 60 {
		t.Errorf("CacheTTL = %d, want 60 (overridden)", g.CacheTTL)
	}

	// Check valueOpts contains expected keys.
	for _, name := range []string{"--spec", "--cache-ttl", "--auth-header"} {
		if !valueOpts[name] {
			t.Errorf("valueOpts missing %q", name)
		}
	}
	// Check boolOpts contains expected keys.
	for _, name := range []string{"--list", "--json", "--refresh"} {
		if !boolOpts[name] {
			t.Errorf("boolOpts missing %q", name)
		}
	}
}

func TestNewGlobalFlagSet_Defaults(t *testing.T) {
	var g GlobalFlags
	_, _, _ = NewGlobalFlagSet(&g)

	// Default CacheTTL when not overridden by parse.
	if g.CacheTTL != cache.DefaultCacheTTL {
		t.Errorf("default CacheTTL = %d, want %d", g.CacheTTL, cache.DefaultCacheTTL)
	}
}

// ---------------------------------------------------------------------------
// ParseCommandArgs
// ---------------------------------------------------------------------------

func TestParseCommandArgs(t *testing.T) {
	cmd := types.CommandDef{
		Name:    "test-cmd",
		HasBody: false,
		Params: []types.ParamDef{
			{Name: "id", Type: types.TypeInt, Required: true, Location: "path", Schema: map[string]any{"type": "integer"}},
			{Name: "verbose", Type: types.TypeBoolean, Location: "tool_input", Schema: map[string]any{"type": "boolean"}},
			{Name: "name", Type: types.TypeString, Location: "query", Choices: []string{"a", "b"}, Schema: map[string]any{"type": "string"}},
			{Name: "tags", Type: types.TypeString, Location: "body", Schema: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
		},
	}

	vals, stdin, err := ParseCommandArgs(cmd, []string{"--id", "7", "--verbose", "--name", "a", "--tags", "x,y"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdin {
		t.Errorf("hasStdin = true, want false")
	}
	if vals["id"] != 7 {
		t.Errorf("id = %v (%T), want 7", vals["id"], vals["id"])
	}
	if vals["verbose"] != true {
		t.Errorf("verbose = %v, want true", vals["verbose"])
	}
	if vals["name"] != "a" {
		t.Errorf("name = %v, want 'a'", vals["name"])
	}
	// tags should be coerced to []any{"x","y"}.
	arr, ok := vals["tags"].([]any)
	if !ok {
		t.Fatalf("tags type = %T, want []any", vals["tags"])
	}
	if len(arr) != 2 || arr[0] != "x" || arr[1] != "y" {
		t.Errorf("tags = %v, want [x y]", arr)
	}
}

func TestParseCommandArgs_RequiredMissing(t *testing.T) {
	cmd := types.CommandDef{
		Name: "test-cmd",
		Params: []types.ParamDef{
			{Name: "id", Type: types.TypeInt, Required: true, Location: "path", Schema: map[string]any{"type": "integer"}},
		},
	}
	_, _, err := ParseCommandArgs(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
	if !strings.Contains(err.Error(), "--id") {
		t.Errorf("error should mention --id: %v", err)
	}
}

func TestParseCommandArgs_InvalidChoice(t *testing.T) {
	cmd := types.CommandDef{
		Name: "test-cmd",
		Params: []types.ParamDef{
			{Name: "name", Type: types.TypeString, Location: "query", Choices: []string{"a", "b"}, Schema: map[string]any{"type": "string"}},
		},
	}
	_, _, err := ParseCommandArgs(cmd, []string{"--name", "z"})
	if err == nil {
		t.Fatal("expected error for invalid choice")
	}
	if !strings.Contains(err.Error(), "invalid choice") {
		t.Errorf("error should mention invalid choice: %v", err)
	}
}

func TestParseCommandArgs_Stdin(t *testing.T) {
	cmd := types.CommandDef{
		Name:    "test-cmd",
		HasBody: true,
	}
	_, stdin, err := ParseCommandArgs(cmd, []string{"--stdin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stdin {
		t.Errorf("hasStdin = false, want true")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func namesOf(cmds []types.CommandDef) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Name
	}
	return out
}

func init() {
	// Suppress JSON output during tests by sending it to os.Stdout
	// (tests that need to capture it will swap util.Out themselves).
	_ = os.DevNull
}
