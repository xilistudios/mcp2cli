// Package util provides shared helpers for secret resolution, CLI output,
// type coercion, and small utilities used across mcp2cli internals.
package util

import (
	"fmt"
	"os"
	"strings"
)

// ResolveSecret resolves env:/file: prefixes, else returns the literal value.
func ResolveSecret(value string) (string, error) {
	if strings.HasPrefix(value, "env:") {
		varName := strings.TrimPrefix(value, "env:")
		v, ok := os.LookupEnv(varName)
		if !ok {
			return "", fmt.Errorf("environment variable %q is not set", varName)
		}
		return v, nil
	}
	if strings.HasPrefix(value, "file:") {
		path := strings.TrimPrefix(value, "file:")
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("secret file not found: %s", path)
		}
		return strings.TrimRight(string(content), "\n"), nil
	}
	return value, nil
}
