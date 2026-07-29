# mcp2cli — Go migration

Source (Python, reference): /tmp/mcp2cli-py  (single file src/mcp2cli/__init__.py, ~4316 lines)
Original repo: https://github.com/knowsuchagency/mcp2cli  (v3.3.1, MIT)

## Goal
Faithful Go port, restructured as a modular project. Single binary `mcp2cli`.

## Stack decisions
- MCP client: github.com/mark3labs/mcp-go (stdio + SSE + streamable HTTP)
- Dynamic subcommands/flags/help: hand-rolled two-stage parsing with stdlib `flag`
  (global FlagSet up to subcommand boundary via SplitAtSubcommand, then a per-command
  FlagSet). This mirrors Python argparse semantics and avoids global/tool flag collisions
  (e.g. --env). cobra deliberately NOT used (persistent-flag inheritance would re-introduce
  the collision Python works around in _split_at_subcommand).
- YAML specs: gopkg.in/yaml.v3
- HTTP: net/http (stdlib)
- No other deps unless justified.

## Package layout (internal/)
- types      ParamDef, CommandDef, BakeConfig
- util       resolve_secret(env:/file:), parse_kv_list, to_kebab, schema_type_to_python,
             coerce_value, output_result(json/raw/toon/head/pretty), command serialization,
             escape help, read_stdin_json
- cache      cache_key_for(sha256[:16]), load_cached/save_cache(TTL by mtime),
             usage tracking (record_usage, sort_commands usage/recent/alpha/default),
             source_hash
- openapi    resolve_refs, load_openapi_spec(url/file json/yaml), extract_openapi_commands,
             execute_openapi (path/query/header/body/multipart/file)
- graphql    introspection query, type unwrap, extract_graphql_commands, build_selection_set,
             build_graphql_document, execute_graphql
- mcp        mcp-go client (stdio/http auto/sse/streamable), extract_mcp_commands,
             call_tool, resources (list/templates/read), prompts (list/get)
- oauth      FileTokenStorage, client_credentials + authorization_code(PKCE+DCR). SCOPE: basic
             faithful port; the ultra-robust edge cases (cross-process expiry sidecar, stale-DCR
             recovery issue #50/54/57/58/59) are stretch goals.
- bake       filter_commands(include/exclude/methods globs), baked CRUD (create/list/show/
             remove/update/install), baked_to_argv, run_baked
- session    persistent daemon over unix domain socket + client (list/stop/start/request)
- cli        dynamic cobra builder from []CommandDef, split_at_subcommand, global flag set,
             main dispatch (_main_impl equivalent)

## Phases
1. scaffold + types + util + cache/usage  (tests ported from tests/test_helpers.py, test_cache.py, test_usage.py)
2. openapi  (tests/test_openapi.py)
3. graphql  (tests/test_graphql.py)
4. mcp      (tests/test_mcp.py)
5. bake     (tests/test_bake.py)
6. cli dispatch + output integration (tests/test_json.py, test_token_savings.py)
7. sessions
8. oauth    (tests/test_oauth.py — stretch)

## Behavior notes (must preserve)
- cache key = sha256(json of config minus cache_ttl/description/include/exclude/methods,
  auth_headers sorted)[:16]; cache files in $MCP2CLI_CACHE_DIR or ~/.cache/mcp2cli; TTL by mtime age.
- usage.json in cache dir; source_hash = sha256(source)[:16]; sort usage/recent/alpha/default.
- output_result precedence: --json > --raw > --toon > default; --json unwraps JSON strings,
  applies head, always valid JSON. pretty when --pretty OR stdout is TTY.
- coerce_value: array (json or comma-split), object (json), boolean, integer, number, schema-less
  fallback parses {/[ strings.
- to_kebab: camelCase boundary insert -, replace _ with -, lowercase.
- OpenAPI name from operationId kebab, else method-slug; dedupe by appending -method.
- content-type negotiation: multipart if any binary prop; else json.
- MCP transport auto = try streamable then sse fallback.
- bake name regex ^[a-z][a-z0-9-]*$; baked.json in $MCP2CLI_CONFIG_DIR or ~/.config/mcp2cli.
- _split_at_subcommand: global value-options consume next token; first positional = subcommand.
