package openapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// ---------------------------------------------------------------------------
// ResolveRefs
// ---------------------------------------------------------------------------

func TestResolveRefs_BasicInline(t *testing.T) {
	spec := map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{
				"Foo": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"bar": map[string]any{"type": "string"},
					},
				},
			},
		},
		"paths": map[string]any{
			"/test": map[string]any{
				"get": map[string]any{
					"responses": map[string]any{
						"200": map[string]any{
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"$ref": "#/components/schemas/Foo",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	resolved := ResolveRefs(spec)
	get := resolved["paths"].(map[string]any)["/test"].(map[string]any)["get"].(map[string]any)
	resp200 := get["responses"].(map[string]any)["200"].(map[string]any)
	contentJSON := resp200["content"].(map[string]any)["application/json"].(map[string]any)
	schema := contentJSON["schema"].(map[string]any)

	if schema["type"] != "object" {
		t.Fatalf("expected type=object, got %v", schema["type"])
	}
	if _, ok := schema["$ref"]; ok {
		t.Fatalf("$ref should have been resolved")
	}
}

func TestResolveRefs_Nested(t *testing.T) {
	spec := map[string]any{
		"definitions": map[string]any{
			"A": map[string]any{"$ref": "#/definitions/B"},
			"B": map[string]any{"type": "string"},
		},
		"paths": map[string]any{
			"/x": map[string]any{
				"get": map[string]any{
					"parameters": []any{
						map[string]any{
							"name":   "q",
							"in":     "query",
							"schema": map[string]any{"$ref": "#/definitions/A"},
						},
					},
				},
			},
		},
	}
	resolved := ResolveRefs(spec)
	params := resolved["paths"].(map[string]any)["/x"].(map[string]any)["get"].(map[string]any)["parameters"].([]any)
	schema := params[0].(map[string]any)["schema"].(map[string]any)
	if schema["type"] != "string" {
		t.Fatalf("expected nested ref resolved to type=string, got %v", schema["type"])
	}
}

func TestResolveRefs_Cyclic(t *testing.T) {
	spec := map[string]any{
		"definitions": map[string]any{
			"Node": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"child": map[string]any{"$ref": "#/definitions/Node"},
				},
			},
		},
		"paths": map[string]any{
			"/n": map[string]any{
				"get": map[string]any{
					"responses": map[string]any{
						"200": map[string]any{
							"schema": map[string]any{"$ref": "#/definitions/Node"},
						},
					},
				},
			},
		},
	}
	// Must not infinite-loop.
	resolved := ResolveRefs(spec)
	schema := resolved["paths"].(map[string]any)["/n"].(map[string]any)["get"].(map[string]any)["responses"].(map[string]any)["200"].(map[string]any)["schema"].(map[string]any)
	if schema["type"] != "object" {
		t.Fatalf("expected type=object, got %v", schema["type"])
	}
	// Cyclic child ref is left as-is ($ref still present).
	child := schema["properties"].(map[string]any)["child"].(map[string]any)
	if _, ok := child["$ref"]; !ok {
		t.Fatalf("cyclic ref should remain as $ref")
	}
}

// ---------------------------------------------------------------------------
// ExtractCommands
// ---------------------------------------------------------------------------

func TestExtractCommands_Basic(t *testing.T) {
	spec := map[string]any{
		"paths": map[string]any{
			"/users": map[string]any{
				"get": map[string]any{
					"operationId": "listUsers",
					"summary":     "List all users",
					"parameters": []any{
						map[string]any{
							"name":     "page",
							"in":       "query",
							"required": false,
							"schema":   map[string]any{"type": "integer"},
						},
						map[string]any{
							"name":     "user_id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "string"},
						},
					},
				},
			},
			"/users/create": map[string]any{
				"post": map[string]any{
					"operationId": "createUser",
					"summary":     "Create a user",
					"requestBody": map[string]any{
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":     "object",
									"required": []any{"name"},
									"properties": map[string]any{
										"name": map[string]any{
											"type":        "string",
											"description": "User name",
										},
										"role": map[string]any{
											"type": "string",
											"enum": []any{"admin", "user", "guest"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	cmds := ExtractCommands(spec)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}

	// Find by name.
	var listCmd, createCmd types.CommandDef
	for _, c := range cmds {
		switch c.Name {
		case "list-users":
			listCmd = c
		case "create-user":
			createCmd = c
		}
	}

	if listCmd.Name == "" {
		t.Fatal("list-users command not found")
	}
	if listCmd.Method != "get" {
		t.Fatalf("expected method=get, got %s", listCmd.Method)
	}
	if listCmd.Path != "/users" {
		t.Fatalf("expected path=/users, got %s", listCmd.Path)
	}
	if listCmd.Description != "List all users" {
		t.Fatalf("expected description 'List all users', got %s", listCmd.Description)
	}

	// Check params.
	var queryParam, pathParam types.ParamDef
	for _, p := range listCmd.Params {
		switch p.Location {
		case "query":
			queryParam = p
		case "path":
			pathParam = p
		}
	}
	if queryParam.Name != "page" {
		t.Fatalf("expected query param 'page', got %s", queryParam.Name)
	}
	if queryParam.Type != types.TypeInt {
		t.Fatalf("expected int type, got %s", queryParam.Type)
	}
	if pathParam.Name != "user-id" {
		t.Fatalf("expected path param 'user-id', got %s", pathParam.Name)
	}
	if pathParam.Type != types.TypeString {
		t.Fatalf("expected string type, got %s", pathParam.Type)
	}

	// POST command.
	if createCmd.Name == "" {
		t.Fatal("create-user command not found")
	}
	if createCmd.Method != "post" {
		t.Fatalf("expected method=post, got %s", createCmd.Method)
	}
	if !createCmd.HasBody {
		t.Fatal("expected HasBody=true")
	}

	var nameParam, roleParam types.ParamDef
	for _, p := range createCmd.Params {
		switch p.Name {
		case "name":
			nameParam = p
		case "role":
			roleParam = p
		}
	}
	if nameParam.Required != true {
		t.Fatal("expected name param required=true")
	}
	if roleParam.Choices == nil || len(roleParam.Choices) != 3 {
		t.Fatalf("expected 3 choices for role, got %v", roleParam.Choices)
	}
}

func TestExtractCommands_MultipartBinary(t *testing.T) {
	spec := map[string]any{
		"paths": map[string]any{
			"/upload": map[string]any{
				"post": map[string]any{
					"operationId": "uploadFile",
					"requestBody": map[string]any{
						"content": map[string]any{
							"multipart/form-data": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"file": map[string]any{
											"type":   "string",
											"format": "binary",
										},
										"description": map[string]any{
											"type": "string",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	cmds := ExtractCommands(spec)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	cmd := cmds[0]
	if cmd.ContentType != "multipart/form-data" {
		t.Fatalf("expected multipart/form-data, got %s", cmd.ContentType)
	}
	var fileP, descP types.ParamDef
	for _, p := range cmd.Params {
		switch p.OriginalName {
		case "file":
			fileP = p
		case "description":
			descP = p
		}
	}
	if fileP.Location != "file" {
		t.Fatalf("expected file location, got %s", fileP.Location)
	}
	if fileP.Type != types.TypeString {
		t.Fatalf("expected str type for file, got %s", fileP.Type)
	}
	if descP.Location != "body" {
		t.Fatalf("expected body location for description, got %s", descP.Location)
	}
}

func TestExtractCommands_NameCollisionDedupe(t *testing.T) {
	spec := map[string]any{
		"paths": map[string]any{
			"/items": map[string]any{
				"get": map[string]any{
					"operationId": "list-items",
				},
				"post": map[string]any{
					"operationId": "list-items", // same operationId → collision
				},
			},
		},
	}
	cmds := ExtractCommands(spec)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	names := map[string]bool{}
	for _, c := range cmds {
		names[c.Name] = true
	}
	// First: "list-items", second: "list-items-post"
	if !names["list-items"] {
		t.Fatal("expected 'list-items' in output")
	}
	if !names["list-items-post"] {
		t.Fatal("expected 'list-items-post' in output")
	}
}

// ---------------------------------------------------------------------------
// LoadSpec
// ---------------------------------------------------------------------------

func TestLoadSpec_JSONFile(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]any{
		"paths": map[string]any{
			"/test": map[string]any{
				"get": map[string]any{"summary": "test"},
			},
		},
	}
	raw, _ := json.Marshal(spec)
	path := filepath.Join(dir, "spec.json")
	os.WriteFile(path, raw, 0644)

	result, err := LoadSpec(path, nil, "", 0, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["paths"] == nil {
		t.Fatal("expected paths in result")
	}
}

func TestLoadSpec_YAMLFile(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `paths:
  /test:
    get:
      summary: test
`
	path := filepath.Join(dir, "spec.yaml")
	os.WriteFile(path, []byte(yamlContent), 0644)

	result, err := LoadSpec(path, nil, "", 0, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["paths"] == nil {
		t.Fatal("expected paths in result")
	}
}

func TestLoadSpec_MissingPaths(t *testing.T) {
	dir := t.TempDir()
	spec := map[string]any{"openapi": "3.0.0"}
	raw, _ := json.Marshal(spec)
	path := filepath.Join(dir, "spec.json")
	os.WriteFile(path, raw, 0644)

	_, err := LoadSpec(path, nil, "", 0, false, nil)
	if err == nil {
		t.Fatal("expected error for missing paths")
	}
	if !strings.Contains(err.Error(), "paths") {
		t.Fatalf("error should mention 'paths', got: %v", err)
	}
}

func TestLoadSpec_URLCaching(t *testing.T) {
	spec := map[string]any{
		"paths": map[string]any{
			"/cached": map[string]any{
				"get": map[string]any{"summary": "cached endpoint"},
			},
		},
	}
	raw, _ := json.Marshal(spec)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
	}))
	defer ts.Close()

	cacheDir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", cacheDir)

	// First load — from server.
	result, err := LoadSpec(ts.URL, nil, "", 3600, false, nil)
	if err != nil {
		t.Fatalf("first load error: %v", err)
	}
	if result["paths"] == nil {
		t.Fatal("expected paths")
	}

	// Stop server — should still load from cache.
	ts.Close()

	result2, err := LoadSpec(ts.URL, nil, "", 3600, false, nil)
	if err != nil {
		t.Fatalf("second load error: %v", err)
	}
	if result2["paths"] == nil {
		t.Fatal("expected paths from cache")
	}
}

func TestLoadSpec_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("not found"))
	}))
	defer ts.Close()

	_, err := LoadSpec(ts.URL, nil, "", 0, true, nil)
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// BuildRequest
// ---------------------------------------------------------------------------

func TestBuildRequest_PathSubstitution(t *testing.T) {
	cmd := types.CommandDef{
		Method: "get",
		Path:   "/users/{user_id}/posts/{post_id}",
		Params: []types.ParamDef{
			{Name: "user-id", OriginalName: "user_id", Location: "path"},
			{Name: "post-id", OriginalName: "post_id", Location: "path"},
			{Name: "page", OriginalName: "page", Location: "query", Schema: map[string]any{"type": "integer"}},
		},
	}
	args := map[string]any{
		"user-id": 42,
		"post-id": "abc",
		"page":    "3",
	}
	req, err := BuildRequest(cmd, "https://api.example.com", args, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(req.URL, "/users/42/posts/abc") {
		t.Fatalf("path not substituted: %s", req.URL)
	}
	if req.Query["page"] != 3 {
		t.Fatalf("expected page=3 (int), got %v", req.Query["page"])
	}
}

func TestBuildRequest_GETQueryParams(t *testing.T) {
	cmd := types.CommandDef{
		Method: "get",
		Path:   "/search",
		Params: []types.ParamDef{
			{Name: "q", OriginalName: "q", Location: "query", Schema: map[string]any{"type": "string"}},
			{Name: "x-auth", OriginalName: "X-Auth", Location: "header"},
		},
	}
	args := map[string]any{
		"q":      "hello",
		"x-auth": "tok123",
	}
	req, err := BuildRequest(cmd, "https://api.example.com", args, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Query["q"] != "hello" {
		t.Fatalf("expected q=hello, got %v", req.Query["q"])
	}
	if req.Headers["X-Auth"] != "tok123" {
		t.Fatalf("expected header X-Auth=tok123, got %v", req.Headers["X-Auth"])
	}
}

func TestBuildRequest_POSTBody(t *testing.T) {
	cmd := types.CommandDef{
		Method: "post",
		Path:   "/users",
		Params: []types.ParamDef{
			{Name: "name", OriginalName: "name", Location: "body", Schema: map[string]any{"type": "string"}},
			{Name: "age", OriginalName: "age", Location: "body", Schema: map[string]any{"type": "integer"}},
		},
	}
	args := map[string]any{
		"name": "Alice",
		"age":  "30",
	}
	req, err := BuildRequest(cmd, "https://api.example.com", args, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Body["name"] != "Alice" {
		t.Fatalf("expected name=Alice, got %v", req.Body["name"])
	}
	if req.Body["age"] != 30 {
		t.Fatalf("expected age=30 (int), got %v", req.Body["age"])
	}
}

func TestBuildRequest_FileParam(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "test.txt")
	os.WriteFile(fpath, []byte("hello"), 0644)

	cmd := types.CommandDef{
		Method:      "post",
		Path:        "/upload",
		ContentType: "multipart/form-data",
		Params: []types.ParamDef{
			{Name: "file", OriginalName: "file", Location: "file"},
			{Name: "note", OriginalName: "note", Location: "body", Schema: map[string]any{"type": "string"}},
		},
	}
	args := map[string]any{
		"file": fpath,
		"note": "a note",
	}
	req, err := BuildRequest(cmd, "https://api.example.com", args, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Files == nil || req.Files["file"].Filename != "test.txt" {
		t.Fatalf("expected file part, got %v", req.Files)
	}
	if req.Body["note"] != "a note" {
		t.Fatalf("expected note=a note, got %v", req.Body["note"])
	}
}

func TestBuildRequest_FileNotFound(t *testing.T) {
	cmd := types.CommandDef{
		Method: "post",
		Path:   "/upload",
		Params: []types.ParamDef{
			{Name: "file", OriginalName: "file", Location: "file"},
		},
	}
	args := map[string]any{"file": "/nonexistent/file.bin"}
	_, err := BuildRequest(cmd, "https://api.example.com", args, false, nil)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("expected 'file not found' in error, got: %v", err)
	}
}

func TestBuildRequest_StdinBody(t *testing.T) {
	cmd := types.CommandDef{
		Method: "post",
		Path:   "/data",
		Params: []types.ParamDef{},
	}
	stdinJSON := map[string]any{"key": "value"}
	req, err := BuildRequest(cmd, "https://api.example.com", map[string]any{}, true, stdinJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Body["key"] != "value" {
		t.Fatalf("expected stdin body, got %v", req.Body)
	}
}

// ---------------------------------------------------------------------------
// Execute
// ---------------------------------------------------------------------------

func TestExecute_JSONOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	oldOut := util.Out
	oldErr := util.Err
	oldTTY := util.StdoutIsTTY
	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	util.Out = buf
	util.Err = errBuf
	util.StdoutIsTTY = func() bool { return false }
	defer func() {
		util.Out = oldOut
		util.Err = oldErr
		util.StdoutIsTTY = oldTTY
	}()

	req := Request{
		Method: "get",
		URL:    ts.URL,
		Query:  map[string]any{},
	}
	err := Execute(req, nil, util.OutputOptions{JSONOutput: true}, ts.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `"ok":true`) {
		t.Fatalf("expected JSON output, got: %s", buf.String())
	}
}

func TestExecute_RawOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`raw bytes here`))
	}))
	defer ts.Close()

	oldOut := util.Out
	oldErr := util.Err
	buf := &bytes.Buffer{}
	util.Out = buf
	util.Err = &bytes.Buffer{}
	defer func() {
		util.Out = oldOut
		util.Err = oldErr
	}()

	req := Request{Method: "get", URL: ts.URL}
	err := Execute(req, nil, util.OutputOptions{Raw: true}, ts.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "raw bytes here" {
		t.Fatalf("expected raw output, got: %q", buf.String())
	}
}

func TestExecute_400Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer ts.Close()

	oldOut := util.Out
	oldErr := util.Err
	util.Out = &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	util.Err = errBuf
	defer func() {
		util.Out = oldOut
		util.Err = oldErr
	}()

	req := Request{Method: "post", URL: ts.URL}
	err := Execute(req, nil, util.OutputOptions{}, ts.Client())
	if err == nil {
		t.Fatal("expected error for HTTP 400")
	}
	if !strings.Contains(err.Error(), "400:") {
		t.Fatalf("expected '400:' in error, got: %q", err.Error())
	}
}

func TestExecute_JSONBodyRoundTrip(t *testing.T) {
	var received map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body) // echo
	}))
	defer ts.Close()

	oldOut := util.Out
	oldErr := util.Err
	buf := &bytes.Buffer{}
	util.Out = buf
	util.Err = &bytes.Buffer{}
	oldTTY := util.StdoutIsTTY
	util.StdoutIsTTY = func() bool { return false }
	defer func() {
		util.Out = oldOut
		util.Err = oldErr
		util.StdoutIsTTY = oldTTY
	}()

	req := Request{
		Method: "post",
		URL:    ts.URL,
		Body:   map[string]any{"name": "Bob", "count": 42},
	}
	err := Execute(req, nil, util.OutputOptions{JSONOutput: true}, ts.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received["name"] != "Bob" {
		t.Fatalf("server didn't receive name=Bob, got: %v", received)
	}
	// Output should be JSON.
	if !strings.Contains(buf.String(), "Bob") {
		t.Fatalf("expected Bob in output, got: %q", buf.String())
	}
}

func TestExecute_QueryParamsEncoded(t *testing.T) {
	var gotURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	oldOut := util.Out
	oldErr := util.Err
	util.Out = &bytes.Buffer{}
	util.Err = &bytes.Buffer{}
	defer func() {
		util.Out = oldOut
		util.Err = oldErr
	}()

	req := Request{
		Method: "get",
		URL:    ts.URL + "/api",
		Query:  map[string]any{"q": "hello", "tags": []any{"a", "b"}},
	}
	err := Execute(req, nil, util.OutputOptions{Raw: true}, ts.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotURL, "q=hello") {
		t.Fatalf("expected q=hello in URL, got: %s", gotURL)
	}
	if !strings.Contains(gotURL, "tags=a") || !strings.Contains(gotURL, "tags=b") {
		t.Fatalf("expected repeated tags params, got: %s", gotURL)
	}
}

func TestExecute_MultipartWithFile(t *testing.T) {
	var contentType string
	var bodyStr string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		bodyStr = string(body)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	oldOut := util.Out
	oldErr := util.Err
	util.Out = &bytes.Buffer{}
	util.Err = &bytes.Buffer{}
	defer func() {
		util.Out = oldOut
		util.Err = oldErr
	}()

	req := Request{
		Method:      "post",
		URL:         ts.URL + "/upload",
		Body:        map[string]any{"note": "test"},
		ContentType: "multipart/form-data",
		Files: map[string]FilePart{
			"file": {
				Filename: "hello.txt",
				Open: func() (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("file content")), nil
				},
				MIME: "text/plain",
			},
		},
	}
	err := Execute(req, nil, util.OutputOptions{Raw: true}, ts.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Fatalf("expected multipart content type, got: %s", contentType)
	}
	if !strings.Contains(bodyStr, "file content") {
		t.Fatalf("expected 'file content' in body, got: %q", bodyStr)
	}
	if !strings.Contains(bodyStr, "test") {
		t.Fatalf("expected 'test' in body for note field, got: %q", bodyStr)
	}
}

func TestExecute_DefaultJSONContentType(t *testing.T) {
	var gotCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	oldOut, oldErr := util.Out, util.Err
	util.Out, util.Err = &bytes.Buffer{}, &bytes.Buffer{}
	defer func() { util.Out, util.Err = oldOut, oldErr }()

	req := Request{Method: "post", URL: ts.URL, Body: map[string]any{"x": 1}}
	Execute(req, nil, util.OutputOptions{Raw: true}, ts.Client())
	if gotCT != "application/json" {
		t.Fatalf("expected application/json, got: %s", gotCT)
	}
}
