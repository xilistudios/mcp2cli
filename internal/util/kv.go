package util

import (
	"fmt"
	"strings"
)

// ParseKVList parses "KEY<delim>VALUE" items from a slice of strings.
// If resolveValues is true, each value is passed through ResolveSecret.
func ParseKVList(items []string, delimiter, label string, resolveValues bool) ([][2]string, error) {
	result := make([][2]string, 0, len(items))
	for _, item := range items {
		if !strings.Contains(item, delimiter) {
			return nil, fmt.Errorf("invalid %s format: %q", label, item)
		}
		parts := strings.SplitN(item, delimiter, 2)
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if resolveValues {
			resolved, err := ResolveSecret(v)
			if err != nil {
				return nil, err
			}
			v = resolved
		}
		result = append(result, [2]string{k, v})
	}
	return result, nil
}
