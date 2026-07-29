package graphql

import (
	"testing"

	"github.com/xilistudios/mcp2cli/internal/types"
)

func TestBuildSelectionSet_Scalar(t *testing.T) {
	typeRef := map[string]any{"kind": "SCALAR", "name": "String"}
	result := BuildSelectionSet(typeRef, nil, 2, nil)
	if result != "" {
		t.Errorf("expected empty for scalar, got %q", result)
	}
}

func TestBuildSelectionSet_Enum(t *testing.T) {
	typeRef := map[string]any{"kind": "ENUM", "name": "Status"}
	result := BuildSelectionSet(typeRef, nil, 2, nil)
	if result != "" {
		t.Errorf("expected empty for enum, got %q", result)
	}
}

func TestBuildSelectionSet_ObjectWithScalars(t *testing.T) {
	typeRef := map[string]any{
		"kind": "OBJECT",
		"name": "User",
	}
	typesByName := map[string]map[string]any{
		"User": {
			"fields": []any{
				map[string]any{
					"name": "id",
					"type": map[string]any{"kind": "SCALAR", "name": "ID"},
				},
				map[string]any{
					"name": "name",
					"type": map[string]any{"kind": "SCALAR", "name": "String"},
				},
				map[string]any{
					"name": "email",
					"type": map[string]any{"kind": "SCALAR", "name": "String"},
				},
			},
		},
	}
	result := BuildSelectionSet(typeRef, typesByName, 2, nil)
	if result != "{ id name email }" {
		t.Errorf("unexpected selection set: %q", result)
	}
}

func TestBuildSelectionSet_NestedObject(t *testing.T) {
	typeRef := map[string]any{
		"kind": "OBJECT",
		"name": "Query",
	}
	typesByName := map[string]map[string]any{
		"Query": {
			"fields": []any{
				map[string]any{
					"name": "user",
					"type": map[string]any{"kind": "OBJECT", "name": "User"},
				},
				map[string]any{
					"name": "count",
					"type": map[string]any{"kind": "SCALAR", "name": "Int"},
				},
			},
		},
		"User": {
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
	}
	result := BuildSelectionSet(typeRef, typesByName, 2, nil)
	// Should expand User at depth 2
	if result == "" {
		t.Fatal("expected non-empty selection set")
	}
	if !contains(result, "user {") {
		t.Errorf("expected nested user selection, got %q", result)
	}
	if !contains(result, "count") {
		t.Errorf("expected count field, got %q", result)
	}
}

func TestBuildSelectionSet_DepthZero(t *testing.T) {
	typeRef := map[string]any{
		"kind": "OBJECT",
		"name": "User",
	}
	typesByName := map[string]map[string]any{
		"User": {
			"fields": []any{
				map[string]any{
					"name": "id",
					"type": map[string]any{"kind": "SCALAR", "name": "ID"},
				},
			},
		},
	}
	result := BuildSelectionSet(typeRef, typesByName, 0, nil)
	if result != "" {
		t.Errorf("expected empty at depth 0, got %q", result)
	}
}

func TestBuildSelectionSet_CycleDetection(t *testing.T) {
	typeRef := map[string]any{
		"kind": "OBJECT",
		"name": "Node",
	}
	typesByName := map[string]map[string]any{
		"Node": {
			"fields": []any{
				map[string]any{
					"name": "id",
					"type": map[string]any{"kind": "SCALAR", "name": "ID"},
				},
				map[string]any{
					"name": "parent",
					"type": map[string]any{"kind": "OBJECT", "name": "Node"},
				},
			},
		},
	}
	result := BuildSelectionSet(typeRef, typesByName, 2, nil)
	// Should not infinitely recurse; parent won't expand because Node is in seen
	if !contains(result, "id") {
		t.Errorf("expected id field, got %q", result)
	}
	if contains(result, "parent {") {
		t.Errorf("should not expand recursive parent, got %q", result)
	}
}

func TestBuildSelectionSet_EnumField(t *testing.T) {
	typeRef := map[string]any{
		"kind": "OBJECT",
		"name": "User",
	}
	typesByName := map[string]map[string]any{
		"User": {
			"fields": []any{
				map[string]any{
					"name": "status",
					"type": map[string]any{"kind": "ENUM", "name": "Status"},
				},
				map[string]any{
					"name": "name",
					"type": map[string]any{"kind": "SCALAR", "name": "String"},
				},
			},
		},
	}
	result := BuildSelectionSet(typeRef, typesByName, 2, nil)
	if !contains(result, "status") {
		t.Errorf("expected status field, got %q", result)
	}
	if !contains(result, "name") {
		t.Errorf("expected name field, got %q", result)
	}
}

func TestBuildSelectionSet_NilTypeRef(t *testing.T) {
	result := BuildSelectionSet(nil, nil, 2, nil)
	if result != "" {
		t.Errorf("expected empty for nil, got %q", result)
	}
}

func TestBuildSelectionSet_MissingType(t *testing.T) {
	typeRef := map[string]any{
		"kind": "OBJECT",
		"name": "Unknown",
	}
	typesByName := map[string]map[string]any{}
	result := BuildSelectionSet(typeRef, typesByName, 2, nil)
	if result != "" {
		t.Errorf("expected empty for missing type, got %q", result)
	}
}

func TestBuildSelectionSet_ObjectOnlyObjects(t *testing.T) {
	// A type where all fields are objects at depth=1 (not expanded)
	typeRef := map[string]any{
		"kind": "OBJECT",
		"name": "Container",
	}
	typesByName := map[string]map[string]any{
		"Container": {
			"fields": []any{
				map[string]any{
					"name": "items",
					"type": map[string]any{"kind": "OBJECT", "name": "Item"},
				},
			},
		},
		"Item": {
			"fields": []any{
				map[string]any{
					"name": "id",
					"type": map[string]any{"kind": "SCALAR", "name": "ID"},
				},
			},
		},
	}
	// depth=1 means we don't expand nested objects
	result := BuildSelectionSet(typeRef, typesByName, 1, nil)
	if result != "" {
		t.Errorf("expected empty at depth=1 with only object fields, got %q", result)
	}
}

func TestDetectFieldCollisions(t *testing.T) {
	queryFields := []any{
		map[string]any{"name": "user"},
		map[string]any{"name": "users"},
	}
	mutationFields := []any{
		map[string]any{"name": "user"},
		map[string]any{"name": "createUser"},
	}
	collisions := detectFieldCollisions(queryFields, mutationFields)
	if !collisions["user"] {
		t.Error("expected 'user' collision")
	}
	if collisions["users"] {
		t.Error("should not have 'users' collision")
	}
	if collisions["createUser"] {
		t.Error("should not have 'createUser' collision")
	}
}

func TestDetectFieldCollisions_None(t *testing.T) {
	queryFields := []any{
		map[string]any{"name": "users"},
	}
	mutationFields := []any{
		map[string]any{"name": "createUser"},
	}
	collisions := detectFieldCollisions(queryFields, mutationFields)
	if len(collisions) != 0 {
		t.Errorf("expected no collisions, got %v", collisions)
	}
}

func makeTestSchema() map[string]any {
	return map[string]any{
		"queryType":    map[string]any{"name": "Query"},
		"mutationType": map[string]any{"name": "Mutation"},
		"types": []any{
			map[string]any{
				"kind": "OBJECT",
				"name": "Query",
				"fields": []any{
					map[string]any{
						"name":        "user",
						"description": "Get a user by ID",
						"args": []any{
							map[string]any{
								"name":        "id",
								"description": "User ID",
								"type": map[string]any{
									"kind": "NON_NULL",
									"ofType": map[string]any{
										"kind": "SCALAR",
										"name": "ID",
									},
								},
							},
						},
						"type": map[string]any{
							"kind": "OBJECT",
							"name": "User",
						},
					},
					map[string]any{
						"name":        "users",
						"description": "List all users",
						"args": []any{
							map[string]any{
								"name": "limit",
								"type": map[string]any{
									"kind": "SCALAR",
									"name": "Int",
								},
							},
						},
						"type": map[string]any{
							"kind": "LIST",
							"ofType": map[string]any{
								"kind": "OBJECT",
								"name": "User",
							},
						},
					},
				},
			},
			map[string]any{
				"kind": "OBJECT",
				"name": "Mutation",
				"fields": []any{
					map[string]any{
						"name":        "user",
						"description": "Update a user",
						"args": []any{
							map[string]any{
								"name": "input",
								"type": map[string]any{
									"kind": "INPUT_OBJECT",
									"name": "UpdateUserInput",
								},
							},
						},
						"type": map[string]any{
							"kind": "OBJECT",
							"name": "User",
						},
					},
					map[string]any{
						"name":        "createUser",
						"description": "Create a user",
						"args": []any{
							map[string]any{
								"name": "name",
								"type": map[string]any{
									"kind": "NON_NULL",
									"ofType": map[string]any{
										"kind": "SCALAR",
										"name": "String",
									},
								},
							},
							map[string]any{
								"name": "role",
								"type": map[string]any{
									"kind": "ENUM",
									"name": "Role",
								},
							},
						},
						"type": map[string]any{
							"kind": "OBJECT",
							"name": "User",
						},
					},
				},
			},
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
			map[string]any{
				"kind": "INPUT_OBJECT",
				"name": "UpdateUserInput",
				"inputFields": []any{
					map[string]any{
						"name": "name",
						"type": map[string]any{"kind": "SCALAR", "name": "String"},
					},
				},
			},
			map[string]any{
				"kind": "ENUM",
				"name": "Role",
				"enumValues": []any{
					map[string]any{"name": "ADMIN"},
					map[string]any{"name": "USER"},
				},
			},
		},
	}
}

func TestExtractCommands_Basic(t *testing.T) {
	schema := makeTestSchema()
	commands := ExtractCommands(schema)

	if len(commands) < 3 {
		t.Fatalf("expected at least 3 commands, got %d", len(commands))
	}

	// Check that "user" collision was handled
	var hasQueryUser, hasMutationUser, hasCreateUser bool
	for _, cmd := range commands {
		switch {
		case cmd.Name == "query-user":
			hasQueryUser = true
			if cmd.GraphQLOperationType != "query" {
				t.Error("expected query-user to be a query")
			}
			if cmd.GraphQLFieldName != "user" {
				t.Errorf("expected GraphQLFieldName=user, got %s", cmd.GraphQLFieldName)
			}
		case cmd.Name == "mutation-user":
			hasMutationUser = true
			if cmd.GraphQLOperationType != "mutation" {
				t.Error("expected mutation-user to be a mutation")
			}
		case cmd.Name == "create-user":
			hasCreateUser = true
			if cmd.GraphQLOperationType != "mutation" {
				t.Error("expected create-user to be a mutation")
			}
		}
	}
	if !hasQueryUser {
		t.Error("missing query-user command")
	}
	if !hasMutationUser {
		t.Error("missing mutation-user command")
	}
	if !hasCreateUser {
		t.Error("missing create-user command")
	}
}

func TestExtractCommands_Params(t *testing.T) {
	schema := makeTestSchema()
	commands := ExtractCommands(schema)

	for _, cmd := range commands {
		if cmd.Name == "create-user" {
			if len(cmd.Params) != 2 {
				t.Fatalf("expected 2 params for create-user, got %d", len(cmd.Params))
			}
			// name param (required string)
			nameParam := cmd.Params[0]
			if nameParam.OriginalName != "name" {
				t.Errorf("expected name param, got %s", nameParam.OriginalName)
			}
			if !nameParam.Required {
				t.Error("expected name to be required")
			}
			if nameParam.Type != types.TypeString {
				t.Errorf("expected TypeString, got %v", nameParam.Type)
			}
			// role param (enum)
			roleParam := cmd.Params[1]
			if roleParam.OriginalName != "role" {
				t.Errorf("expected role param, got %s", roleParam.OriginalName)
			}
			if roleParam.Choices == nil || len(roleParam.Choices) != 2 {
				t.Errorf("expected 2 choices, got %v", roleParam.Choices)
			}
			return
		}
	}
	t.Error("create-user command not found")
}

func TestExtractCommands_SkipsIntrospectionFields(t *testing.T) {
	schema := map[string]any{
		"queryType": map[string]any{"name": "Query"},
		"types": []any{
			map[string]any{
				"kind": "OBJECT",
				"name": "Query",
				"fields": []any{
					map[string]any{
						"name": "realField",
						"type": map[string]any{"kind": "SCALAR", "name": "String"},
					},
					map[string]any{
						"name": "__typename",
						"type": map[string]any{"kind": "SCALAR", "name": "String"},
					},
					map[string]any{
						"name": "__schema",
						"type": map[string]any{"kind": "OBJECT", "name": "__Schema"},
					},
				},
			},
		},
	}
	commands := ExtractCommands(schema)
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d: %v", len(commands), commands)
	}
	if commands[0].Name != "real-field" {
		t.Errorf("expected real-field, got %s", commands[0].Name)
	}
}

func TestExtractCommands_NoMutation(t *testing.T) {
	schema := map[string]any{
		"queryType": map[string]any{"name": "Query"},
		"types": []any{
			map[string]any{
				"kind": "OBJECT",
				"name": "Query",
				"fields": []any{
					map[string]any{
						"name": "ping",
						"type": map[string]any{"kind": "SCALAR", "name": "String"},
					},
				},
			},
		},
	}
	commands := ExtractCommands(schema)
	if len(commands) != 1 {
		t.Fatalf("expected 1, got %d", len(commands))
	}
	if commands[0].GraphQLOperationType != "query" {
		t.Error("expected query operation")
	}
}

func TestExtractCommands_EmptySchema(t *testing.T) {
	schema := map[string]any{
		"types": []any{},
	}
	commands := ExtractCommands(schema)
	if len(commands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(commands))
	}
}

func TestExtractCommands_UsersListParam(t *testing.T) {
	schema := makeTestSchema()
	commands := ExtractCommands(schema)

	for _, cmd := range commands {
		if cmd.Name == "users" && cmd.GraphQLOperationType == "query" {
			if len(cmd.Params) != 1 {
				t.Fatalf("expected 1 param for users, got %d", len(cmd.Params))
			}
			limit := cmd.Params[0]
			if limit.OriginalName != "limit" {
				t.Errorf("expected limit, got %s", limit.OriginalName)
			}
			if limit.Required {
				t.Error("limit should not be required")
			}
			return
		}
	}
	t.Error("users command not found")
}

func TestBuildGraphQLParam_ListArg(t *testing.T) {
	arg := map[string]any{
		"name":        "ids",
		"description": "User IDs",
		"type": map[string]any{
			"kind": "LIST",
			"ofType": map[string]any{
				"kind": "SCALAR",
				"name": "ID",
			},
		},
	}
	p := buildGraphQLParam(arg, nil)
	if p.Name != "ids" {
		t.Errorf("expected ids, got %s", p.Name)
	}
	if p.Location != "graphql_arg" {
		t.Errorf("expected graphql_arg location, got %s", p.Location)
	}
	if p.Schema["type"] != "array" {
		t.Errorf("expected array type in schema, got %v", p.Schema["type"])
	}
	if !contains(p.Description, "JSON array") {
		t.Errorf("expected JSON array hint, got %s", p.Description)
	}
}

func TestBuildGraphQLParam_InputObjectArg(t *testing.T) {
	arg := map[string]any{
		"name": "input",
		"type": map[string]any{
			"kind": "INPUT_OBJECT",
			"name": "CreateInput",
		},
	}
	p := buildGraphQLParam(arg, nil)
	if p.Schema["type"] != "object" {
		t.Errorf("expected object type, got %v", p.Schema["type"])
	}
	if !contains(p.Description, "JSON object") {
		t.Errorf("expected JSON object hint, got %s", p.Description)
	}
}

func TestSortedCommandNames(t *testing.T) {
	cmds := []types.CommandDef{
		{Name: "zebra"},
		{Name: "alpha"},
		{Name: "middle"},
	}
	names := SortedCommandNames(cmds)
	expected := []string{"alpha", "middle", "zebra"}
	for i, n := range names {
		if n != expected[i] {
			t.Errorf("expected %s at %d, got %s", expected[i], i, n)
		}
	}
}
