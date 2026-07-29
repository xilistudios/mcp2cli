package graphql

import (
	"testing"

	"github.com/xilistudios/mcp2cli/internal/types"
)

func TestBuildDocument_BasicQuery(t *testing.T) {
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
				Schema:       map[string]any{"graphql_type": "ID!", "type": "string"},
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
	paramValues := map[string]any{"user-id": "123"}

	doc, vars, fieldName := BuildDocument(cmd, schema, nil, paramValues, "")

	if fieldName != "user" {
		t.Errorf("expected fieldName=user, got %s", fieldName)
	}
	if !contains(doc, "query") {
		t.Errorf("expected query in document, got %s", doc)
	}
	if !contains(doc, "$id: ID!") {
		t.Errorf("expected variable declaration, got %s", doc)
	}
	if !contains(doc, "user(id: $id)") {
		t.Errorf("expected field with args, got %s", doc)
	}
	if vars["id"] != "123" {
		t.Errorf("expected id=123 in variables, got %v", vars["id"])
	}
}

func TestBuildDocument_Mutation(t *testing.T) {
	cmd := types.CommandDef{
		Name:                 "create-user",
		GraphQLOperationType: "mutation",
		GraphQLFieldName:     "createUser",
		GraphQLReturnType: map[string]any{
			"kind": "OBJECT",
			"name": "User",
		},
		Params: []types.ParamDef{
			{
				Name:         "name",
				OriginalName: "name",
				Type:         types.TypeString,
				Required:     true,
				Schema:       map[string]any{"graphql_type": "String!"},
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
				},
			},
		},
	}
	paramValues := map[string]any{"name": "Alice"}

	doc, _, _ := BuildDocument(cmd, schema, nil, paramValues, "")
	if !contains(doc, "mutation") {
		t.Errorf("expected mutation in document, got %s", doc)
	}
	if !contains(doc, "createUser(name: $name)") {
		t.Errorf("expected createUser with args, got %s", doc)
	}
}

func TestBuildDocument_FieldsOverride(t *testing.T) {
	cmd := types.CommandDef{
		Name:                 "users",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "users",
	}

	doc, _, _ := BuildDocument(cmd, map[string]any{"types": []any{}}, nil, nil, "id name")
	if !contains(doc, "{ id name }") {
		t.Errorf("expected fields override in document, got %s", doc)
	}
}

func TestBuildDocument_StdinVars(t *testing.T) {
	cmd := types.CommandDef{
		Name:                 "user",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "user",
		Params: []types.ParamDef{
			{
				Name:         "user-id",
				OriginalName: "id",
				Type:         types.TypeString,
				Schema:       map[string]any{"graphql_type": "ID!"},
			},
		},
	}
	stdinVars := map[string]any{"id": "456"}

	doc, vars, _ := BuildDocument(cmd, map[string]any{"types": []any{}}, stdinVars, nil, "")
	if !contains(doc, "$id: ID!") {
		t.Errorf("expected variable declaration, got %s", doc)
	}
	if vars["id"] != "456" {
		t.Errorf("expected id=456, got %v", vars["id"])
	}
}

func TestBuildDocument_NoParams(t *testing.T) {
	cmd := types.CommandDef{
		Name:                 "ping",
		GraphQLOperationType: "query",
		GraphQLFieldName:     "ping",
		GraphQLReturnType: map[string]any{
			"kind": "SCALAR",
			"name": "String",
		},
	}

	doc, vars, _ := BuildDocument(cmd, map[string]any{"types": []any{}}, nil, nil, "")
	if !contains(doc, "query { ping  }") {
		t.Errorf("expected simple query, got %s", doc)
	}
	if len(vars) != 0 {
		t.Errorf("expected empty vars, got %v", vars)
	}
}

func TestBuildDocument_DefaultOperationType(t *testing.T) {
	// When GraphQLOperationType is empty, defaults to "query"
	cmd := types.CommandDef{
		Name:             "ping",
		GraphQLFieldName: "ping",
	}

	doc, _, _ := BuildDocument(cmd, map[string]any{"types": []any{}}, nil, nil, "")
	if !contains(doc, "query") {
		t.Errorf("expected default query operation, got %s", doc)
	}
}

func TestBuildDocument_DefaultFieldName(t *testing.T) {
	// When GraphQLFieldName is empty, falls back to Name
	cmd := types.CommandDef{
		Name:                 "my-field",
		GraphQLOperationType: "query",
	}

	doc, _, fieldName := BuildDocument(cmd, map[string]any{"types": []any{}}, nil, nil, "")
	if fieldName != "my-field" {
		t.Errorf("expected fieldName=my-field, got %s", fieldName)
	}
	if !contains(doc, "my-field") {
		t.Errorf("expected my-field in document, got %s", doc)
	}
}
