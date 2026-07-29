// Package bake implements baked-tool management for mcp2cli: filtering,
// config persistence, CLI argv reconstruction, and CRUD operations.
package bake

import (
	"regexp"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/types"
)

// globMatch reports whether name matches an fnmatch-style pattern.
// Supported wildcards: * (any sequence), ? (single char), [seq] (char class).
// Implementation: translate the pattern to a regexp, anchor with ^...$.
func globMatch(pattern, name string) bool {
	re := globToRegexp(pattern)
	matched, _ := regexp.MatchString(re, name)
	return matched
}

// globToRegexp translates an fnmatch-style pattern to a regexp string.
func globToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '[':
			// Find the closing bracket.
			end := strings.IndexByte(pattern[i:], ']')
			if end < 0 || end == 1 {
				// No closing bracket or empty class — treat '[' as literal.
				b.WriteString(regexp.QuoteMeta("["))
			} else {
				// Emit the character class as-is (already valid in regexp).
				b.WriteString(pattern[i : i+end+1])
				i += end
			}
		default:
			// Escape regex metacharacters.
			b.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	b.WriteByte('$')
	return b.String()
}

// FilterCommands filters commands by HTTP method, include whitelist, then
// exclude blacklist (in that order). Commands with an empty Method (MCP)
// always pass the methods filter.
func FilterCommands(commands []types.CommandDef, include, exclude, methods []string) []types.CommandDef {
	result := make([]types.CommandDef, 0, len(commands))
	result = append(result, commands...)

	// 1. Methods filter.
	if len(methods) > 0 {
		upper := make([]string, len(methods))
		for i, m := range methods {
			upper[i] = strings.ToUpper(m)
		}
		filtered := result[:0]
		for _, cmd := range result {
			if cmd.Method == "" || containsStr(upper, strings.ToUpper(cmd.Method)) {
				filtered = append(filtered, cmd)
			}
		}
		result = filtered
	}

	// 2. Include whitelist.
	if len(include) > 0 {
		filtered := result[:0]
		for _, cmd := range result {
			if anyGlobMatch(include, cmd.Name) {
				filtered = append(filtered, cmd)
			}
		}
		result = filtered
	}

	// 3. Exclude blacklist.
	if len(exclude) > 0 {
		filtered := result[:0]
		for _, cmd := range result {
			if !anyGlobMatch(exclude, cmd.Name) {
				filtered = append(filtered, cmd)
			}
		}
		result = filtered
	}

	return result
}

func containsStr(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}

func anyGlobMatch(patterns []string, name string) bool {
	for _, pat := range patterns {
		if globMatch(pat, name) {
			return true
		}
	}
	return false
}
