package util

import (
	"regexp"
	"strings"
)

var camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// ToKebab converts a camelCase or snake_case name to kebab-case.
func ToKebab(name string) string {
	s := camelBoundary.ReplaceAllString(name, "$1-$2")
	return strings.ToLower(strings.ReplaceAll(s, "_", "-"))
}
