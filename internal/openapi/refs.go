package openapi

import (
	"encoding/json"
	"strings"
)

// ResolveRefs returns a deep copy of spec with all JSON $ref pointers
// resolved. Only local "#/" refs are resolved; external refs are left
// as-is. A seen set prevents infinite loops on cyclic references.
func ResolveRefs(spec map[string]any) map[string]any {
	// Deep copy via JSON round-trip.
	b, _ := json.Marshal(spec)
	var copy map[string]any
	json.Unmarshal(b, &copy)

	return resolveNode(copy, copy, map[string]bool{}).(map[string]any)
}

func resolveNode(node any, root map[string]any, seen map[string]bool) any {
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok {
			if seen[ref] {
				return n // cycle — return as-is
			}
			// Copy-on-write: each $ref branch gets its own seen set
			// (mirrors Python's `seen = seen | {ref}`).
			branchSeen := make(map[string]bool, len(seen)+1)
			for k, v := range seen {
				branchSeen[k] = v
			}
			branchSeen[ref] = true
			seen = branchSeen
			if strings.HasPrefix(ref, "#/") {
				parts := strings.Split(ref[2:], "/")
				target := root
				for _, p := range parts {
					if m, ok := target[p].(map[string]any); ok {
						target = m
					} else {
						return n // broken ref — return as-is
					}
				}
				// Deep copy the resolved target and recurse into it.
				b, _ := json.Marshal(target)
				var cp map[string]any
				json.Unmarshal(b, &cp)
				return resolveNode(cp, root, seen)
			}
			return n // non-local ref
		}
		out := make(map[string]any, len(n))
		for k, v := range n {
			out[k] = resolveNode(v, root, seen)
		}
		return out
	case []any:
		out := make([]any, len(n))
		for i, v := range n {
			out[i] = resolveNode(v, root, seen)
		}
		return out
	default:
		return n
	}
}
