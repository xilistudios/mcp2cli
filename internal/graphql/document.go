package graphql

import (
	"fmt"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// BuildDocument constructs a GraphQL query/mutation document string and a
// variables map from a CommandDef, the parsed CLI arguments, and the schema.
//
// It returns (document, variables, fieldName).
//
// If fieldsOverride is non-empty it is used as the selection set verbatim;
// otherwise the selection set is auto-generated from the return type.
//
// stdinVars, when non-nil, is used as the variables map directly (from
// --stdin). When nil, variables are built from the parsed param values.
func BuildDocument(
	cmd types.CommandDef,
	schema map[string]any,
	stdinVars map[string]any,
	paramValues map[string]any,
	fieldsOverride string,
) (string, map[string]any, string) {
	typesByName := buildTypesByName(schema)

	// Build variables map
	var variables map[string]any
	if stdinVars != nil {
		variables = stdinVars
	} else {
		variables = make(map[string]any)
		for _, p := range cmd.Params {
			val := paramValues[p.Name]
			if val != nil {
				variables[p.OriginalName] = util.CoerceValue(val, p.Schema)
			}
		}
	}

	// Build variable declarations
	var varDecls []string
	for _, p := range cmd.Params {
		if _, ok := variables[p.OriginalName]; ok {
			gqlType, _ := p.Schema["graphql_type"].(string)
			if gqlType == "" {
				gqlType = "String"
			}
			varDecls = append(varDecls, fmt.Sprintf("$%s: %s", p.OriginalName, gqlType))
		}
	}

	// Build selection set
	var selection string
	if fieldsOverride != "" {
		selection = "{ " + fieldsOverride + " }"
	} else if cmd.GraphQLReturnType != nil {
		selection = BuildSelectionSet(cmd.GraphQLReturnType, typesByName, 2, nil)
	}

	// Build argument list for the field
	var fieldArgs []string
	for _, p := range cmd.Params {
		if _, ok := variables[p.OriginalName]; ok {
			fieldArgs = append(fieldArgs, fmt.Sprintf("%s: $%s", p.OriginalName, p.OriginalName))
		}
	}

	fieldName := cmd.GraphQLFieldName
	if fieldName == "" {
		fieldName = cmd.Name
	}
	opType := cmd.GraphQLOperationType
	if opType == "" {
		opType = "query"
	}

	var b strings.Builder
	b.WriteString(opType)
	if len(varDecls) > 0 {
		b.WriteString("(")
		b.WriteString(strings.Join(varDecls, ", "))
		b.WriteString(")")
	}
	b.WriteString(" { ")
	b.WriteString(fieldName)
	if len(fieldArgs) > 0 {
		b.WriteString("(")
		b.WriteString(strings.Join(fieldArgs, ", "))
		b.WriteString(")")
	}
	b.WriteString(" ")
	b.WriteString(selection)
	b.WriteString(" }")

	return b.String(), variables, fieldName
}
