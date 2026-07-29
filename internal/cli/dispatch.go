// Package cli implements the command-line interface for mcp2cli.
// dispatch.go wires the global flag parsing, source-mode routing,
// and the three mode handlers (OpenAPI, GraphQL, MCP) into a single
// Run entrypoint.
package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/bake"
	"github.com/xilistudios/mcp2cli/internal/cache"
	"github.com/xilistudios/mcp2cli/internal/graphql"
	"github.com/xilistudios/mcp2cli/internal/mcp"
	"github.com/xilistudios/mcp2cli/internal/openapi"
	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// ---------------------------------------------------------------------------
// Silent error sentinel — main.go skips printing when err.Error() == ""
// ---------------------------------------------------------------------------

type silentErr struct{}

func (silentErr) Error() string { return "" }

var errSilent = silentErr{}

// ---------------------------------------------------------------------------
// Public entrypoint
// ---------------------------------------------------------------------------

// Run is the main entrypoint for the CLI. It dispatches to the bake
// subcommand, baked-tool shortcut (@name), or the normal mainImpl flow.
func Run(argv []string) error {
	if len(argv) > 0 {
		if argv[0] == "bake" {
			return runBake(argv[1:])
		}
		if strings.HasPrefix(argv[0], "@") {
			return runBaked(argv[0][1:], argv[1:])
		}
	}
	return mainImpl(argv, nil)
}

// ---------------------------------------------------------------------------
// Baked-tool runner
// ---------------------------------------------------------------------------

// runBaked loads a baked config by name and re-enters mainImpl with the
// baked argv prepended and the bake filter config attached.
func runBaked(name string, extra []string) error {
	cfg, ok, err := bake.Load(name)
	if err != nil {
		return fmt.Errorf("loading baked tool %q: %w", name, err)
	}
	if !ok {
		return fmt.Errorf("no baked tool named %q", name)
	}
	synthetic := append(bake.BakedToArgv(cfg), extra...)
	bakeCfg := &bake.Config{
		Include: cfg.Include,
		Exclude: cfg.Exclude,
		Methods: cfg.Methods,
	}
	return mainImpl(synthetic, bakeCfg)
}

// ---------------------------------------------------------------------------
// mainImpl — core CLI logic
// ---------------------------------------------------------------------------

func mainImpl(argv []string, baked *bake.Config) error {
	// 1. Parse global flags.
	g := &GlobalFlags{}
	fs, valueOpts, boolOpts := NewGlobalFlagSet(g)
	globalArgv, toolArgv := SplitAtSubcommand(argv, valueOpts, boolOpts)

	if err := fs.Parse(globalArgv); err != nil {
		return err
	}
	remaining := append(append([]string{}, fs.Args()...), toolArgv...)

	// 4. --search implies --list
	if g.SearchPattern != "" {
		g.ListCommands = true
	}

	// 5. Parse auth headers and env vars.
	authHeaders, err := util.ParseKVList(g.AuthHeaders, ":", "auth header", true)
	if err != nil {
		return err
	}
	envPairs, err := util.ParseKVList(g.Env, "=", "env", false)
	if err != nil {
		return err
	}
	envVars := make(map[string]string, len(envPairs))
	for _, p := range envPairs {
		envVars[p[0]] = p[1]
	}

	// 6. Version
	if g.ShowVersion {
		fmt.Fprintln(util.Out, "mcp2cli 3.3.1")
		return nil
	}

	// 7. Validate source modes.
	needsSource := !(g.SessionList || g.SessionStop != "" || g.Session != "")
	active := 0
	for _, s := range []string{g.Spec, g.MCP, g.MCPStdio, g.GraphQL} {
		if s != "" {
			active++
		}
	}
	if needsSource && active == 0 {
		return fmt.Errorf("one of --spec, --mcp, --mcp-stdio, or --graphql is required.")
	}
	if active > 1 {
		return fmt.Errorf("--spec, --mcp, --mcp-stdio, and --graphql are mutually exclusive.")
	}

	// 8. OAuth stub.
	if g.OAuth || g.OAuthClientID != "" || g.OAuthClientSecret != "" {
		return fmt.Errorf("OAuth is not yet implemented in the Go port")
	}

	// 9. Sessions stub.
	if g.SessionList || g.SessionStop != "" || g.SessionStart != "" || g.Session != "" {
		return fmt.Errorf("sessions are not yet implemented in the Go port")
	}

	// 10. Resolve resource/prompt actions.
	resourceAction := ""
	if g.ListResources {
		resourceAction = "list"
	} else if g.ListResourceTemplates {
		resourceAction = "templates"
	} else if g.ReadResource != "" {
		resourceAction = "read"
	}
	promptAction := ""
	if g.ListPrompts {
		promptAction = "list"
	} else if g.GetPrompt != "" {
		promptAction = "get"
	}

	// 11. Route to the correct mode handler.
	if g.GraphQL != "" {
		return handleGraphQL(g, authHeaders, remaining)
	}
	if g.MCP != "" || g.MCPStdio != "" {
		return handleMCP(g, authHeaders, envVars, remaining, resourceAction, promptAction, baked)
	}
	return handleOpenAPI(g, authHeaders, remaining, baked)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func headPtr(head int) *int {
	if head > 0 {
		return &head
	}
	return nil
}

func outOpts(g *GlobalFlags) util.OutputOptions {
	return util.OutputOptions{
		Pretty:     g.Pretty,
		Raw:        g.Raw,
		Toon:       g.Toon,
		Head:       headPtr(g.Head),
		JSONOutput: g.JSONOutput,
	}
}

func listOpts(g *GlobalFlags, srcHash string) ListOptions {
	return ListOptions{
		Verbose:    g.Verbose,
		Compact:    g.Compact,
		JSONOutput: g.JSONOutput,
		Pretty:     g.Pretty,
		SourceHash: srcHash,
		SortMode:   g.SortMode,
		Top:        g.Top,
		Out:        util.Out,
	}
}

func findCommand(commands []types.CommandDef, name string) (types.CommandDef, bool) {
	for _, cmd := range commands {
		if cmd.Name == name {
			return cmd, true
		}
	}
	return types.CommandDef{}, false
}

// ---------------------------------------------------------------------------
// OpenAPI mode
// ---------------------------------------------------------------------------

func handleOpenAPI(g *GlobalFlags, authHeaders [][2]string, remaining []string, baked *bake.Config) error {
	srcHash := cache.SourceHashFor(g.Spec)

	spec, err := openapi.LoadSpec(g.Spec, authHeaders, g.CacheKey, g.CacheTTL, g.Refresh, nil)
	if err != nil {
		return err
	}

	commands := openapi.ExtractCommands(spec)
	if baked != nil {
		commands = bake.FilterCommands(commands, baked.Include, baked.Exclude, baked.Methods)
	}

	// List mode
	if g.ListCommands {
		if g.SearchPattern != "" {
			commands = FilterCommandsSearch(commands, g.SearchPattern)
			if len(commands) == 0 {
				if !g.JSONOutput {
					fmt.Fprintln(util.Out, "No tools matching '"+g.SearchPattern+"'.")
				}
				ListOpenAPI(commands, listOpts(g, srcHash))
				return nil
			}
			if !g.Compact && !g.JSONOutput {
				fmt.Fprintln(util.Out, "Tools matching '"+g.SearchPattern+"':")
			}
		}
		ListOpenAPI(commands, listOpts(g, srcHash))
		return nil
	}

	// No subcommand given
	if len(remaining) == 0 {
		fmt.Fprintln(util.Err, "Use --list to see all available commands.")
		return errSilent
	}

	// Derive base URL
	baseURL := g.BaseURL
	if baseURL == "" {
		if servers, ok := spec["servers"].([]any); ok && len(servers) > 0 {
			if srvMap, ok := servers[0].(map[string]any); ok {
				if u, ok := srvMap["url"].(string); ok {
					baseURL = u
				}
			}
		}
		if baseURL == "" || !strings.HasPrefix(baseURL, "http") {
			if strings.HasPrefix(g.Spec, "http") {
				parsed, perr := url.Parse(g.Spec)
				if perr == nil {
					origin := parsed.Scheme + "://" + parsed.Host
					if baseURL != "" && !strings.HasPrefix(baseURL, "http") {
						baseURL = origin + baseURL
					} else {
						baseURL = origin
					}
				}
			} else if baseURL == "" {
				return fmt.Errorf("cannot determine base URL. Use --base-url.")
			}
		}
	}

	// Find and execute the command
	cmd, ok := findCommand(commands, remaining[0])
	if !ok {
		return fmt.Errorf("unknown command: %s (see --list)", remaining[0])
	}

	values, hasStdin, err := ParseCommandArgs(cmd, remaining[1:])
	if err != nil {
		return err
	}

	var stdinJSON any
	if hasStdin {
		stdinJSON, err = util.ReadStdinJSON("OpenAPI request body")
		if err != nil {
			return err
		}
	}

	req, err := openapi.BuildRequest(cmd, baseURL, values, hasStdin, stdinJSON)
	if err != nil {
		return err
	}

	if err := openapi.Execute(req, authHeaders, outOpts(g), nil); err != nil {
		return err
	}

	cache.RecordUsage(srcHash, cmd.Name) // ignore error
	return nil
}

// ---------------------------------------------------------------------------
// GraphQL mode
// ---------------------------------------------------------------------------

func handleGraphQL(g *GlobalFlags, authHeaders [][2]string, remaining []string) error {
	srcHash := cache.SourceHashFor(g.GraphQL)

	schema, err := graphql.LoadSchema(g.GraphQL, authHeaders, g.CacheKey, g.CacheTTL, g.Refresh, nil)
	if err != nil {
		return err
	}

	commands := graphql.ExtractCommands(schema)

	// List mode
	if g.ListCommands {
		if g.SearchPattern != "" {
			commands = FilterCommandsSearch(commands, g.SearchPattern)
			if len(commands) == 0 {
				if !g.JSONOutput {
					fmt.Fprintln(util.Out, "No operations matching '"+g.SearchPattern+"'.")
				}
				ListGraphQL(commands, listOpts(g, srcHash))
				return nil
			}
			if !g.Compact && !g.JSONOutput {
				fmt.Fprintln(util.Out, "Operations matching '"+g.SearchPattern+"':")
			}
		}
		ListGraphQL(commands, listOpts(g, srcHash))
		return nil
	}

	// No subcommand given
	if len(remaining) == 0 {
		if !g.Compact && !g.JSONOutput {
			fmt.Fprintln(util.Out, "Available operations:")
		}
		ListGraphQL(commands, listOpts(g, srcHash))
		if !g.Compact && !g.JSONOutput {
			fmt.Fprintln(util.Out, "\nUse --list for the same output, or provide a subcommand.")
		}
		fmt.Fprintln(util.Err, "Use --list to see all available commands.")
		return errSilent
	}

	// Find and execute the command
	cmd, ok := findCommand(commands, remaining[0])
	if !ok {
		return fmt.Errorf("unknown command: %s (see --list)", remaining[0])
	}

	values, _, err := ParseCommandArgs(cmd, remaining[1:])
	if err != nil {
		return err
	}

	// Build paramValues keyed by OriginalName for GraphQL execution.
	paramValues := make(map[string]any)
	for _, p := range cmd.Params {
		if v, ok := values[p.Name]; ok {
			paramValues[p.OriginalName] = v
		}
	}

	if err := graphql.ExecuteGraphQL(cmd, g.GraphQL, schema, authHeaders, paramValues, g.Fields, nil, outOpts(g), nil); err != nil {
		return err
	}

	usageName := cmd.GraphQLFieldName
	if usageName == "" {
		usageName = cmd.Name
	}
	cache.RecordUsage(srcHash, usageName)
	return nil
}

// ---------------------------------------------------------------------------
// MCP mode
// ---------------------------------------------------------------------------

func handleMCP(g *GlobalFlags, authHeaders [][2]string, envVars map[string]string, remaining []string, resourceAction, promptAction string, baked *bake.Config) error {
	source := g.MCP
	isStdio := false
	if g.MCPStdio != "" {
		source = g.MCPStdio
		isStdio = true
	}

	configForCache := map[string]any{
		"source":       source,
		"auth_headers": authHeaders,
		"transport":    g.Transport,
		"env_vars":     envVars,
		"is_stdio":     isStdio,
	}
	key := g.CacheKey
	if key == "" {
		key = cache.CacheKeyFor(configForCache)
	}
	srcHash := cache.SourceHashFor(source)
	ctx := context.Background()

	// Resource/prompt actions: connect once and handle.
	if resourceAction != "" || promptAction != "" {
		client, err := mcp.Connect(ctx, source, isStdio, authHeaders, envVars, g.Transport)
		if err != nil {
			return err
		}
		defer client.Close()

		oo := outOpts(g)

		switch resourceAction {
		case "list":
			data, _ := client.ListResources(ctx)
			util.OutputResult(data, oo)
		case "templates":
			data, _ := client.ListResourceTemplates(ctx)
			util.OutputResult(data, oo)
		case "read":
			txt, _ := client.ReadResource(ctx, g.ReadResource)
			util.OutputResult(txt, oo)
		}

		switch promptAction {
		case "list":
			data, _ := client.ListPrompts(ctx)
			util.OutputResult(data, oo)
		case "get":
			pargs := make(map[string]any)
			for _, pa := range g.PromptArg {
				if idx := strings.Index(pa, "="); idx >= 0 {
					pargs[pa[:idx]] = pa[idx+1:]
				}
			}
			data, _ := client.GetPrompt(ctx, g.GetPrompt, pargs)
			util.OutputResult(data, oo)
		}
		return nil
	}

	// LIST mode
	if g.ListCommands {
		tools, err := mcp.FetchToolsCached(ctx, key, g.CacheTTL, g.Refresh, source, isStdio, authHeaders, envVars, g.Transport)
		if err != nil {
			return err
		}
		commands := mcp.ExtractCommands(tools)
		if baked != nil {
			commands = bake.FilterCommands(commands, baked.Include, baked.Exclude, baked.Methods)
		}
		if g.SearchPattern != "" {
			commands = FilterCommandsSearch(commands, g.SearchPattern)
			if len(commands) == 0 {
				if !g.JSONOutput {
					fmt.Fprintln(util.Out, "No tools matching '"+g.SearchPattern+"'.")
				}
				ListMCP(commands, listOpts(g, srcHash))
				return nil
			}
			if !g.Compact && !g.JSONOutput {
				fmt.Fprintln(util.Out, "Tools matching '"+g.SearchPattern+"':")
			}
		}
		ListMCP(commands, listOpts(g, srcHash))
		return nil
	}

	// Need tool list for command lookup (try cache first).
	tools, err := mcp.FetchToolsCached(ctx, key, g.CacheTTL, g.Refresh, source, isStdio, authHeaders, envVars, g.Transport)
	if err != nil {
		return err
	}
	commands := mcp.ExtractCommands(tools)
	if baked != nil {
		commands = bake.FilterCommands(commands, baked.Include, baked.Exclude, baked.Methods)
	}

	// No subcommand given: show available tools.
	if len(remaining) == 0 {
		if !g.Compact && !g.JSONOutput {
			fmt.Fprintln(util.Out, "Available tools:")
		}
		ListMCP(commands, listOpts(g, srcHash))
		if !g.Compact && !g.JSONOutput {
			fmt.Fprintln(util.Out, "\nUse --list for the same output, or provide a subcommand.")
		}
		return nil
	}

	// Find and execute the MCP tool call.
	cmd, ok := findCommand(commands, remaining[0])
	if !ok {
		return fmt.Errorf("unknown command: %s (see --list)", remaining[0])
	}

	toolName := cmd.ToolName
	if toolName == "" {
		toolName = cmd.Name
	}

	arguments := make(map[string]any)

	// Parse tool arguments. ParseCommandArgs handles --stdin when cmd.HasBody.
	values, hasStdin, perr := ParseCommandArgs(cmd, remaining[1:])
	if perr != nil {
		return perr
	}

	if hasStdin {
		stdinData, serr := util.ReadStdinJSON("MCP tool arguments")
		if serr != nil {
			return serr
		}
		if m, ok := stdinData.(map[string]any); ok {
			arguments = m
		}
	} else {
		for _, p := range cmd.Params {
			if v, ok := values[p.Name]; ok {
				arguments[p.OriginalName] = v
			}
		}
	}

	// Connect and call the tool.
	client, err := mcp.Connect(ctx, source, isStdio, authHeaders, envVars, g.Transport)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.CallTool(ctx, toolName, arguments)
	if err != nil {
		return err
	}

	if g.JSONOutput {
		envelope := map[string]any{
			"content":           result.Content,
			"structuredContent": result.StructuredContent,
			"isError":           result.IsError,
		}
		util.OutputResult(envelope, util.OutputOptions{
			JSONOutput: true,
			Pretty:     g.Pretty,
			Head:       headPtr(g.Head),
		})
	} else {
		text := result.Text()
		if result.IsError {
			fmt.Fprintln(util.Err, text)
			return errSilent
		}
		oo := outOpts(g)
		util.OutputResult(text, oo)
	}

	cache.RecordUsage(srcHash, toolName) // ignore error
	return nil
}
