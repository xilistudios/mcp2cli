package util

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/types"
)

// SchemaTypeToPython maps a JSON Schema "type" to a ParamType and optional suffix.
func SchemaTypeToPython(schema map[string]any) (types.ParamType, string) {
	switch schema["type"] {
	case "integer":
		return types.TypeInt, ""
	case "number":
		return types.TypeFloat, ""
	case "boolean":
		return types.TypeBoolean, ""
	case "array":
		return types.TypeString, " (JSON array)"
	case "object":
		return types.TypeString, " (JSON object)"
	default:
		return types.TypeString, ""
	}
}

// coerceItem converts a single string value to the given JSON schema type.
func coerceItem(value string, itemType any) any {
	switch itemType {
	case "integer":
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
		return value
	case "number":
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
		return value
	case "boolean":
		lower := strings.ToLower(value)
		return lower == "true" || lower == "1" || lower == "yes"
	default:
		return value
	}
}

// CoerceValue converts a value according to a JSON Schema type definition.
func CoerceValue(value any, schema map[string]any) any {
	if value == nil {
		return nil
	}
	t := schema["type"]

	switch t {
	case "array":
		// Already []any
		if _, ok := value.([]any); ok {
			return value
		}
		if s, ok := value.(string); ok {
			// Get item type from schema
			var itemType any
			if items, ok := schema["items"].(map[string]any); ok {
				itemType = items["type"]
			}
			// Try full JSON parse
			var parsed any
			if json.Unmarshal([]byte(s), &parsed) == nil {
				if arr, ok := parsed.([]any); ok {
					// Coerce items if we have a type
					if itemType != nil {
						for i, v := range arr {
							if vs, ok := v.(string); ok {
								arr[i] = coerceItem(vs, itemType)
							} else if itemType == "integer" {
								if f, ok := v.(float64); ok {
									arr[i] = int(f)
								}
							}
						}
					}
					return arr
				}
			}
			// Comma-split
			if strings.Contains(s, ",") {
				parts := strings.Split(s, ",")
				out := make([]any, 0, len(parts))
				for _, p := range parts {
					out = append(out, coerceItem(strings.TrimSpace(p), itemType))
				}
				return out
			}
			return []any{coerceItem(s, itemType)}
		}
		return value

	case "object":
		if s, ok := value.(string); ok {
			var parsed any
			if json.Unmarshal([]byte(s), &parsed) == nil {
				return parsed
			}
			return value
		}
		return value

	case "boolean":
		if b, ok := value.(bool); ok {
			return b
		}
		if s, ok := value.(string); ok {
			lower := strings.ToLower(s)
			return lower == "true" || lower == "1" || lower == "yes"
		}
		return value

	case "integer":
		switch v := value.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
			return value
		default:
			return value
		}

	case "number":
		switch v := value.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
			return value
		default:
			return value
		}

	default:
		// type key absent (nil): try to detect JSON object/array in strings
		if s, ok := value.(string); ok {
			tr := strings.TrimSpace(s)
			if tr != "" && (tr[0] == '{' || tr[0] == '[') {
				var parsed any
				if json.Unmarshal([]byte(tr), &parsed) == nil {
					if _, isMap := parsed.(map[string]any); isMap {
						return parsed
					}
					if _, isArr := parsed.([]any); isArr {
						return parsed
					}
				}
			}
		}
		return value
	}
}
