package graphql

import (
	"sort"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// BuildSelectionSet auto-generates a GraphQL selection set from a return type.
//
// depth controls how many levels of nested objects are expanded (default 2).
// seen tracks already-visited type names to avoid infinite recursion.
func BuildSelectionSet(typeRef map[string]any, typesByName map[string]map[string]any, depth int, seen map[string]bool) string {
	if seen == nil {
		seen = make(map[string]bool)
	}

	named, _, _ := UnwrapType(typeRef)
	typeName, _ := named["name"].(string)
	typeKind, _ := named["kind"].(string)

	// Scalar / enum — no selection needed
	if typeKind == "SCALAR" || typeKind == "ENUM" {
		return ""
	}

	if seen[typeName] || depth <= 0 {
		return ""
	}

	typeDef := typesByName[typeName]
	if typeDef == nil {
		return ""
	}
	fieldsRaw, ok := typeDef["fields"].([]any)
	if !ok || len(fieldsRaw) == 0 {
		return ""
	}

	// Mark this type as seen (copy to avoid cross-branch pollution).
	seenCopy := make(map[string]bool, len(seen)+1)
	for k, v := range seen {
		seenCopy[k] = v
	}
	seenCopy[typeName] = true

	var parts []string
	for _, fRaw := range fieldsRaw {
		f, ok := fRaw.(map[string]any)
		if !ok {
			continue
		}
		fName, _ := f["name"].(string)
		if fName == "" {
			continue
		}
		fType := toTypeMap(f["type"])
		fNamed, _, _ := UnwrapType(fType)
		fKind, _ := fNamed["kind"].(string)

		if fKind == "SCALAR" || fKind == "ENUM" {
			parts = append(parts, fName)
		} else if fKind == "OBJECT" && depth > 1 {
			nested := BuildSelectionSet(fType, typesByName, depth-1, seenCopy)
			if nested != "" {
				parts = append(parts, fName+" "+nested)
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return "{ " + strings.Join(parts, " ") + " }"
}

// detectFieldCollisions returns field names that appear in both query and
// mutation types.
func detectFieldCollisions(queryFields, mutationFields []any) map[string]bool {
	names := make(map[string]bool)
	collisions := make(map[string]bool)

	for _, fields := range []any{queryFields, mutationFields} {
		switch fs := fields.(type) {
		case []any:
			for _, fRaw := range fs {
				if f, ok := fRaw.(map[string]any); ok {
					n, _ := f["name"].(string)
					if n != "" {
						if names[n] {
							collisions[n] = true
						}
						names[n] = true
					}
				}
			}
		}
	}
	return collisions
}

// buildGraphQLParam converts a single GraphQL field argument into a ParamDef.
func buildGraphQLParam(arg map[string]any, typesByName map[string]map[string]any) types.ParamDef {
	argType := toTypeMap(arg["type"])
	gqlTypeStr := GraphQLTypeString(argType)
	pyType, required, choices := TypeToPython(argType, typesByName)
	namedT, _, isList := UnwrapType(argType)

	// Build schema for CoerceValue
	paramSchema := map[string]any{
		"graphql_type": gqlTypeStr,
	}
	if isList {
		paramSchema["type"] = "array"
		innerNamed, _, _ := UnwrapType(namedT)
		itemTypeName, _ := innerNamed["name"].(string)
		if itemTypeName == "" {
			itemTypeName = "String"
		}
		itemMap := map[string]string{
			"Int":     "integer",
			"Float":   "number",
			"String":  "string",
			"ID":      "string",
			"Boolean": "boolean",
		}
		paramSchema["items"] = map[string]any{"type": itemMap[itemTypeName]}
	} else {
		kindT, _ := namedT["kind"].(string)
		switch kindT {
		case "INPUT_OBJECT":
			paramSchema["type"] = "object"
		case "ENUM":
			paramSchema["type"] = "string"
		}
	}

	argDesc, _ := arg["description"].(string)
	if argDesc == "" {
		argDesc, _ = arg["name"].(string)
	}
	if isList {
		argDesc += " (JSON array)"
	} else if kindT, _ := namedT["kind"].(string); kindT == "INPUT_OBJECT" {
		argDesc += " (JSON object)"
	}

	origName, _ := arg["name"].(string)
	return types.ParamDef{
		Name:         util.ToKebab(origName),
		OriginalName: origName,
		Type:         pyType,
		Required:     required,
		Description:  argDesc,
		Choices:      choices,
		Location:     "graphql_arg",
		Schema:       paramSchema,
	}
}

// ExtractCommands converts an introspection schema into a list of CommandDefs.
func ExtractCommands(schema map[string]any) []types.CommandDef {
	typesByName := buildTypesByName(schema)

	queryType := schema["queryType"]
	mutationType := schema["mutationType"]

	var queryTypeName, mutationTypeName string
	if queryType != nil {
		if qt, ok := queryType.(map[string]any); ok {
			queryTypeName, _ = qt["name"].(string)
		}
	}
	if mutationType != nil {
		if mt, ok := mutationType.(map[string]any); ok {
			mutationTypeName, _ = mt["name"].(string)
		}
	}

	queryFields := getFields(typesByName, queryTypeName)
	mutationFields := getFields(typesByName, mutationTypeName)
	collisions := detectFieldCollisions(queryFields, mutationFields)

	type opSpec struct {
		opType   string
		typeName string
		fields   []any
	}
	ops := []opSpec{
		{"query", queryTypeName, queryFields},
		{"mutation", mutationTypeName, mutationFields},
	}

	var commands []types.CommandDef
	seenNames := make(map[string]bool)

	for _, op := range ops {
		for _, fieldRaw := range op.fields {
			fieldDef, ok := fieldRaw.(map[string]any)
			if !ok {
				continue
			}
			fieldName, _ := fieldDef["name"].(string)
			if fieldName == "" || strings.HasPrefix(fieldName, "__") {
				continue
			}

			cliName := util.ToKebab(fieldName)
			if collisions[fieldName] {
				cliName = op.opType + "-" + cliName
			}
			if seenNames[cliName] {
				cliName = op.opType + "-" + cliName
			}
			seenNames[cliName] = true

			desc, _ := fieldDef["description"].(string)
			if desc == "" {
				desc = op.opType + " " + fieldName
			}

			argsRaw, _ := fieldDef["args"].([]any)
			params := make([]types.ParamDef, 0, len(argsRaw))
			for _, argRaw := range argsRaw {
				if arg, ok := argRaw.(map[string]any); ok {
					params = append(params, buildGraphQLParam(arg, typesByName))
				}
			}

			commands = append(commands, types.CommandDef{
				Name:                   cliName,
				Description:            desc,
				Params:                 params,
				HasBody:                len(params) > 0,
				GraphQLOperationType:   op.opType,
				GraphQLFieldName:       fieldName,
				GraphQLReturnType:      toTypeMap(fieldDef["type"]),
			})
		}
	}

	return commands
}

// buildTypesByName creates a type-name -> type-definition map from the schema.
func buildTypesByName(schema map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any)
	typesRaw, ok := schema["types"].([]any)
	if !ok {
		return result
	}
	for _, t := range typesRaw {
		if m, ok := t.(map[string]any); ok {
			if name, ok := m["name"].(string); ok && name != "" {
				result[name] = m
			}
		}
	}
	return result
}

// getFields returns the "fields" array for the given type name, or nil.
func getFields(typesByName map[string]map[string]any, typeName string) []any {
	if typeName == "" {
		return nil
	}
	t := typesByName[typeName]
	if t == nil {
		return nil
	}
	fields, _ := t["fields"].([]any)
	return fields
}

// SortedCommandNames returns the command names sorted alphabetically.
// Useful for deterministic listing.
func SortedCommandNames(commands []types.CommandDef) []string {
	names := make([]string, len(commands))
	for i, c := range commands {
		names[i] = c.Name
	}
	sort.Strings(names)
	return names
}
