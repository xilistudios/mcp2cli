package oauth

import (
	"fmt"
	"net/url"
	"strings"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
)

// Options holds the CLI-supplied OAuth flags.
type Options struct {
	ClientID     string
	ClientSecret string
	ClientName   string
	Scope        string
	RedirectURI  string
	Flow         string
}

// Scopes splits the Scope string on spaces and commas, trims, and drops empties.
func (o Options) Scopes() []string {
	if o.Scope == "" {
		return nil
	}
	// Split on both spaces and commas.
	raw := strings.FieldsFunc(o.Scope, func(r rune) bool {
		return r == ' ' || r == ','
	})
	var out []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Validate checks that RedirectURI, if provided, meets RFC 8252 requirements.
func (o Options) Validate() error {
	if o.RedirectURI == "" {
		return nil
	}
	u, err := url.Parse(o.RedirectURI)
	if err != nil {
		return fmt.Errorf("invalid --oauth-redirect-uri: %w", err)
	}
	if u.Scheme != "http" {
		return fmt.Errorf("--oauth-redirect-uri must use http://, got '%s://'", u.Scheme)
	}
	if u.Port() == "" {
		return fmt.Errorf("--oauth-redirect-uri must include an explicit port number (e.g. http://localhost:3334/oauth/callback)")
	}
	host := u.Hostname()
	switch host {
	case "localhost", "127.0.0.1", "::1":
		// ok
	default:
		return fmt.Errorf("--oauth-redirect-uri host must be a loopback address (localhost or 127.0.0.1), got %q", host)
	}
	return nil
}

// BuildConfig creates an mcptransport.OAuthConfig from the CLI options and a token store.
func BuildConfig(o Options, store mcptransport.TokenStore) mcptransport.OAuthConfig {
	return mcptransport.OAuthConfig{
		ClientID:     o.ClientID,
		ClientSecret: o.ClientSecret,
		RedirectURI:  o.RedirectURI,
		Scopes:       o.Scopes(),
		TokenStore:   store,
		PKCEEnabled:  true,
	}
}

// EffectiveClientName returns the configured client name or the default.
func (o Options) EffectiveClientName() string {
	if o.ClientName != "" {
		return o.ClientName
	}
	return "mcp2cli"
}
