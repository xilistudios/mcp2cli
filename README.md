# mcp2cli (Go)

Turn any **MCP server**, **OpenAPI spec**, or **GraphQL endpoint** into a fully-typed CLI.

This is a Go port of [knowsuchagency/mcp2cli](https://github.com/knowsuchagency/mcp2cli) (Python),
restructured as a modular Go project and compiled to a single static binary.

```
mcp2cli --spec openapi.json --list                 # list operations
mcp2cli --spec openapi.json get-user --id 42       # call an operation
mcp2cli --mcp-stdio "my-mcp-server" --list         # list MCP tools
mcp2cli --mcp-stdio "my-mcp-server" search --q hi  # call a tool
mcp2cli --graphql https://api/graphql --list       # list queries/mutations
mcp2cli --graphql https://api/graphql user --id 7  # run a query (auto selection set)
```

## Features

- **OpenAPI** (`--spec`): JSON or YAML, local file or URL, `$ref` resolution, path/query/header/body
  params, multipart file uploads, content-type negotiation.
- **MCP** (`--mcp` HTTP / `--mcp-stdio`): streamable-HTTP + SSE (auto fallback) and stdio transports,
  tools, resources (`--list-resources`, `--read-resource`, `--list-resource-templates`) and
  prompts (`--list-prompts`, `--get-prompt`). Built on [mcp-go](https://github.com/mark3labs/mcp-go).
- **GraphQL** (`--graphql`): introspection-driven command generation, automatic selection sets,
  enum choices, `--fields` override.
- **Baking** (`mcp2cli bake …` / `@name`): save connection settings as named tools, install
  `~/.local/bin` wrappers, include/exclude/method filtering.
- **Output control**: `--json` (always-valid JSON), `--raw`, `--pretty`, `--head N`, `--toon`
  (token-efficient encoding via `@toon-format/cli`), `--compact` and `--top N` listings.
- **Caching & usage**: spec/schema/tool caching with `--cache-ttl` / `--refresh`; usage tracking
  with `--sort usage|recent|alpha|default`.
- **Secrets**: `--auth-header 'Name:env:VAR'` or `file:/path` resolution.

## Install

### Quick install (recommended)

```sh
curl -fsSL https://raw.githubusercontent.com/xilistudios/mcp2cli/main/install.sh | sh
```

Options:

```sh
# Install a specific version
curl -fsSL https://raw.githubusercontent.com/xilistudios/mcp2cli/main/install.sh | sh -s -- --version v1.0.0

# Install to a custom prefix (default: ~/.local)
curl -fsSL https://raw.githubusercontent.com/xilistudios/mcp2cli/main/install.sh | sh -s -- --prefix /usr/local
```

The script auto-detects your OS and architecture, downloads the correct binary from GitHub Releases, verifies the checksum, and places it in `<prefix>/bin/mcp2cli`.

### From source

```
go build -o mcp2cli ./cmd/mcp2cli
# or
go install github.com/xilistudios/mcp2cli/cmd/mcp2cli@latest
```

## Usage

Run `mcp2cli --help`. Exactly one source mode is required: `--spec`, `--mcp`, `--mcp-stdio`,
or `--graphql`. Add `--list` to discover generated subcommands, then call them with `--<flag> value`.

### Baking

```
mcp2cli bake create gh --spec https://api.github.com/openapi.json --cache-ttl 600
mcp2cli bake list
gh --list            # after: mcp2cli bake install gh  (creates ~/.local/bin/gh)
mcp2cli @gh --list   # or invoke directly
```

### Sessions

Keep one MCP connection alive across many calls (avoids reconnecting each time):

```
mcp2cli --mcp https://api.example.com/mcp --session-start work
mcp2cli --session-list
mcp2cli --session work search --q "quarterly report"   # reuses the live connection
mcp2cli --session-stop work
```

### OAuth

```
# client credentials (server-to-server)
mcp2cli --mcp https://api.example.com/mcp --oauth \
  --oauth-client-id "$ID" --oauth-client-secret "$SECRET" --list

# authorization code + PKCE (opens a browser; DCR if no client id)
mcp2cli --mcp https://api.example.com/mcp --oauth \
  --oauth-redirect-uri http://localhost:3334/oauth/callback --list
```

Tokens, refresh tokens and dynamic-client-registration credentials (client id **and**
secret) are persisted in the **OS keyring** (service `mcp2cli-oauth`), so they survive
reboots and aggressive cache cleaners. When no keyring is available (headless servers,
containers), mcp2cli falls back to files under `~/.config/mcp2cli/oauth/<hash>/` with
`0600` permissions. Old credentials left in `~/.cache/mcp2cli/oauth/` are picked up and
migrated automatically on first use.

```bash
# where are my credentials kept, are they expired, which backend is active?
mcp2cli --mcp https://api.example.com/mcp --oauth-status

# log out: delete every stored token/client credential for this server
mcp2cli --mcp https://api.example.com/mcp --oauth-reset
```

Force a backend with `MCP2CLI_TOKEN_STORE=keyring` (fail if no keyring is available —
never writes plaintext) or `MCP2CLI_TOKEN_STORE=file` (skip the keyring entirely).
Expired access tokens are refreshed silently with the stored refresh token; if renewal
is rejected, the next run falls back to a fresh interactive login.

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

## Status

Implemented & tested: OpenAPI, MCP (stdio + HTTP), GraphQL, baking, listing/search/sort,
output formats, caching, usage tracking, **persistent sessions** and **OAuth**.

- **Sessions** (`--session-start/--session-list/--session-stop/--session`): a background daemon
  holds one long-lived MCP connection and serves requests over a Unix domain socket, so repeated
  calls reuse a single connection (no reconnect/handshake per invocation).
- **OAuth** (`--oauth*`): authorization-code + PKCE (with dynamic client registration when no
  client id is given) and client-credentials flows. Credentials (tokens, refresh tokens, client
  id/secret) live in the OS keyring with a durable file fallback under the config dir, silent
  refresh on expiry, and `--oauth-status` / `--oauth-reset` for management. authorization-code
  opens a browser + local callback server; client-credentials discovers the token endpoint
  (RFC 9728 / well-known), caches the grant until near expiry, and injects a bearer token.

The interactive authorization-code browser flow is not covered by automated tests (it requires a
real browser + OAuth server); the token store (keyring + file fallback, migration, status,
reset), callback server, discovery, client-credentials grant and connection wiring are all
unit/integration tested against an embedded OAuth server.

## License

MIT (same as the original project).
