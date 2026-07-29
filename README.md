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
  cli                global flags, argv splitting, dynamic per-command parser, dispatch
```

## Status

Implemented & tested: OpenAPI, MCP (stdio + HTTP), GraphQL, baking, listing/search/sort,
output formats, caching, usage tracking.

Not yet ported (return a clear error):
- **OAuth** (`--oauth*`): authorization-code + PKCE + DCR and client-credentials flows.
- **Sessions** (`--session*`): persistent daemon over a Unix socket.

## License

MIT (same as the original project).
