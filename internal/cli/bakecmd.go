// bakecmd.go implements the "bake" subcommand dispatcher.
// It parses bake-subcommand arguments and delegates to the existing
// bake package CRUD functions (Create, Load/Show, List, Remove, Update, Install).
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/bake"
	"github.com/xilistudios/mcp2cli/internal/cache"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// runBake dispatches the "mcp2cli bake <subcommand>" family.
func runBake(argv []string) error {
	// Help when no subcommand given.
	if len(argv) == 0 || argv[0] == "-h" || argv[0] == "--help" {
		fmt.Fprintln(util.Out, "Usage: mcp2cli bake <command> [options]")
		fmt.Fprintln(util.Out, "")
		fmt.Fprintln(util.Out, "Commands:")
		fmt.Fprintln(util.Out, "  create    Save connection settings as a named baked tool")
		fmt.Fprintln(util.Out, "  list      List all baked tools")
		fmt.Fprintln(util.Out, "  show      Show config for a baked tool (secrets masked)")
		fmt.Fprintln(util.Out, "  remove    Delete a baked tool")
		fmt.Fprintln(util.Out, "  update    Update settings on an existing baked tool")
		fmt.Fprintln(util.Out, "  install   Create a ~/.local/bin wrapper script")
		fmt.Fprintln(util.Out, "")
		fmt.Fprintln(util.Out, "Run 'mcp2cli bake <command> --help' for command-specific help.")
		if len(argv) == 0 {
			return fmt.Errorf("no bake subcommand")
		}
		return nil
	}

	action := argv[0]
	rest := argv[1:]

	switch action {
	case "create":
		return bakeCreate(rest)
	case "list":
		return bakeList(rest)
	case "show":
		return bakeShow(rest)
	case "remove":
		return bakeRemove(rest)
	case "update":
		return bakeUpdate(rest)
	case "install":
		return bakeInstall(rest)
	default:
		fmt.Fprintf(util.Err, "Unknown bake subcommand: %s\n", action)
		return fmt.Errorf("unknown bake subcommand: %s", action)
	}
}

// ---------------------------------------------------------------------------
// bake create
// ---------------------------------------------------------------------------

func bakeCreate(argv []string) error {
	fs := flag.NewFlagSet("bake create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		name, spec, mcpCmd, mcpStdio       string
		baseURL, transport, description     string
		oauthClientID, oauthClientSecret    string
		oauthClientName, oauthScope         string
		oauthRedirectURI, oauthFlow         string
		include, exclude, methods           string
		cacheTTL                            int
		useOAuth, force                     bool
		authHeaders, envs                   []string
	)

	fs.StringVar(&spec, "spec", "", "")
	fs.StringVar(&mcpCmd, "mcp", "", "")
	fs.StringVar(&mcpStdio, "mcp-stdio", "", "")
	fs.StringVar(&baseURL, "base-url", "", "")
	fs.StringVar(&transport, "transport", "auto", "")
	fs.StringVar(&description, "description", "", "")
	fs.StringVar(&include, "include", "", "")
	fs.StringVar(&exclude, "exclude", "", "")
	fs.StringVar(&methods, "methods", "", "")
	fs.IntVar(&cacheTTL, "cache-ttl", cache.DefaultCacheTTL, "")
	fs.BoolVar(&useOAuth, "oauth", false, "")
	fs.StringVar(&oauthClientID, "oauth-client-id", "", "")
	fs.StringVar(&oauthClientSecret, "oauth-client-secret", "", "")
	fs.StringVar(&oauthClientName, "oauth-client-name", "mcp2cli", "")
	fs.StringVar(&oauthScope, "oauth-scope", "", "")
	fs.StringVar(&oauthRedirectURI, "oauth-redirect-uri", "", "")
	fs.StringVar(&oauthFlow, "oauth-flow", "auto", "")
	fs.BoolVar(&force, "force", false, "")

	fs.Var(&stringSliceValue{vals: &authHeaders}, "auth-header", "")
	fs.Var(&stringSliceValue{vals: &envs}, "env", "")

	// Handle --help before parsing.
	for _, a := range argv {
		if a == "-h" || a == "--help" {
			fmt.Fprintln(util.Out, "Usage: mcp2cli bake create <name> [options]")
			fmt.Fprintln(util.Out, "")
			fmt.Fprintln(util.Out, "Save connection settings as a named baked tool.")
			fmt.Fprintln(util.Out, "")
			fmt.Fprintln(util.Out, "Options:")
			fmt.Fprintln(util.Out, "  --spec URL            OpenAPI spec URL or file path")
			fmt.Fprintln(util.Out, "  --mcp URL             MCP server URL")
			fmt.Fprintln(util.Out, "  --mcp-stdio CMD       MCP stdio command")
			fmt.Fprintln(util.Out, "  --base-url URL        Base URL override")
			fmt.Fprintln(util.Out, "  --auth-header K:V     Auth header (repeatable)")
			fmt.Fprintln(util.Out, "  --env K=V             Env var (repeatable)")
			fmt.Fprintln(util.Out, "  --cache-ttl N         Cache TTL in seconds")
			fmt.Fprintln(util.Out, "  --transport TYPE      Transport (auto|sse|streamable)")
			fmt.Fprintln(util.Out, "  --include GLOBS       Comma-separated include globs")
			fmt.Fprintln(util.Out, "  --exclude GLOBS       Comma-separated exclude globs")
			fmt.Fprintln(util.Out, "  --methods METHODS     Comma-separated HTTP methods")
			fmt.Fprintln(util.Out, "  --description TEXT    Description")
			fmt.Fprintln(util.Out, "  --force               Overwrite existing")
			return nil
		}
	}

	// Go's flag.FlagSet stops at the first non-flag argument. The name
	// can appear either as a positional arg or after flags. Split it out
	// manually: first pass extracts any leading non-flag token as name,
	// then the rest goes to Parse.
	var flagArgs []string
	for i, a := range argv {
		if !strings.HasPrefix(a, "-") {
			// Positional: use as name.
			name = a
			flagArgs = append(argv[:i], argv[i+1:]...)
			break
		}
	}
	if flagArgs == nil {
		flagArgs = argv
	}

	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	// Allow --name as an alternative.
	if name == "" {
		name = "" // already set above if positional found
	}
	// Check remaining positional args (shouldn't happen but handle gracefully).
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		return fmt.Errorf("bake create requires a tool name")
	}

	parsedAuth, err := util.ParseKVList(authHeaders, ":", "auth header", true)
	if err != nil {
		return err
	}
	parsedEnv, err := util.ParseKVList(envs, "=", "env", false)
	if err != nil {
		return err
	}
	envVars := make(map[string]string, len(parsedEnv))
	for _, p := range parsedEnv {
		envVars[p[0]] = p[1]
	}

	// Determine source type and source.
	sourceType := ""
	source := ""
	if spec != "" {
		sourceType = "spec"
		source = spec
	} else if mcpCmd != "" {
		sourceType = "mcp"
		source = mcpCmd
	} else if mcpStdio != "" {
		sourceType = "mcp_stdio"
		source = mcpStdio
	}
	if sourceType == "" {
		return fmt.Errorf("one of --spec, --mcp, or --mcp-stdio is required")
	}

	splitCSV := func(s string) []string {
		if s == "" {
			return nil
		}
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}

	cfg := bake.Config{
		SourceType:        sourceType,
		Source:            source,
		BaseURL:           baseURL,
		AuthHeaders:       parsedAuth,
		EnvVars:           envVars,
		CacheTTL:          cacheTTL,
		Transport:         transport,
		OAuth:             useOAuth,
		OAuthClientID:     oauthClientID,
		OAuthClientSecret: oauthClientSecret,
		OAuthClientName:   oauthClientName,
		OAuthScope:        oauthScope,
		OAuthRedirectURI:  oauthRedirectURI,
		OAuthFlow:         oauthFlow,
		Include:           splitCSV(include),
		Exclude:           splitCSV(exclude),
		Methods:           splitCSV(methods),
		Description:       description,
	}

	if err := bake.Create(name, cfg, force); err != nil {
		fmt.Fprintln(util.Err, "Error:", err)
		return err
	}
	fmt.Fprintf(util.Out, "Baked tool %q created.\n", name)
	return nil
}

// ---------------------------------------------------------------------------
// bake list
// ---------------------------------------------------------------------------

func bakeList(argv []string) error {
	for _, a := range argv {
		if a == "-h" || a == "--help" {
			fmt.Fprintln(util.Out, "Usage: mcp2cli bake list")
			fmt.Fprintln(util.Out, "List all baked tools.")
			return nil
		}
	}
	return bake.List(util.Out)
}

// ---------------------------------------------------------------------------
// bake show
// ---------------------------------------------------------------------------

func bakeShow(argv []string) error {
	for _, a := range argv {
		if a == "-h" || a == "--help" {
			fmt.Fprintln(util.Out, "Usage: mcp2cli bake show <name>")
			fmt.Fprintln(util.Out, "Show config for a baked tool (secrets masked).")
			return nil
		}
	}
	if len(argv) == 0 {
		return fmt.Errorf("bake show requires a tool name")
	}
	name := argv[0]
	display, err := bake.Show(name)
	if err != nil {
		fmt.Fprintln(util.Err, "Error:", err)
		return err
	}
	util.OutputResult(display, util.OutputOptions{JSONOutput: true, Pretty: true})
	return nil
}

// ---------------------------------------------------------------------------
// bake remove
// ---------------------------------------------------------------------------

func bakeRemove(argv []string) error {
	for _, a := range argv {
		if a == "-h" || a == "--help" {
			fmt.Fprintln(util.Out, "Usage: mcp2cli bake remove <name>")
			fmt.Fprintln(util.Out, "Delete a baked tool.")
			return nil
		}
	}
	if len(argv) == 0 {
		return fmt.Errorf("bake remove requires a tool name")
	}
	name := argv[0]
	if err := bake.Remove(name); err != nil {
		fmt.Fprintln(util.Err, "Error:", err)
		return err
	}
	fmt.Fprintf(util.Out, "Baked tool %q removed.\n", name)
	return nil
}

// ---------------------------------------------------------------------------
// bake update
// ---------------------------------------------------------------------------

func bakeUpdate(argv []string) error {
	fs := flag.NewFlagSet("bake update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		cacheTTL             int
		include, exclude     string
		methods              string
		description, baseURL string
		transport            string
		hasCacheTTL          bool
	)

	fs.Var(&optionalInt{val: &cacheTTL, set: &hasCacheTTL}, "cache-ttl", "")
	fs.StringVar(&include, "include", "", "")
	fs.StringVar(&exclude, "exclude", "", "")
	fs.StringVar(&methods, "methods", "", "")
	fs.StringVar(&description, "description", "", "")
	fs.StringVar(&baseURL, "base-url", "", "")
	fs.StringVar(&transport, "transport", "", "")

	for _, a := range argv {
		if a == "-h" || a == "--help" {
			fmt.Fprintln(util.Out, "Usage: mcp2cli bake update <name> [options]")
			fmt.Fprintln(util.Out, "")
			fmt.Fprintln(util.Out, "Options:")
			fmt.Fprintln(util.Out, "  --cache-ttl N         Cache TTL")
			fmt.Fprintln(util.Out, "  --include GLOBS       Comma-separated include globs")
			fmt.Fprintln(util.Out, "  --exclude GLOBS       Comma-separated exclude globs")
			fmt.Fprintln(util.Out, "  --methods METHODS     Comma-separated HTTP methods")
			fmt.Fprintln(util.Out, "  --description TEXT    Description")
			fmt.Fprintln(util.Out, "  --base-url URL        Base URL")
			fmt.Fprintln(util.Out, "  --transport TYPE      Transport")
			return nil
		}
	}

	if err := fs.Parse(argv); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("bake update requires a tool name")
	}
	name := fs.Arg(0)

	// Track which flags were explicitly set.
	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })

	opts := bake.UpdateOptions{}
	if hasCacheTTL {
		opts.CacheTTL = &cacheTTL
	}
	if setFlags["include"] {
		v := splitCSV(include)
		opts.Include = &v
	}
	if setFlags["exclude"] {
		v := splitCSV(exclude)
		opts.Exclude = &v
	}
	if setFlags["methods"] {
		v := splitCSV(methods)
		opts.Methods = &v
	}
	if setFlags["description"] {
		opts.Description = &description
	}
	if setFlags["base-url"] {
		opts.BaseURL = &baseURL
	}
	if setFlags["transport"] {
		opts.Transport = &transport
	}

	if err := bake.Update(name, opts); err != nil {
		fmt.Fprintln(util.Err, "Error:", err)
		return err
	}
	fmt.Fprintf(util.Out, "Baked tool %q updated.\n", name)
	return nil
}

// splitCSV splits a comma-separated string, trimming whitespace.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// bake install
// ---------------------------------------------------------------------------

func bakeInstall(argv []string) error {
	fs := flag.NewFlagSet("bake install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var dir string
	fs.StringVar(&dir, "dir", "", "")

	for _, a := range argv {
		if a == "-h" || a == "--help" {
			fmt.Fprintln(util.Out, "Usage: mcp2cli bake install <name> [--dir DIR]")
			fmt.Fprintln(util.Out, "")
			fmt.Fprintln(util.Out, "Create a wrapper script in ~/.local/bin (or --dir).")
			return nil
		}
	}

	if err := fs.Parse(argv); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("bake install requires a tool name")
	}
	name := fs.Arg(0)

	wrapper, err := bake.Install(name, dir)
	if err != nil {
		fmt.Fprintln(util.Err, "Error:", err)
		return err
	}
	fmt.Fprintf(util.Out, "Installed wrapper: %s\n", wrapper)

	if dir == "" {
		home, herr := os.UserHomeDir()
		if herr == nil {
			binDir := home + "/.local/bin"
			path := os.Getenv("PATH")
			if !strings.Contains(path, binDir) {
				fmt.Fprintf(util.Out, "  Note: %s may not be in your PATH\n", binDir)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// optionalInt implements flag.Value for an int that tracks whether it was set.
type optionalInt struct {
	val *int
	set *bool
}

func (o *optionalInt) String() string {
	if o.val == nil {
		return "0"
	}
	return fmt.Sprintf("%d", *o.val)
}

func (o *optionalInt) Set(s string) error {
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return err
	}
	*o.val = v
	*o.set = true
	return nil
}
