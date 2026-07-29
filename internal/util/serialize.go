package util

import (
	"strings"

	"github.com/xilistudios/mcp2cli/internal/types"
)

// ParamToDict converts a ParamDef to a map for JSON serialization.
func ParamToDict(p types.ParamDef) map[string]any {
	d := map[string]any{
		"name":        p.Name,
		"type":        string(p.Type),
		"required":    p.Required,
		"description": p.Description,
		"location":    p.Location,
	}
	if p.Choices != nil {
		d["choices"] = p.Choices
	}
	return d
}

// CommandToDict converts a CommandDef to a map for JSON serialization.
func CommandToDict(cmd types.CommandDef) map[string]any {
	d := map[string]any{
		"name":        cmd.Name,
		"description": cmd.Description,
	}
	if cmd.Method != "" {
		d["method"] = strings.ToUpper(cmd.Method)
	}
	if cmd.Path != "" {
		d["path"] = cmd.Path
	}
	if cmd.ToolName != "" {
		d["toolName"] = cmd.ToolName
	}
	if cmd.GraphQLOperationType != "" {
		d["operationType"] = cmd.GraphQLOperationType
	}
	params := make([]map[string]any, 0, len(cmd.Params))
	for _, p := range cmd.Params {
		params = append(params, ParamToDict(p))
	}
	d["parameters"] = params
	return d
}

// PrintCommandsJSON emits commands as JSON. If compact is true, only names are included.
func PrintCommandsJSON(commands []types.CommandDef, compact, pretty bool) {
	if compact {
		names := make([]any, 0, len(commands))
		for _, cmd := range commands {
			names = append(names, cmd.Name)
		}
		EmitJSON(names, pretty)
		return
	}
	arr := make([]any, 0, len(commands))
	for _, cmd := range commands {
		arr = append(arr, CommandToDict(cmd))
	}
	EmitJSON(arr, pretty)
}
