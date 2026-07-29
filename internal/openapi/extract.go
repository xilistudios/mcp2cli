package openapi

import (
	"fmt"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// ExtractCommands converts an OpenAPI spec's paths into CommandDef entries.
func ExtractCommands(spec map[string]any) []types.CommandDef {
	var commands []types.CommandDef
	seen := map[string]int{}

	paths, _ := spec["paths"].(map[string]any)
	methods := []string{"get", "post", "put", "delete", "patch"}

	for path, pathItem := range paths {
		methodsMap, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}
		for _, method := range methods {
			detailsRaw, ok := methodsMap[method]
			if !ok {
				continue
			}
			details, ok := detailsRaw.(map[string]any)
			if !ok {
				continue
			}

			// --- name ---
			var name string
			if opID, ok := details["operationId"].(string); ok && opID != "" {
				name = util.ToKebab(opID)
			} else {
				slug := path
				if len(slug) > 0 && slug[0] == '/' {
					slug = slug[1:]
				}
				if len(slug) > 0 && slug[len(slug)-1] == '/' {
					slug = slug[:len(slug)-1]
				}
				slug = strings.ReplaceAll(slug, "/", "-")
				slug = strings.ReplaceAll(slug, "{", "")
				slug = strings.ReplaceAll(slug, "}", "")
				if slug != "" {
					name = method + "-" + slug
				} else {
					name = method
				}
			}

			// Deduplicate: mirror Python seen_names logic exactly.
			if seen[name] > 0 {
				seen[name]++
				name = name + "-" + method
			}
			seen[name] = 1

			// --- description ---
			desc := ""
			if s, ok := details["summary"].(string); ok && s != "" {
				desc = s
			} else if s, ok := details["description"].(string); ok && s != "" {
				desc = s
			} else {
				desc = fmt.Sprintf("%s %s", methodUpper(method), path)
			}

			// --- parameters (path, query, header) ---
			var params []types.ParamDef

			if paramList, ok := details["parameters"].([]any); ok {
				for _, pi := range paramList {
					param, ok := pi.(map[string]any)
					if !ok {
						continue
					}
					schema := getMap(param, "schema")
					pyType, suffix := util.SchemaTypeToPython(schema)

					pName, _ := param["name"].(string)
					req, _ := param["required"].(bool)
					descVal := param["description"]
					if descVal == nil || fmt.Sprint(descVal) == "" {
						descVal = pName
					}
					loc := "query"
					if v, ok := param["in"].(string); ok {
						loc = v
					}

					params = append(params, types.ParamDef{
						Name:         util.ToKebab(pName),
						OriginalName: pName,
						Type:         pyType,
						Required:     req,
						Description:  fmt.Sprint(descVal) + suffix,
						Choices:      enumFromSchema(schema),
						Location:     loc,
						Schema:       schema,
					})
				}
			}

			// --- request body ---
			rbContent := getNested(details, "requestBody", "content")
			multipartSchema := getNested(rbContent, "multipart/form-data", "schema")
			jsonSchema := getNested(rbContent, "application/json", "schema")

			mpProps := getMap(multipartSchema, "properties")
			hasBinary := false
			for _, pv := range mpProps {
				if pm, ok := pv.(map[string]any); ok {
					if f, ok := pm["format"].(string); ok && f == "binary" {
						hasBinary = true
						break
					}
				}
			}

			var rbSchema map[string]any
			var contentType string

			switch {
			case hasBinary:
				rbSchema = multipartSchema
				contentType = "multipart/form-data"
			case len(jsonSchema) > 0:
				rbSchema = jsonSchema
			case len(mpProps) > 0:
				rbSchema = multipartSchema
				contentType = "multipart/form-data"
			default:
				rbSchema = map[string]any{}
			}

			requiredFields := map[string]bool{}
			if reqArr, ok := rbSchema["required"].([]any); ok {
				for _, r := range reqArr {
					if s, ok := r.(string); ok {
						requiredFields[s] = true
					}
				}
			}

			properties := getMap(rbSchema, "properties")
			hasBody := len(properties) > 0

			for propName, propRaw := range properties {
				propSchema, ok := propRaw.(map[string]any)
				if !ok {
					continue
				}
				isBinary := contentType == "multipart/form-data" &&
					propSchema["format"] == "binary"

				var loc string
				var pyType types.ParamType
				var suffix string

				if isBinary {
					loc = "file"
					pyType = types.TypeString
					suffix = " (file path)"
				} else {
					pyType, suffix = util.SchemaTypeToPython(propSchema)
					loc = "body"
				}

				propDesc := propSchema["description"]
				if propDesc == nil || fmt.Sprint(propDesc) == "" {
					propDesc = propName
				}

				params = append(params, types.ParamDef{
					Name:         util.ToKebab(propName),
					OriginalName: propName,
					Type:         pyType,
					Required:     requiredFields[propName],
					Description:  fmt.Sprint(propDesc) + suffix,
					Choices:      enumFromSchema(propSchema),
					Location:     loc,
					Schema:       propSchema,
				})
			}

			commands = append(commands, types.CommandDef{
				Name:        name,
				Description: desc,
				Params:      params,
				HasBody:     hasBody,
				Method:      method,
				Path:        path,
				ContentType: contentType,
			})
		}
	}
	return commands
}

// --- helpers ---

func getMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	v, ok := m[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return v
}

func getNested(m map[string]any, keys ...string) map[string]any {
	cur := m
	for _, k := range keys {
		cur = getMap(cur, k)
	}
	return cur
}

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

func methodUpper(method string) string {
	switch method {
	case "get":
		return "GET"
	case "post":
		return "POST"
	case "put":
		return "PUT"
	case "delete":
		return "DELETE"
	case "patch":
		return "PATCH"
	default:
		return method
	}
}
