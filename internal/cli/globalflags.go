package cli

import (
	"flag"
	"io"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/cache"
)

// GlobalFlags holds all global CLI flags for mcp2cli.
type GlobalFlags struct {
	Spec, MCP, MCPStdio, GraphQL string
	AuthHeaders                  []string
	BaseURL, CacheKey            string
	CacheTTL                     int
	Refresh, ListCommands        bool
	SearchPattern                string
	Verbose                      bool
	SortMode                     string
	Top                          int
	Compact, Pretty              bool
	Raw, JSONOutput, Toon        bool
	Head                         int
	Fields                       string
	Transport                    string
	Env                          []string
	OAuth                        bool
	OAuthClientID                string
	OAuthClientSecret            string
	OAuthClientName              string
	OAuthScope                   string
	OAuthRedirectURI             string
	OAuthFlow                    string
	ListResources                bool
	ListResourceTemplates        bool
	ReadResource                 string
	ListPrompts                  bool
	GetPrompt                    string
	PromptArg                    []string
	SessionStart                 string
	SessionStop                  string
	SessionList                  bool
	Session                      string
	ShowVersion                  bool
}

// stringSliceValue implements flag.Value for repeatable string flags.
type stringSliceValue struct {
	vals *[]string
}

func (s *stringSliceValue) String() string {
	if s.vals == nil {
		return ""
	}
	return strings.Join(*s.vals, ",")
}

func (s *stringSliceValue) Set(v string) error {
	*s.vals = append(*s.vals, v)
	return nil
}

// NewGlobalFlagSet builds a flag.FlagSet binding to g, and returns the
// sets of value-option names and bool-option names (for SplitAtSubcommand).
func NewGlobalFlagSet(g *GlobalFlags) (fs *flag.FlagSet, valueOpts map[string]bool, boolOpts map[string]bool) {
	fs = flag.NewFlagSet("mcp2cli", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	// Defaults
	g.CacheTTL = cache.DefaultCacheTTL
	g.OAuthClientName = "mcp2cli"
	g.OAuthFlow = "auto"
	g.Transport = "auto"

	valueOpts = make(map[string]bool)
	boolOpts = make(map[string]bool)

	// Helper to register value flags.
	addValue := func(name string, p *string, def, usage string) {
		fs.StringVar(p, name, def, usage)
		valueOpts["--"+name] = true
	}
	addValueInt := func(name string, p *int, def int, usage string) {
		fs.IntVar(p, name, def, usage)
		valueOpts["--"+name] = true
	}
	addValueRepeatable := func(name string, vals *[]string) {
		fs.Var(&stringSliceValue{vals: vals}, name, "(repeatable)")
		valueOpts["--"+name] = true
	}
	addBool := func(name string, p *bool, def bool, usage string) {
		fs.BoolVar(p, name, def, usage)
		boolOpts["--"+name] = true
	}

	// Value options
	addValue("spec", &g.Spec, "", "Path or URL to OpenAPI spec")
	addValue("mcp", &g.MCP, "", "MCP server URL")
	addValue("mcp-stdio", &g.MCPStdio, "", "MCP stdio command")
	addValue("graphql", &g.GraphQL, "", "GraphQL endpoint URL")
	addValueRepeatable("auth-header", &g.AuthHeaders)
	addValue("base-url", &g.BaseURL, "", "Base URL override")
	addValue("cache-key", &g.CacheKey, "", "Cache key override")
	addValueInt("cache-ttl", &g.CacheTTL, g.CacheTTL, "Cache TTL in seconds")
	addValue("search", &g.SearchPattern, "", "Search/filter commands by pattern")
	addValue("sort", &g.SortMode, "", "Sort mode (alpha, usage, recent)")
	addValueInt("top", &g.Top, 0, "Show top N commands")
	addValue("fields", &g.Fields, "", "Comma-separated fields to include")
	addValue("transport", &g.Transport, g.Transport, "Transport type (auto, http, stdio)")
	addValueRepeatable("env", &g.Env)
	addValue("oauth-client-id", &g.OAuthClientID, "", "OAuth client ID")
	addValue("oauth-client-secret", &g.OAuthClientSecret, "", "OAuth client secret")
	addValue("oauth-client-name", &g.OAuthClientName, g.OAuthClientName, "OAuth client name")
	addValue("oauth-scope", &g.OAuthScope, "", "OAuth scope")
	addValue("oauth-redirect-uri", &g.OAuthRedirectURI, "", "OAuth redirect URI")
	addValue("oauth-flow", &g.OAuthFlow, g.OAuthFlow, "OAuth flow (auto, device, pkce)")
	addValue("read-resource", &g.ReadResource, "", "Read MCP resource by URI")
	addValue("get-prompt", &g.GetPrompt, "", "Get MCP prompt by name")
	addValueRepeatable("prompt-arg", &g.PromptArg)
	addValue("session-start", &g.SessionStart, "", "Start a session with name")
	addValue("session-stop", &g.SessionStop, "", "Stop a session by name")
	addValue("session", &g.Session, "", "Use a specific session")
	addValueInt("head", &g.Head, 0, "Show first N results")

	// Bool options
	addBool("refresh", &g.Refresh, false, "Force refresh cache")
	addBool("list", &g.ListCommands, false, "List available commands")
	addBool("verbose", &g.Verbose, false, "Verbose output")
	addBool("compact", &g.Compact, false, "Compact output")
	addBool("pretty", &g.Pretty, false, "Pretty-print JSON output")
	addBool("raw", &g.Raw, false, "Raw output (no formatting)")
	addBool("json", &g.JSONOutput, false, "Output as JSON")
	addBool("toon", &g.Toon, false, "Output in TOON format")
	addBool("oauth", &g.OAuth, false, "Enable OAuth authentication")
	addBool("list-resources", &g.ListResources, false, "List MCP resources")
	addBool("list-resource-templates", &g.ListResourceTemplates, false, "List MCP resource templates")
	addBool("list-prompts", &g.ListPrompts, false, "List MCP prompts")
	addBool("session-list", &g.SessionList, false, "List active sessions")
	addBool("version", &g.ShowVersion, false, "Show version")

	return fs, valueOpts, boolOpts
}
