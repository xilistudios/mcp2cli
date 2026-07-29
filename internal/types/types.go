// Package types defines the core data structures shared across all
// mcp2cli internal packages. These mirror the Python dataclasses and
// are kept deliberately free of external dependencies.
package types

// ---------------------------------------------------------------------------
// ParamType
// ---------------------------------------------------------------------------

// ParamType enumerates the CLI-facing parameter types. The string values
// MUST match the Python _python_type_name output exactly for --list --json
// parity:
//
//	python None  -> "boolean" (a store_true flag)
//	python int   -> "int"
//	python float -> "float"
//	python str   -> "str"
type ParamType string

const (
	TypeString  ParamType = "str"
	TypeInt     ParamType = "int"
	TypeFloat   ParamType = "float"
	TypeBoolean ParamType = "boolean"
)

// ---------------------------------------------------------------------------
// ParamDef
// ---------------------------------------------------------------------------

// ParamDef mirrors the Python ParamDef dataclass.
type ParamDef struct {
	Name         string         // kebab-case CLI flag
	OriginalName string         // original name for API/tool call
	Type         ParamType      // TypeBoolean means boolean flag (store_true)
	Required     bool           // whether the param is required
	Description  string         // help text
	Choices      []string       // nil when none
	Location     string         // path|query|header|body|tool_input|file|graphql_arg
	Schema       map[string]any // full JSON schema for the param
}

// IsBoolFlag reports whether p is a boolean store_true flag.
func (p ParamDef) IsBoolFlag() bool { return p.Type == TypeBoolean }

// ---------------------------------------------------------------------------
// CommandDef
// ---------------------------------------------------------------------------

// CommandDef mirrors the Python CommandDef dataclass.
type CommandDef struct {
	Name        string
	Description string
	Params      []ParamDef
	HasBody     bool

	// OpenAPI fields
	Method      string // "", or get/post/put/delete/patch (lowercase)
	Path        string
	ContentType string // "" = json, "multipart/form-data", etc.

	// MCP fields
	ToolName string

	// GraphQL fields
	GraphQLOperationType string         // "query" or "mutation"
	GraphQLFieldName     string         // original field name pre-kebab
	GraphQLReturnType    map[string]any // return type info for selection set
}

// ---------------------------------------------------------------------------
// BakeConfig
// ---------------------------------------------------------------------------

// BakeConfig mirrors the Python BakeConfig dataclass.
type BakeConfig struct {
	Include []string
	Exclude []string
	Methods []string
}
