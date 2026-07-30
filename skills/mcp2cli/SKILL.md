---
name: mcp2cli
description: Turn any MCP server, OpenAPI spec, or GraphQL endpoint into a CLI. Use this skill when the user wants to interact with an MCP server, OpenAPI/REST API, or GraphQL API via command line, discover available tools/endpoints, call API operations, manage baked tools, use persistent sessions, or authenticate via OAuth. Triggers include "mcp2cli", "call this MCP server", "use this API", "list tools from", "bake", "session", "graphql", or any task involving MCP tool invocation, OpenAPI endpoint calls, or GraphQL queries without writing code.
---

# mcp2cli (Go)

Turn any MCP server, OpenAPI spec, or GraphQL endpoint into a fully-typed CLI at runtime. Single static Go binary. No codegen.

Go port of [knowsuchagency/mcp2cli](https://github.com/knowsuchagency/mcp2cli) (Python), restructured as a modular project.

## Install

```bash
# One-line installer (downloads prebuilt binary)
curl -fsSL https://raw.githubusercontent.com/xilistudios/mcp2cli/main/install.sh | sh

# Or install with Go
go install github.com/xilistudios/mcp2cli/cmd/mcp2cli@latest

# Or build from source
git clone https://github.com/xilistudios/mcp2cli && cd mcp2cli
go build -o mcp2cli ./cmd/mcp2cli
```

## Core Workflow

1. **Connect** to a source (MCP server, OpenAPI spec, or GraphQL endpoint)
2. **Discover** available commands with `--list` (or filter with `--search`)
3. **Inspect** a specific command with `<command> --help`
4. **Execute** the command with flags

```bash
# OpenAPI spec (remote or local, JSON or YAML)
mcp2cli --spec https://petstore3.swagger.io/api/v3/openapi.json --list
mcp2cli --spec ./openapi.json --base-url https://api.example.com list-pets --status available

# MCP over HTTP (streamable-HTTP + SSE auto fallback)
mcp2cli --mcp https://mcp.example.com/sse --list
mcp2cli --mcp https://mcp.example.com/sse create-task --title "Fix bug"

# MCP over stdio
mcp2cli --mcp-stdio "npx @modelcontextprotocol/server-filesystem /tmp" --list
mcp2cli --mcp-stdio "npx @modelcontextprotocol/server-filesystem /tmp" read-file --path /tmp/hello.txt

# GraphQL endpoint
mcp2cli --graphql https://api.example.com/graphql --list
mcp2cli --graphql https://api.example.com/graphql users --limit 10
mcp2cli --graphql https://api.example.com/graphql create-user --name "Alice"
```

## CLI Reference

```
mcp2cli [global options] <subcommand> [command options]
mcp2cli <subcommand> --help        # inspect a specific command
mcp2cli bake <action> [options]    # manage baked tools
```

Exactly one source mode is required: `--spec`, `--mcp`, `--mcp-stdio`, or `--graphql`.

### Global Options

| Flag | Description |
|------|-------------|
| `--spec PATH_OR_URL` | OpenAPI spec (JSON/YAML, local or remote) |
| `--mcp URL` | MCP server URL (streamable-HTTP + SSE auto fallback) |
| `--mcp-stdio CMD` | MCP stdio command (e.g. `"npx @mcp/server"`) |
| `--graphql URL` | GraphQL endpoint URL |
| `--base-url URL` | Base URL override for OpenAPI specs |
| `--auth-header 'Name:env:VAR'` | Auth header (repeatable). Supports `env:VAR` or `file:/path` |
| `--list` | List available commands |
| `--search PATTERN` | Filter commands by pattern |
| `--sort MODE` | Sort: `alpha`, `usage`, `recent`, `default` |
| `--top N` | Show top N commands |
| `--compact` | Compact listing output |
| `--fields FIELDS` | Comma-separated fields to include (GraphQL) |
| `--transport TYPE` | Transport: `auto`, `http`, `stdio` |
| `--env KEY=VAL` | Environment variables for stdio (repeatable) |
| `--json` | Always-valid JSON output |
| `--raw` | Raw output (no formatting) |
| `--pretty` | Pretty-print JSON |
| `--toon` | Token-efficient TOON encoding |
| `--head N` | Show first N results |
| `--cache-ttl SECONDS` | Cache TTL (default 300) |
| `--cache-key KEY` | Cache key override |
| `--refresh` | Force refresh cache |
| `--verbose` | Verbose output |
| `--version` | Show version |

### MCP Resources & Prompts

```bash
mcp2cli --mcp URL --list-resources              # list available resources
mcp2cli --mcp URL --list-resource-templates     # list resource templates
mcp2cli --mcp URL --read-resource "uri://..."   # read a specific resource
mcp2cli --mcp URL --list-prompts                # list available prompts
mcp2cli --mcp URL --get-prompt "name" --prompt-arg "key=value"  # get a prompt
```

### Sessions (Persistent MCP Connections)

Keep one MCP connection alive across many calls (avoids reconnecting each time):

```bash
mcp2cli --mcp https://api.example.com/mcp --session-start work
mcp2cli --session-list
mcp2cli --session work search --q "quarterly report"   # reuses live connection
mcp2cli --session-stop work
```

A background daemon holds a long-lived MCP connection and serves requests over a Unix domain socket.

### OAuth

```bash
# Client credentials (server-to-server)
mcp2cli --mcp https://api.example.com/mcp --oauth \
  --oauth-client-id "$ID" --oauth-client-secret "$SECRET" --list

# Authorization code + PKCE (opens browser; DCR if no client id)
mcp2cli --mcp https://api.example.com/mcp --oauth \
  --oauth-redirect-uri http://localhost:3334/oauth/callback --list
```

| Flag | Description |
|------|-------------|
| `--oauth` | Enable OAuth authentication |
| `--oauth-client-id ID` | OAuth client ID |
| `--oauth-client-secret SECRET` | OAuth client secret |
| `--oauth-client-name NAME` | Client name for DCR (default: mcp2cli) |
| `--oauth-scope SCOPE` | OAuth scope |
| `--oauth-redirect-uri URI` | Redirect URI for auth code flow |
| `--oauth-flow FLOW` | Flow: `auto`, `device`, `pkce` |

### Baking (Named Tools)

Save connection settings as named tools for quick reuse:

```bash
# Create a baked tool
mcp2cli bake create gh --spec https://api.github.com/openapi.json --cache-ttl 600

# List baked tools
mcp2cli bake list

# Show details
mcp2cli bake show gh

# Install wrapper to ~/.local/bin
mcp2cli bake install gh
gh --list            # use the wrapper directly

# Or invoke via @ syntax
mcp2cli @gh --list

# Update / remove
mcp2cli bake update gh --cache-ttl 1200
mcp2cli bake remove gh
```

Bake supports `--include`, `--exclude`, and `--methods` filtering to limit which commands are exposed.

## Output Formats

Precedence: `--json` > `--raw` > `--toon` > default.

- `--json`: Always-valid JSON. Unwraps JSON strings, applies `--head`.
- `--raw`: Raw content, no formatting.
- `--toon`: Token-efficient encoding (via `@toon-format/cli`).
- `--pretty`: Pretty-print (auto-enabled when stdout is TTY).
- `--head N`: Limit results to first N items.

## Caching & Usage

- Spec/schema/tool responses are cached with configurable TTL (`--cache-ttl`).
- Cache location: `$MCP2CLI_CACHE_DIR` or `~/.cache/mcp2cli`.
- Usage tracking: commands sorted by `usage`, `recent`, `alpha`, or `default`.
- Force refresh with `--refresh`.

## Secrets

Auth headers support secret resolution:

```bash
# From environment variable
--auth-header 'Authorization:env:MY_TOKEN'

# From file
--auth-header 'Authorization:file:/path/to/token'
```

## Architecture

```
cmd/mcp2cli          entrypoint
internal/
  types              ParamDef, CommandDef, BakeConfig
  util               secrets, kv parsing, kebab, value coercion, output formatting, TOON
  cache              cache keys, TTL cache, usage tracking + sorting
  openapi            $ref resolution, spec loading, command extraction, execution
  graphql            introspection, command extraction, selection sets, execution
  mcp                mcp-go client wrapper (stdio/SSE/streamable), tools/resources/prompts
  bake               command filtering, baked-config CRUD, wrapper install
  session            persistent MCP daemon (unix socket) + client routing
  oauth              token store, PKCE callback server, client_credentials, endpoint discovery
  cli                global flags, argv splitting, dynamic per-command parser, dispatch
```

## Key Behaviors

- OpenAPI command names derived from `operationId` (kebab-case), deduped by appending method.
- MCP transport `auto` tries streamable-HTTP first, falls back to SSE.
- Content-type negotiation: multipart if any binary property, else JSON.
- GraphQL: introspection-driven command generation with automatic selection sets.
- Bake name must match `^[a-z][a-z0-9-]*$`.
- Baked config stored in `$MCP2CLI_CONFIG_DIR` or `~/.config/mcp2cli`.

## Examples

```bash
# Discover and call a GitHub API operation
mcp2cli --spec https://api.github.com/openapi.json --search "repos" --list
mcp2cli --spec https://api.github.com/openapi.json list-repos --per-page 5 --json

# Use a local MCP server with environment
mcp2cli --mcp-stdio "node server.js" --env API_KEY=secret --list

# GraphQL with field selection
mcp2cli --graphql https://api.example.com/graphql users --limit 10 --fields "id,name,email"

# Bake + session workflow
mcp2cli bake create myapi --mcp https://api.example.com/mcp --cache-ttl 600
mcp2cli @myapi --session-start work
mcp2cli --session work search --q "docs"
mcp2cli --session-stop work

# Token-efficient output for LLM consumption
mcp2cli --spec ./openapi.json get-user --id 42 --toon
```

## Links

- Repository: https://github.com/xilistudios/mcp2cli
- Original (Python): https://github.com/knowsuchagency/mcp2cli
- License: MIT
