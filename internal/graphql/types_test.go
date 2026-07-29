package graphql

import (
	"testing"

	"github.com/xilistudios/mcp2cli/internal/types"
)

func TestUnwrapType_Named(t *testing.T) {
	typeRef := map[string]any{
		"kind": "SCALAR",
		"name": "String",
	}
	named, isNonNull, isList := UnwrapType(typeRef)
	if named["name"] != "String" {
		t.Errorf("expected String, got %v", named["name"])
	}
	if isNonNull {
		t.Error("expected isNonNull=false")
	}
	if isList {
		t.Error("expected isList=false")
	}
}

func TestUnwrapType_NonNull(t *testing.T) {
	typeRef := map[string]any{
		"kind": "NON_NULL",
		"ofType": map[string]any{
			"kind": "SCALAR",
			"name": "String",
		},
	}
	named, isNonNull, isList := UnwrapType(typeRef)
	if named["name"] != "String" {
		t.Errorf("expected String, got %v", named["name"])
	}
	if !isNonNull {
		t.Error("expected isNonNull=true")
	}
	if isList {
		t.Error("expected isList=false")
	}
}

func TestUnwrapType_ListNonNull(t *testing.T) {
	typeRef := map[string]any{
		"kind": "NON_NULL",
		"ofType": map[string]any{
			"kind": "LIST",
			"ofType": map[string]any{
				"kind": "NON_NULL",
				"ofType": map[string]any{
					"kind": "OBJECT",
					"name": "User",
				},
			},
		},
	}
	named, isNonNull, isList := UnwrapType(typeRef)
	if named["name"] != "User" {
		t.Errorf("expected User, got %v", named["name"])
	}
	if !isNonNull {
		t.Error("expected isNonNull=true")
	}
	if !isList {
		t.Error("expected isList=true")
	}
}

func TestUnwrapType_Nil(t *testing.T) {
	named, isNonNull, isList := UnwrapType(nil)
	if len(named) == 0 {
		// Expected: returns empty map
	}
	_ = isNonNull
	_ = isList
}

func TestUnwrapType_NilOfType(t *testing.T) {
	// Kind is NON_NULL but ofType is missing
	typeRef := map[string]any{
		"kind": "NON_NULL",
	}
	named, _, _ := UnwrapType(typeRef)
	if len(named) != 0 {
		// Should gracefully return empty
	}
}

func TestGraphQLTypeString_Named(t *testing.T) {
	typeRef := map[string]any{"kind": "SCALAR", "name": "String"}
	if got := GraphQLTypeString(typeRef); got != "String" {
		t.Errorf("expected String, got %q", got)
	}
}

func TestGraphQLTypeString_NonNull(t *testing.T) {
	typeRef := map[string]any{
		"kind": "NON_NULL",
		"ofType": map[string]any{
			"kind": "SCALAR",
			"name": "ID",
		},
	}
	if got := GraphQLTypeString(typeRef); got != "ID!" {
		t.Errorf("expected ID!, got %q", got)
	}
}

func TestGraphQLTypeString_ListNonNull(t *testing.T) {
	typeRef := map[string]any{
		"kind": "LIST",
		"ofType": map[string]any{
			"kind": "NON_NULL",
			"ofType": map[string]any{
				"kind": "SCALAR",
				"name": "String",
			},
		},
	}
	if got := GraphQLTypeString(typeRef); got != "[String!]" {
		t.Errorf("expected [String!], got %q", got)
	}
}

func TestGraphQLTypeString_NestedNonNull(t *testing.T) {
	typeRef := map[string]any{
		"kind": "NON_NULL",
		"ofType": map[string]any{
			"kind": "LIST",
			"ofType": map[string]any{
				"kind": "NON_NULL",
				"ofType": map[string]any{
					"kind": "OBJECT",
					"name": "User",
				},
			},
		},
	}
	if got := GraphQLTypeString(typeRef); got != "[User!]!" {
		t.Errorf("expected [User!]!, got %q", got)
	}
}

func TestGraphQLTypeString_Nil(t *testing.T) {
	if got := GraphQLTypeString(nil); got != "String" {
		t.Errorf("expected String default, got %q", got)
	}
}

func TestGraphQLTypeString_NoName(t *testing.T) {
	typeRef := map[string]any{"kind": "SCALAR"}
	if got := GraphQLTypeString(typeRef); got != "String" {
		t.Errorf("expected String default, got %q", got)
	}
}

func TestTypeToPython_String(t *testing.T) {
	typeRef := map[string]any{"kind": "SCALAR", "name": "String"}
	pyType, req, choices := TypeToPython(typeRef, nil)
	if pyType != types.TypeString {
		t.Errorf("expected TypeString, got %v", pyType)
	}
	if req {
		t.Error("expected required=false")
	}
	if choices != nil {
		t.Errorf("expected nil choices, got %v", choices)
	}
}

func TestTypeToPython_NonNullInt(t *testing.T) {
	typeRef := map[string]any{
		"kind": "NON_NULL",
		"ofType": map[string]any{
			"kind": "SCALAR",
			"name": "Int",
		},
	}
	pyType, req, _ := TypeToPython(typeRef, nil)
	if pyType != types.TypeInt {
		t.Errorf("expected TypeInt, got %v", pyType)
	}
	if !req {
		t.Error("expected required=true")
	}
}

func TestTypeToPython_Float(t *testing.T) {
	typeRef := map[string]any{"kind": "SCALAR", "name": "Float"}
	pyType, _, _ := TypeToPython(typeRef, nil)
	if pyType != types.TypeFloat {
		t.Errorf("expected TypeFloat, got %v", pyType)
	}
}

func TestTypeToPython_Boolean(t *testing.T) {
	typeRef := map[string]any{"kind": "SCALAR", "name": "Boolean"}
	pyType, _, _ := TypeToPython(typeRef, nil)
	if pyType != types.TypeBoolean {
		t.Errorf("expected TypeBoolean, got %v", pyType)
	}
}

func TestTypeToPython_ID(t *testing.T) {
	typeRef := map[string]any{"kind": "SCALAR", "name": "ID"}
	pyType, _, _ := TypeToPython(typeRef, nil)
	if pyType != types.TypeString {
		t.Errorf("expected TypeString for ID, got %v", pyType)
	}
}

func TestTypeToPython_List(t *testing.T) {
	typeRef := map[string]any{
		"kind": "LIST",
		"ofType": map[string]any{
			"kind": "SCALAR",
			"name": "Int",
		},
	}
	pyType, _, _ := TypeToPython(typeRef, nil)
	if pyType != types.TypeString {
		t.Errorf("expected TypeString for list, got %v", pyType)
	}
}

func TestTypeToPython_Enum(t *testing.T) {
	typeRef := map[string]any{
		"kind": "ENUM",
		"name": "Status",
	}
	typesByName := map[string]map[string]any{
		"Status": {
			"kind": "ENUM",
			"name": "Status",
			"enumValues": []any{
				map[string]any{"name": "ACTIVE"},
				map[string]any{"name": "INACTIVE"},
			},
		},
	}
	pyType, _, choices := TypeToPython(typeRef, typesByName)
	if pyType != types.TypeString {
		t.Errorf("expected TypeString for enum, got %v", pyType)
	}
	if len(choices) != 2 {
		t.Errorf("expected 2 choices, got %d", len(choices))
	}
}

func TestTypeToPython_EnumNoValues(t *testing.T) {
	typeRef := map[string]any{
		"kind": "ENUM",
		"name": "Empty",
	}
	typesByName := map[string]map[string]any{
		"Empty": {
			"kind": "ENUM",
			"name": "Empty",
		},
	}
	pyType, _, choices := TypeToPython(typeRef, typesByName)
	if pyType != types.TypeString {
		t.Errorf("expected TypeString, got %v", pyType)
	}
	if choices != nil {
		t.Errorf("expected nil choices for empty enum, got %v", choices)
	}
}

func TestTypeToPython_InputObject(t *testing.T) {
	typeRef := map[string]any{
		"kind": "INPUT_OBJECT",
		"name": "CreateUserInput",
	}
	pyType, _, _ := TypeToPython(typeRef, nil)
	if pyType != types.TypeString {
		t.Errorf("expected TypeString for input object, got %v", pyType)
	}
}

func TestTypeToPython_UnknownScalar(t *testing.T) {
	typeRef := map[string]any{
		"kind": "SCALAR",
		"name": "DateTime",
	}
	pyType, _, _ := TypeToPython(typeRef, nil)
	if pyType != types.TypeString {
		t.Errorf("expected TypeString for unknown scalar, got %v", pyType)
	}
}
