package graphql

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

func captureOutput(fn func()) string {
	old := util.Out
	var buf bytes.Buffer
	util.Out = &buf
	fn()
	util.Out = old
	return buf.String()
}

func TestExecuteGraphQL_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		query, _ := req["query"].(string)
		if !contains(query, "user") {
			t.Errorf("expected user in query, got %s", query)
		}

		resp := map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id":   "1",
					"name": "Alice",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cmd := types.CommandDef{
		Name:                 "user",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "user",
		GraphQLReturnType: map[string]any{
			"kind": "OBJECT",
			"name": "User",
		},
		Params: []types.ParamDef{
			{
				Name:         "user-id",
				OriginalName: "id",
				Type:         types.TypeString,
				Required:     true,
				Schema:       map[string]any{"graphql_type": "ID!"},
			},
		},
	}
	schema := map[string]any{
		"types": []any{
			map[string]any{
				"kind": "OBJECT",
				"name": "User",
				"fields": []any{
					map[string]any{
						"name": "id",
						"type": map[string]any{"kind": "SCALAR", "name": "ID"},
					},
					map[string]any{
						"name": "name",
						"type": map[string]any{"kind": "SCALAR", "name": "String"},
					},
				},
			},
		},
	}
	paramValues := map[string]any{"user-id": "1"}
	opts := util.OutputOptions{JSONOutput: true}

	out := captureOutput(func() {
		err := ExecuteGraphQL(cmd, srv.URL, schema, nil, paramValues, "", nil, opts, nil)
		if err != nil {
			t.Fatalf("ExecuteGraphQL: %v", err)
		}
	})
	if !contains(out, "Alice") {
		t.Errorf("expected Alice in output, got %s", out)
	}
}

func TestExecuteGraphQL_AuthHeaders(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		resp := map[string]any{"data": map[string]any{"ping": "pong"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cmd := types.CommandDef{
		Name:                 "ping",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "ping",
	}
	schema := map[string]any{"types": []any{}}
	opts := util.OutputOptions{JSONOutput: true}

	captureOutput(func() {
		err := ExecuteGraphQL(cmd, srv.URL, schema, [][2]string{{"Authorization", "Bearer tok"}}, nil, "", nil, opts, nil)
		if err != nil {
			t.Fatalf("ExecuteGraphQL: %v", err)
		}
	})
	if gotAuth != "Bearer tok" {
		t.Errorf("expected auth header, got %q", gotAuth)
	}
}

func TestExecuteGraphQL_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"errors": []any{
				map[string]any{"message": "Field not found"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cmd := types.CommandDef{
		Name:                 "bad",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "bad",
	}
	schema := map[string]any{"types": []any{}}
	opts := util.OutputOptions{JSONOutput: true}

	err := ExecuteGraphQL(cmd, srv.URL, schema, nil, nil, "", nil, opts, nil)
	if err == nil {
		t.Fatal("expected error for GraphQL errors")
	}
	if !contains(err.Error(), "Field not found") {
		t.Errorf("expected error message, got: %v", err)
	}
}

func TestExecuteGraphQL_PartialErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"user": map[string]any{"id": "1"},
			},
			"errors": []any{
				map[string]any{"message": "partial error"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cmd := types.CommandDef{
		Name:                 "user",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "user",
	}
	schema := map[string]any{"types": []any{}}
	opts := util.OutputOptions{JSONOutput: true}

	// Should not error — partial errors are included in output
	out := captureOutput(func() {
		err := ExecuteGraphQL(cmd, srv.URL, schema, nil, nil, "", nil, opts, nil)
		if err != nil {
			t.Fatalf("should not error on partial errors: %v", err)
		}
	})
	if !contains(out, "partial error") {
		t.Errorf("expected partial error in output, got %s", out)
	}
}

func TestExecuteGraphQL_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cmd := types.CommandDef{
		Name:                 "test",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "test",
	}
	schema := map[string]any{"types": []any{}}
	opts := util.OutputOptions{JSONOutput: true}

	err := ExecuteGraphQL(cmd, srv.URL, schema, nil, nil, "", nil, opts, nil)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestExecuteGraphQL_StdinVars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		vars, _ := req["variables"].(map[string]any)
		if vars == nil || vars["id"] != "789" {
			t.Errorf("expected id=789 in variables, got %v", vars)
		}

		resp := map[string]any{"data": map[string]any{"user": map[string]any{"id": "789"}}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cmd := types.CommandDef{
		Name:                 "user",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "user",
		Params: []types.ParamDef{
			{
				Name:         "user-id",
				OriginalName: "id",
				Schema:       map[string]any{"graphql_type": "ID!"},
			},
		},
	}
	schema := map[string]any{"types": []any{}}
	stdinVars := map[string]any{"id": "789"}
	opts := util.OutputOptions{JSONOutput: true}

	captureOutput(func() {
		err := ExecuteGraphQL(cmd, srv.URL, schema, nil, nil, "", stdinVars, opts, nil)
		if err != nil {
			t.Fatalf("ExecuteGraphQL: %v", err)
		}
	})
}

func TestExecuteGraphQL_FieldsOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		query, _ := req["query"].(string)
		if !contains(query, "idOnly") {
			t.Errorf("expected fields override in query, got %s", query)
		}

		resp := map[string]any{"data": map[string]any{"user": map[string]any{"id": "1"}}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cmd := types.CommandDef{
		Name:                 "user",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "user",
	}
	schema := map[string]any{"types": []any{}}
	opts := util.OutputOptions{JSONOutput: true}

	captureOutput(func() {
		err := ExecuteGraphQL(cmd, srv.URL, schema, nil, nil, "idOnly", nil, opts, nil)
		if err != nil {
			t.Fatalf("ExecuteGraphQL: %v", err)
		}
	})
}

func TestExecuteGraphQL_DefaultClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"data": map[string]any{"ping": "ok"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cmd := types.CommandDef{
		Name:                 "ping",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "ping",
	}
	schema := map[string]any{"types": []any{}}
	opts := util.OutputOptions{JSONOutput: true}

	// Pass nil client to test default
	out := captureOutput(func() {
		err := ExecuteGraphQL(cmd, srv.URL, schema, nil, nil, "", nil, opts, nil)
		if err != nil {
			t.Fatalf("ExecuteGraphQL: %v", err)
		}
	})
	if !contains(out, "ok") {
		t.Errorf("expected ok in output, got %s", out)
	}
}

func TestExecuteGraphQL_FieldDataExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"users": []any{
					map[string]any{"id": "1", "name": "Alice"},
					map[string]any{"id": "2", "name": "Bob"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cmd := types.CommandDef{
		Name:                 "users",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "users",
	}
	schema := map[string]any{"types": []any{}}
	opts := util.OutputOptions{JSONOutput: true}

	out := captureOutput(func() {
		err := ExecuteGraphQL(cmd, srv.URL, schema, nil, nil, "", nil, opts, nil)
		if err != nil {
			t.Fatalf("ExecuteGraphQL: %v", err)
		}
	})
	if !contains(out, "Alice") {
		t.Errorf("expected Alice in output, got %s", out)
	}
	if !contains(out, "Bob") {
		t.Errorf("expected Bob in output, got %s", out)
	}
}
