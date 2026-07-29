// Package mcp provides MCP (Model Context Protocol) client support for
// mcp2cli, using the mark3labs/mcp-go SDK.
package mcp

import (
	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// ExtractCommands converts MCP tool descriptors into CLI CommandDefs.
//
// Each tool is expected to be a map with keys "name" (string),
// "description" (string), and "inputSchema" (map).  This mirrors the
// Python extract_mcp_commands function exactly.
func ExtractCommands(tools []map[string]any) []types.CommandDef {
	commands := make([]types.CommandDef, 0, len(tools))
	for _, tool := range tools {
		name := util.ToKebab(toolStr(tool, "name", "unknown"))
		desc := toolStr(tool, "description", "")

		schema, _ := tool["inputSchema"].(map[string]any)
		if schema == nil {
			schema = map[string]any{}
		}

		// Build set of required field names.
		requiredSet := make(map[string]bool)
		if reqSlice, ok := schema["required"].([]any); ok {
			for _, r := range reqSlice {
				if s, ok := r.(string); ok {
					requiredSet[s] = true
				}
			}
		}

		// Iterate properties.
		var params []types.ParamDef
		if props, ok := schema["properties"].(map[string]any); ok {
			for propName, propRaw := range props {
				propSchema, _ := propRaw.(map[string]any)
				if propSchema == nil {
					propSchema = map[string]any{}
				}

				pyType, suffix := util.SchemaTypeToPython(propSchema)

				descText := toolStr(propSchema, "description", propName) + suffix

				params = append(params, types.ParamDef{
					Name:         util.ToKebab(propName),
					OriginalName: propName,
					Type:         pyType,
					Required:     requiredSet[propName],
					Description:  descText,
					Choices:      enumFromSchema(propSchema),
					Location:     "tool_input",
					Schema:       propSchema,
				})
			}
		}

		toolName, _ := tool["name"].(string)

		commands = append(commands, types.CommandDef{
			Name:        name,
			Description: desc,
			Params:      params,
			HasBody:     len(params) > 0,
			ToolName:    toolName,
		})
	}
	return commands
}

// enumFromSchema extracts string enum values from a JSON Schema property.
func enumFromSchema(schema map[string]any) []string {
	raw, ok := schema["enum"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toolStr extracts a string from a map, returning fallback if missing or
// not a string.
func toolStr(m map[string]any, key, fallback string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return fallback
}
