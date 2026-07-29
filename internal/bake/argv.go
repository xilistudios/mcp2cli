package bake

import "strconv"

// BakedToArgv reconstructs CLI argv from a Config, mirroring the Python
// _baked_to_argv function. The returned slice is in the canonical order
// expected by the CLI parser.
func BakedToArgv(c Config) []string {
	var argv []string

	// Source type → flag.
	switch c.SourceType {
	case "spec":
		argv = append(argv, "--spec", c.Source)
	case "mcp":
		argv = append(argv, "--mcp", c.Source)
	case "mcp_stdio":
		argv = append(argv, "--mcp-stdio", c.Source)
	}

	if c.BaseURL != "" {
		argv = append(argv, "--base-url", c.BaseURL)
	}
	for _, hv := range c.AuthHeaders {
		argv = append(argv, "--auth-header", hv[0]+":"+hv[1])
	}
	for k, v := range c.EnvVars {
		argv = append(argv, "--env", k+"="+v)
	}
	if c.CacheTTL != 0 {
		argv = append(argv, "--cache-ttl", strconv.Itoa(c.CacheTTL))
	}
	if c.Transport != "" && c.Transport != "auto" {
		argv = append(argv, "--transport", c.Transport)
	}
	if c.OAuth {
		argv = append(argv, "--oauth")
	}
	if c.OAuthClientID != "" {
		argv = append(argv, "--oauth-client-id", c.OAuthClientID)
	}
	if c.OAuthClientSecret != "" {
		argv = append(argv, "--oauth-client-secret", c.OAuthClientSecret)
	}
	if c.OAuthClientName != "" && c.OAuthClientName != "mcp2cli" {
		argv = append(argv, "--oauth-client-name", c.OAuthClientName)
	}
	if c.OAuthScope != "" {
		argv = append(argv, "--oauth-scope", c.OAuthScope)
	}
	if c.OAuthRedirectURI != "" {
		argv = append(argv, "--oauth-redirect-uri", c.OAuthRedirectURI)
	}
	if c.OAuthFlow != "" && c.OAuthFlow != "auto" {
		argv = append(argv, "--oauth-flow", c.OAuthFlow)
	}

	return argv
}
