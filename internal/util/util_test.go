package util_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xilistudios/mcp2cli/internal/types"
	. "github.com/xilistudios/mcp2cli/internal/util"
)

// helper to capture Out/Err and StdoutIsTTY for tests that emit output
func captureOutput(t *testing.T) (restore func()) {
	t.Helper()
	oldOut, oldErr, oldTTY := Out, Err, StdoutIsTTY
	Out = &bytes.Buffer{}
	Err = &bytes.Buffer{}
	StdoutIsTTY = func() bool { return false }
	return func() {
		Out = oldOut
		Err = oldErr
		StdoutIsTTY = oldTTY
	}
}

func outBuf() *bytes.Buffer { return Out.(*bytes.Buffer) }
func errBuf() *bytes.Buffer { return Err.(*bytes.Buffer) }

// ---------------------------------------------------------------------------
// ResolveSecret
// ---------------------------------------------------------------------------

func TestResolveSecret_Literal(t *testing.T) {
	got, err := ResolveSecret("hello")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestResolveSecret_Env(t *testing.T) {
	t.Setenv("MCP2CLI_TEST_SECRET", "s3cret")
	got, err := ResolveSecret("env:MCP2CLI_TEST_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if got != "s3cret" {
		t.Fatalf("got %q, want %q", got, "s3cret")
	}
}

func TestResolveSecret_EnvEmptyValue(t *testing.T) {
	t.Setenv("MCP2CLI_TEST_EMPTY", "")
	got, err := ResolveSecret("env:MCP2CLI_TEST_EMPTY")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestResolveSecret_EnvUnset(t *testing.T) {
	os.Unsetenv("MCP2CLI_DOES_NOT_EXIST_12345")
	_, err := ResolveSecret("env:MCP2CLI_DOES_NOT_EXIST_12345")
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
	if !strings.Contains(err.Error(), "is not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSecret_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	os.WriteFile(path, []byte("mysecret\n"), 0644)
	got, err := ResolveSecret("file:" + path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "mysecret" {
		t.Fatalf("got %q, want %q (trailing newline stripped)", got, "mysecret")
	}
}

func TestResolveSecret_FileMissing(t *testing.T) {
	_, err := ResolveSecret("file:/nonexistent/path/secret.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ParseKVList
// ---------------------------------------------------------------------------

func TestParseKVList_Valid(t *testing.T) {
	pairs, err := ParseKVList([]string{"A:b", "C:d"}, ":", "header", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2", len(pairs))
	}
	if pairs[0][0] != "A" || pairs[0][1] != "b" {
		t.Fatalf("pair 0: got %v", pairs[0])
	}
	if pairs[1][0] != "C" || pairs[1][1] != "d" {
		t.Fatalf("pair 1: got %v", pairs[1])
	}
}

func TestParseKVList_MissingDelimiter(t *testing.T) {
	_, err := ParseKVList([]string{"nodelimhere"}, ":", "header", false)
	if err == nil {
		t.Fatal("expected error for missing delimiter")
	}
	if !strings.Contains(err.Error(), "invalid header format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseKVList_ResolveValues(t *testing.T) {
	t.Setenv("MCP2CLI_TEST_TOKEN", "tok123")
	pairs, err := ParseKVList([]string{"Authorization:env:MCP2CLI_TEST_TOKEN"}, ":", "header", true)
	if err != nil {
		t.Fatal(err)
	}
	if pairs[0][1] != "tok123" {
		t.Fatalf("resolved value: got %q, want %q", pairs[0][1], "tok123")
	}
}

// ---------------------------------------------------------------------------
// ToKebab
// ---------------------------------------------------------------------------

func TestToKebab_CamelCase(t *testing.T) {
	got := ToKebab("camelCase")
	if got != "camel-case" {
		t.Fatalf("got %q, want %q", got, "camel-case")
	}
}

func TestToKebab_SnakeCase(t *testing.T) {
	got := ToKebab("snake_case")
	if got != "snake-case" {
		t.Fatalf("got %q, want %q", got, "snake-case")
	}
}

func TestToKebab_Mixed(t *testing.T) {
	got := ToKebab("foo_barBaz")
	if got != "foo-bar-baz" {
		t.Fatalf("got %q, want %q", got, "foo-bar-baz")
	}
}

func TestToKebab_AlreadyKebab(t *testing.T) {
	got := ToKebab("already-kebab")
	if got != "already-kebab" {
		t.Fatalf("got %q, want %q", got, "already-kebab")
	}
}

func TestToKebab_Acronyms(t *testing.T) {
	// Document behavior: the regex ([a-z0-9])([A-Z]) only fires at
	// lower→upper boundaries, so consecutive uppercase letters cluster.
	// "getHTTPServer" -> "get-HTTPServer" -> "get-httpserver"
	got := ToKebab("getHTTPServer")
	if got != "get-httpserver" {
		t.Fatalf("got %q, want %q", got, "get-httpserver")
	}
}

// ---------------------------------------------------------------------------
// SchemaTypeToPython
// ---------------------------------------------------------------------------

func TestSchemaTypeToPython(t *testing.T) {
	tests := []struct {
		schemaType string
		wantType   types.ParamType
		wantSuffix string
	}{
		{"integer", types.TypeInt, ""},
		{"number", types.TypeFloat, ""},
		{"boolean", types.TypeBoolean, ""},
		{"array", types.TypeString, " (JSON array)"},
		{"object", types.TypeString, " (JSON object)"},
		{"string", types.TypeString, ""},
	}
	for _, tt := range tests {
		t.Run(tt.schemaType, func(t *testing.T) {
			schema := map[string]any{"type": tt.schemaType}
			gotType, gotSuffix := SchemaTypeToPython(schema)
			if gotType != tt.wantType {
				t.Errorf("type: got %q, want %q", gotType, tt.wantType)
			}
			if gotSuffix != tt.wantSuffix {
				t.Errorf("suffix: got %q, want %q", gotSuffix, tt.wantSuffix)
			}
		})
	}

	// default (no type key)
	t.Run("default_no_type", func(t *testing.T) {
		gotType, gotSuffix := SchemaTypeToPython(map[string]any{})
		if gotType != types.TypeString {
			t.Errorf("type: got %q, want %q", gotType, types.TypeString)
		}
		if gotSuffix != "" {
			t.Errorf("suffix: got %q, want empty", gotSuffix)
		}
	})
}

// ---------------------------------------------------------------------------
// CoerceValue
// ---------------------------------------------------------------------------

func TestCoerceValue_Nil(t *testing.T) {
	got := CoerceValue(nil, map[string]any{"type": "string"})
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestCoerceValue_ArrayFromJSON(t *testing.T) {
	schema := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}
	got := CoerceValue("[1,2,3]", schema)
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want []any", got)
	}
	if len(arr) != 3 {
		t.Fatalf("got len %d, want 3", len(arr))
	}
	// Should be ints after coercion
	for i, v := range arr {
		if _, ok := v.(int); !ok {
			t.Errorf("arr[%d]: got %T (%v), want int", i, v, v)
		}
	}
}

func TestCoerceValue_ArrayCommaSplit(t *testing.T) {
	schema := map[string]any{"type": "array"}
	got := CoerceValue("a,b,c", schema)
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want []any", got)
	}
	if len(arr) != 3 {
		t.Fatalf("got len %d, want 3", len(arr))
	}
	for i, v := range arr {
		if s, ok := v.(string); !ok {
			t.Errorf("arr[%d]: got %T, want string", i, v)
		} else {
			expected := string(rune('a' + i))
			if s != expected {
				t.Errorf("arr[%d]: got %q, want %q", i, s, expected)
			}
		}
	}
}

func TestCoerceValue_ArrayCommaSplitIntegers(t *testing.T) {
	schema := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}
	got := CoerceValue("1,2", schema)
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want []any", got)
	}
	if len(arr) != 2 {
		t.Fatalf("got len %d, want 2", len(arr))
	}
	for i, v := range arr {
		if n, ok := v.(int); !ok {
			t.Errorf("arr[%d]: got %T, want int", i, v)
		} else if n != i+1 {
			t.Errorf("arr[%d]: got %d, want %d", i, n, i+1)
		}
	}
}

func TestCoerceValue_ObjectFromJSON(t *testing.T) {
	schema := map[string]any{"type": "object"}
	got := CoerceValue(`{"a":1}`, schema)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map[string]any", got)
	}
	if m["a"] != float64(1) {
		t.Fatalf("got %v, want 1", m["a"])
	}
}

func TestCoerceValue_BooleanTrue(t *testing.T) {
	schema := map[string]any{"type": "boolean"}
	got := CoerceValue(true, schema)
	if got != true {
		t.Fatalf("got %v, want true", got)
	}
}

func TestCoerceValue_BooleanFromString(t *testing.T) {
	schema := map[string]any{"type": "boolean"}
	if got := CoerceValue("true", schema); got != true {
		t.Fatalf("got %v, want true", got)
	}
	if got := CoerceValue("1", schema); got != true {
		t.Fatalf("got %v, want true", got)
	}
	if got := CoerceValue("no", schema); got != false {
		t.Fatalf("got %v, want false", got)
	}
}

func TestCoerceValue_IntegerFromString(t *testing.T) {
	schema := map[string]any{"type": "integer"}
	got := CoerceValue("42", schema)
	if got != 42 {
		t.Fatalf("got %v, want 42", got)
	}
}

func TestCoerceValue_IntegerFromFloat64(t *testing.T) {
	schema := map[string]any{"type": "integer"}
	got := CoerceValue(float64(7), schema)
	if got != 7 {
		t.Fatalf("got %v, want 7", got)
	}
}

func TestCoerceValue_NumberFromString(t *testing.T) {
	schema := map[string]any{"type": "number"}
	got := CoerceValue("3.14", schema)
	if got != 3.14 {
		t.Fatalf("got %v, want 3.14", got)
	}
}

func TestCoerceValue_SchemalessJSONObject(t *testing.T) {
	schema := map[string]any{} // no type key
	got := CoerceValue(`{"a":1}`, schema)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map[string]any", got)
	}
	if m["a"] != float64(1) {
		t.Fatalf("got %v, want 1", m["a"])
	}
}

func TestCoerceValue_SchemalessJSONArray(t *testing.T) {
	schema := map[string]any{} // no type key
	got := CoerceValue("[1,2]", schema)
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want []any", got)
	}
	if len(arr) != 2 {
		t.Fatalf("got len %d, want 2", len(arr))
	}
}

func TestCoerceValue_PlainStringUnchanged(t *testing.T) {
	schema := map[string]any{"type": "string"}
	got := CoerceValue("hello", schema)
	if got != "hello" {
		t.Fatalf("got %v, want %q", got, "hello")
	}
}

func TestCoerceValue_AlreadySlice(t *testing.T) {
	schema := map[string]any{"type": "array"}
	input := []any{1, 2, 3}
	got := CoerceValue(input, schema)
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want []any", got)
	}
	if len(arr) != 3 {
		t.Fatalf("got len %d, want 3", len(arr))
	}
}

func TestCoerceValue_ObjectInvalidJSON(t *testing.T) {
	schema := map[string]any{"type": "object"}
	got := CoerceValue("not json", schema)
	if got != "not json" {
		t.Fatalf("got %v, want %q", got, "not json")
	}
}

// ---------------------------------------------------------------------------
// ApplyHead
// ---------------------------------------------------------------------------

func TestApplyHead_Slice(t *testing.T) {
	data := []any{1, 2, 3}
	got := ApplyHead(data, 2)
	arr := got.([]any)
	if len(arr) != 2 {
		t.Fatalf("got len %d, want 2", len(arr))
	}
}

func TestApplyHead_SliceLonger(t *testing.T) {
	data := []any{1}
	got := ApplyHead(data, 5)
	arr := got.([]any)
	if len(arr) != 1 {
		t.Fatalf("got len %d, want 1", len(arr))
	}
}

func TestApplyHead_NonSlice(t *testing.T) {
	data := map[string]any{"a": 1}
	got := ApplyHead(data, 5)
	if got == nil {
		t.Fatal("got nil, want non-nil")
	}
	if m, ok := got.(map[string]any); !ok || m["a"] != 1 {
		t.Fatalf("got %v, want original map", got)
	}
}

// ---------------------------------------------------------------------------
// ReadStdinJSONFrom
// ---------------------------------------------------------------------------

func TestReadStdinJSONFrom_Valid(t *testing.T) {
	r := strings.NewReader(`{"key":"value"}`)
	got, err := ReadStdinJSONFrom(r, "test")
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map[string]any", got)
	}
	if m["key"] != "value" {
		t.Fatalf("got %v, want value", m["key"])
	}
}

func TestReadStdinJSONFrom_Empty(t *testing.T) {
	r := strings.NewReader("   \n  ")
	_, err := ReadStdinJSONFrom(r, "test")
	if err == nil {
		t.Fatal("expected error for empty stdin")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadStdinJSONFrom_Invalid(t *testing.T) {
	r := strings.NewReader(`{not json}`)
	_, err := ReadStdinJSONFrom(r, "mycontext")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON on stdin for mycontext") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// EmitJSON
// ---------------------------------------------------------------------------

func TestEmitJSON_Pretty(t *testing.T) {
	restore := captureOutput(t)
	defer restore()
	EmitJSON(map[string]any{"a": 1}, true)
	s := outBuf().String()
	if !strings.Contains(s, "\n") || !strings.Contains(s, "  ") {
		t.Fatalf("expected pretty output, got %q", s)
	}
}

func TestEmitJSON_Compact(t *testing.T) {
	restore := captureOutput(t)
	defer restore()
	EmitJSON(map[string]any{"a": 1}, false)
	s := outBuf().String()
	if strings.Contains(s, "  ") {
		t.Fatalf("expected compact output, got %q", s)
	}
}

// ---------------------------------------------------------------------------
// OutputResult
// ---------------------------------------------------------------------------

func TestOutputResult_JSONOutput_UnwrapString(t *testing.T) {
	restore := captureOutput(t)
	defer restore()
	OutputResult(`{"key":"val"}`, OutputOptions{JSONOutput: true})
	s := outBuf().String()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", s, err)
	}
	if parsed["key"] != "val" {
		t.Fatalf("got %v, want val", parsed["key"])
	}
}

func TestOutputResult_JSONOutput_WithHead(t *testing.T) {
	restore := captureOutput(t)
	defer restore()
	n := 2
	OutputResult(`[1,2,3]`, OutputOptions{JSONOutput: true, Head: &n})
	s := outBuf().String()
	var arr []any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		t.Fatalf("expected JSON array, got %q: %v", s, err)
	}
	if len(arr) != 2 {
		t.Fatalf("got len %d, want 2", len(arr))
	}
}

func TestOutputResult_Raw_String(t *testing.T) {
	restore := captureOutput(t)
	defer restore()
	OutputResult("hello world", OutputOptions{Raw: true})
	s := outBuf().String()
	if strings.TrimSpace(s) != "hello world" {
		t.Fatalf("got %q, want %q", strings.TrimSpace(s), "hello world")
	}
}

func TestOutputResult_Default_ParsedJSON(t *testing.T) {
	restore := captureOutput(t)
	defer restore()
	OutputResult(`{"a":1}`, OutputOptions{})
	s := outBuf().String()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", s, err)
	}
}

func TestOutputResult_Default_NonJSONString(t *testing.T) {
	restore := captureOutput(t)
	defer restore()
	OutputResult("not json at all", OutputOptions{})
	s := outBuf().String()
	if strings.TrimSpace(s) != "not json at all" {
		t.Fatalf("got %q, want %q", strings.TrimSpace(s), "not json at all")
	}
}

// ---------------------------------------------------------------------------
// ParamToDict / CommandToDict
// ---------------------------------------------------------------------------

func TestParamToDict_WithChoices(t *testing.T) {
	p := types.ParamDef{
		Name:         "format",
		OriginalName: "format",
		Type:         types.TypeString,
		Required:     false,
		Description:  "output format",
		Choices:      []string{"json", "text"},
		Location:     "query",
		Schema:       map[string]any{},
	}
	d := ParamToDict(p)
	if d["name"] != "format" {
		t.Fatalf("name: got %v", d["name"])
	}
	if d["type"] != "str" {
		t.Fatalf("type: got %v", d["type"])
	}
	if d["required"] != false {
		t.Fatalf("required: got %v", d["required"])
	}
	choices, ok := d["choices"].([]string)
	if !ok || len(choices) != 2 {
		t.Fatalf("choices: got %v", d["choices"])
	}
}

func TestParamToDict_WithoutChoices(t *testing.T) {
	p := types.ParamDef{
		Name:         "name",
		OriginalName: "name",
		Type:         types.TypeString,
		Required:     true,
		Description:  "the name",
		Choices:      nil,
		Location:     "path",
		Schema:       map[string]any{},
	}
	d := ParamToDict(p)
	if _, ok := d["choices"]; ok {
		t.Fatal("choices should be absent when nil")
	}
}

func TestCommandToDict_FullFields(t *testing.T) {
	cmd := types.CommandDef{
		Name:                 "get-user",
		Description:          "Get a user",
		Method:               "get",
		Path:                 "/users/{id}",
		ToolName:             "get_user",
		GraphQLOperationType: "query",
		Params: []types.ParamDef{
			{Name: "id", OriginalName: "id", Type: types.TypeInt, Required: true, Description: "User ID", Location: "path", Schema: map[string]any{}},
		},
	}
	d := CommandToDict(cmd)
	if d["method"] != "GET" {
		t.Fatalf("method: got %v, want GET", d["method"])
	}
	if d["path"] != "/users/{id}" {
		t.Fatalf("path: got %v", d["path"])
	}
	if d["toolName"] != "get_user" {
		t.Fatalf("toolName: got %v", d["toolName"])
	}
	if d["operationType"] != "query" {
		t.Fatalf("operationType: got %v", d["operationType"])
	}
	params, ok := d["parameters"].([]map[string]any)
	if !ok || len(params) != 1 {
		t.Fatalf("parameters: got %v", d["parameters"])
	}
}

func TestCommandToDict_ConditionalFields(t *testing.T) {
	cmd := types.CommandDef{
		Name:        "simple",
		Description: "Simple command",
		Params:      []types.ParamDef{},
	}
	d := CommandToDict(cmd)
	if _, ok := d["method"]; ok {
		t.Fatal("method should be absent when empty")
	}
	if _, ok := d["path"]; ok {
		t.Fatal("path should be absent when empty")
	}
	if _, ok := d["toolName"]; ok {
		t.Fatal("toolName should be absent when empty")
	}
	if _, ok := d["operationType"]; ok {
		t.Fatal("operationType should be absent when empty")
	}
}

// ---------------------------------------------------------------------------
// PrintCommandsJSON
// ---------------------------------------------------------------------------

func TestPrintCommandsJSON_Compact(t *testing.T) {
	restore := captureOutput(t)
	defer restore()
	cmds := []types.CommandDef{
		{Name: "alpha", Description: "A", Params: []types.ParamDef{}},
		{Name: "beta", Description: "B", Params: []types.ParamDef{}},
	}
	PrintCommandsJSON(cmds, true, false)
	s := outBuf().String()
	var arr []any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		t.Fatalf("expected JSON array, got %q: %v", s, err)
	}
	if len(arr) != 2 {
		t.Fatalf("got len %d, want 2", len(arr))
	}
	if arr[0] != "alpha" || arr[1] != "beta" {
		t.Fatalf("got %v, want [alpha beta]", arr)
	}
}

func TestPrintCommandsJSON_Full(t *testing.T) {
	restore := captureOutput(t)
	defer restore()
	cmds := []types.CommandDef{
		{Name: "alpha", Description: "A", Params: []types.ParamDef{}},
	}
	PrintCommandsJSON(cmds, false, false)
	s := outBuf().String()
	var arr []map[string]any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		t.Fatalf("expected JSON array of objects, got %q: %v", s, err)
	}
	if len(arr) != 1 {
		t.Fatalf("got len %d, want 1", len(arr))
	}
	if arr[0]["name"] != "alpha" {
		t.Fatalf("name: got %v", arr[0]["name"])
	}
	if arr[0]["description"] != "A" {
		t.Fatalf("description: got %v", arr[0]["description"])
	}
}

// ---------------------------------------------------------------------------
// FindToonCLI / ToonEncode (basic — may be empty if no toon installed)
// ---------------------------------------------------------------------------

func TestFindToonCLI_ReturnsString(t *testing.T) {
	// Just ensure it doesn't panic
	_ = FindToonCLI()
}

func TestToonEncode_Basic(t *testing.T) {
	// If toon is installed, it should work; otherwise returns false
	_, ok := ToonEncode(`{"a":1}`)
	// We don't assert true because toon may not be installed
	_ = ok
}
