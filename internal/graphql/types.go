package graphql

import (
	"github.com/xilistudios/mcp2cli/internal/types"
)

// UnwrapType unwraps NON_NULL and LIST wrappers from a GraphQL type reference,
// returning the underlying named type along with whether the original was
// non-null or a list.
func UnwrapType(typeRef map[string]any) (map[string]any, bool, bool) {
	if typeRef == nil {
		return map[string]any{}, false, false
	}
	isNonNull := false
	isList := false
	t := typeRef
	for {
		if t == nil {
			return typeRef, isNonNull, isList
		}
		kind, _ := t["kind"].(string)
		switch kind {
		case "NON_NULL":
			isNonNull = true
			if ot, ok := t["ofType"].(map[string]any); ok {
				t = ot
			} else {
				return map[string]any{}, isNonNull, isList
			}
		case "LIST":
			isList = true
			if ot, ok := t["ofType"].(map[string]any); ok {
				t = ot
			} else {
				return map[string]any{}, isNonNull, isList
			}
		default:
			return t, isNonNull, isList
		}
	}
}

// toTypeMap safely converts an any value to map[string]any.
func toTypeMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// GraphQLTypeString reconstructs GraphQL type notation from an introspection
// type ref. E.g. "[String!]" or "ID!" or "Int".
func GraphQLTypeString(typeRef map[string]any) string {
	if typeRef == nil {
		return "String"
	}
	kind, _ := typeRef["kind"].(string)
	switch kind {
	case "NON_NULL":
		inner := GraphQLTypeString(toTypeMap(typeRef["ofType"]))
		return inner + "!"
	case "LIST":
		inner := GraphQLTypeString(toTypeMap(typeRef["ofType"]))
		return "[" + inner + "]"
	default:
		if name, ok := typeRef["name"].(string); ok && name != "" {
			return name
		}
		return "String"
	}
}

// TypeToPython maps a GraphQL introspection type to the corresponding
// ParamType, whether it is required, and optional enum choices.
//
// typesByName is the map of type name -> type definition from the schema.
func TypeToPython(typeRef map[string]any, typesByName map[string]map[string]any) (types.ParamType, bool, []string) {
	named, isNonNull, isList := UnwrapType(typeRef)
	typeName, _ := named["name"].(string)
	typeKind, _ := named["kind"].(string)

	if isList {
		return types.TypeString, isNonNull, nil
	}

	if typeKind == "ENUM" {
		enumType := typesByName[typeName]
		if enumType == nil {
			return types.TypeString, isNonNull, nil
		}
		enumValuesRaw, ok := enumType["enumValues"].([]any)
		if !ok || len(enumValuesRaw) == 0 {
			return types.TypeString, isNonNull, nil
		}
		choices := make([]string, 0, len(enumValuesRaw))
		for _, ev := range enumValuesRaw {
			if evMap, ok := ev.(map[string]any); ok {
				if name, ok := evMap["name"].(string); ok {
					choices = append(choices, name)
				}
			}
		}
		if len(choices) == 0 {
			return types.TypeString, isNonNull, nil
		}
		return types.TypeString, isNonNull, choices
	}

	if typeKind == "INPUT_OBJECT" {
		return types.TypeString, isNonNull, nil
	}

	// Scalar mapping
	scalarMap := map[string]types.ParamType{
		"String":  types.TypeString,
		"ID":      types.TypeString,
		"Int":     types.TypeInt,
		"Float":   types.TypeFloat,
		"Boolean": types.TypeBoolean,
	}
	pyType, ok := scalarMap[typeName]
	if !ok {
		pyType = types.TypeString
	}
	return pyType, isNonNull, nil
}
