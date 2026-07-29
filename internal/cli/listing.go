package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/cache"
	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// TruncateDescription truncates desc to maxLen characters, cutting at
// the last space if possible, and appending "...".
func TruncateDescription(desc string, maxLen int) string {
	if len(desc) <= maxLen {
		return desc
	}
	s := desc[:maxLen]
	if idx := strings.LastIndex(s, " "); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimRight(s, " ") + "..."
}

// WrapDescription word-wraps desc to fit within totalWidth columns.
// Continuation lines are prefixed with indent spaces.
// Words longer than totalWidth-indent are kept on their own line
// (break_long_words=false, break_on_hyphens=false).
func WrapDescription(desc string, indent, totalWidth int) string {
	if desc == "" {
		return ""
	}
	words := strings.Fields(desc)
	if len(words) == 0 {
		return ""
	}

	prefix := strings.Repeat(" ", indent)
	maxFirstLine := totalWidth
	maxContLine := totalWidth - indent
	if maxFirstLine < 1 {
		maxFirstLine = 1
	}
	if maxContLine < 1 {
		maxContLine = 1
	}

	var lines []string
	curLine := words[0]
	curWidth := len(words[0])

	for _, w := range words[1:] {
		needed := curWidth + 1 + len(w)
		limit := maxFirstLine
		if len(lines) > 0 {
			limit = maxContLine
		}
		if needed <= limit {
			curLine += " " + w
			curWidth += 1 + len(w)
		} else {
			lines = append(lines, curLine)
			curLine = w
			curWidth = len(w)
		}
	}
	lines = append(lines, curLine)

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
			b.WriteString(prefix)
		}
		b.WriteString(line)
	}
	return b.String()
}

// FilterCommandsSearch returns commands whose Name or Description
// contains pattern (case-insensitive substring match).
func FilterCommandsSearch(commands []types.CommandDef, pattern string) []types.CommandDef {
	if pattern == "" {
		return commands
	}
	lower := strings.ToLower(pattern)
	var out []types.CommandDef
	for _, cmd := range commands {
		if strings.Contains(strings.ToLower(cmd.Name), lower) ||
			strings.Contains(strings.ToLower(cmd.Description), lower) {
			out = append(out, cmd)
		}
	}
	return out
}

// ApplyListOptions sorts and trims commands per the given options.
func ApplyListOptions(commands []types.CommandDef, sourceHash, sortMode string, top int) []types.CommandDef {
	effective := cache.ResolveSortMode(sortMode, sourceHash)
	commands = cache.SortCommands(commands, effective, sourceHash)
	if top > 0 && top < len(commands) {
		commands = commands[:top]
	}
	return commands
}

// ListOptions controls how command lists are rendered.
type ListOptions struct {
	Verbose    bool
	Compact    bool
	JSONOutput bool
	Pretty     bool
	SourceHash string
	SortMode   string
	Top        int
	Out        io.Writer
}

// ListOpenAPI renders commands in OpenAPI style (grouped by prefix).
func ListOpenAPI(commands []types.CommandDef, o ListOptions) {
	commands = ApplyListOptions(commands, o.SourceHash, o.SortMode, o.Top)

	if o.JSONOutput {
		util.PrintCommandsJSON(commands, o.Compact, o.Pretty)
		return
	}
	if o.Compact {
		names := make([]string, len(commands))
		for i, cmd := range commands {
			names[i] = cmd.Name
		}
		fmt.Fprintln(o.Out, strings.Join(names, " "))
		return
	}

	// Group by prefix.
	groups := map[string][]types.CommandDef{}
	var groupOrder []string
	for _, cmd := range commands {
		prefix := "other"
		if idx := strings.Index(cmd.Name, "-"); idx >= 0 {
			prefix = cmd.Name[:idx]
		}
		if _, ok := groups[prefix]; !ok {
			groupOrder = append(groupOrder, prefix)
		}
		groups[prefix] = append(groups[prefix], cmd)
	}
	sort.Strings(groupOrder)

	for _, group := range groupOrder {
		fmt.Fprintf(o.Out, "\n%s:\n", group)
		for _, cmd := range groups[group] {
			line := fmt.Sprintf("  %-45s %-6s", cmd.Name, strings.ToUpper(cmd.Method))
			if cmd.Description != "" {
				if o.Verbose {
					line += " " + WrapDescription(cmd.Description, 54, 110)
				} else {
					line += " " + TruncateDescription(cmd.Description, 60)
				}
			}
			fmt.Fprintln(o.Out, line)
		}
	}
}

// ListMCP renders commands in MCP style (flat list).
func ListMCP(commands []types.CommandDef, o ListOptions) {
	commands = ApplyListOptions(commands, o.SourceHash, o.SortMode, o.Top)

	if o.JSONOutput {
		util.PrintCommandsJSON(commands, o.Compact, o.Pretty)
		return
	}
	if o.Compact {
		names := make([]string, len(commands))
		for i, cmd := range commands {
			names[i] = cmd.Name
		}
		fmt.Fprintln(o.Out, strings.Join(names, " "))
		return
	}

	for _, cmd := range commands {
		fmt.Fprintf(o.Out, "  %-40s", cmd.Name)
		if cmd.Description != "" {
			if o.Verbose {
				fmt.Fprint(o.Out, "  "+WrapDescription(cmd.Description, 42, 110))
			} else {
				fmt.Fprint(o.Out, " "+TruncateDescription(cmd.Description, 70))
			}
		}
		fmt.Fprintln(o.Out)
	}
}

// ListGraphQL renders commands in GraphQL style (grouped by operation type).
func ListGraphQL(commands []types.CommandDef, o ListOptions) {
	commands = ApplyListOptions(commands, o.SourceHash, o.SortMode, o.Top)

	if o.JSONOutput {
		util.PrintCommandsJSON(commands, o.Compact, o.Pretty)
		return
	}
	if o.Compact {
		names := make([]string, len(commands))
		for i, cmd := range commands {
			names[i] = cmd.Name
		}
		fmt.Fprintln(o.Out, strings.Join(names, " "))
		return
	}

	// Group by operation type.
	queries := []types.CommandDef{}
	mutations := []types.CommandDef{}
	for _, cmd := range commands {
		switch cmd.GraphQLOperationType {
		case "mutation":
			mutations = append(mutations, cmd)
		default:
			queries = append(queries, cmd)
		}
	}

	printGroup := func(header string, cmds []types.CommandDef) {
		if len(cmds) == 0 {
			return
		}
		fmt.Fprintf(o.Out, "\n%s:\n", header)
		for _, cmd := range cmds {
			fmt.Fprintf(o.Out, "  %-40s", cmd.Name)
			if cmd.Description != "" {
				if o.Verbose {
					fmt.Fprint(o.Out, "  "+WrapDescription(cmd.Description, 42, 100))
				} else {
					fmt.Fprint(o.Out, " "+TruncateDescription(cmd.Description, 60))
				}
			}
			fmt.Fprintln(o.Out)
		}
	}

	printGroup("queries", queries)
	printGroup("mutations", mutations)
}
