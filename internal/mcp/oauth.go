package mcp

import (
	"context"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
)

// ConnectOAuth establishes an HTTP MCP client with OAuth (authorization-code
// + PKCE, with dynamic client registration when no client_id is configured).
// On an OAuth-authorization-required error it invokes authorize (which runs
// the browser flow) and retries.
//
// transport: "auto" | "streamable" | "sse".
func ConnectOAuth(
	ctx context.Context,
	baseURL string,
	authHeaders [][2]string,
	oauthCfg mcptransport.OAuthConfig,
	transport string,
	authorize func(ctx context.Context, handler *mcptransport.OAuthHandler) error,
) (*Client, error) {
	headers := make(map[string]string, len(authHeaders))
	for _, h := range authHeaders {
		headers[h[0]] = h[1]
	}

	// build constructs either a streamable or SSE OAuth client.
	build := func(sse bool) (*mcpgo.Client, error) {
		if sse {
			var opts []mcptransport.ClientOption
			if len(headers) > 0 {
				opts = append(opts, mcpgo.WithHeaders(headers))
			}
			return mcpgo.NewOAuthSSEClient(baseURL, oauthCfg, opts...)
		}
		var opts []mcptransport.StreamableHTTPCOption
		if len(headers) > 0 {
			opts = append(opts, mcptransport.WithHTTPHeaders(headers))
		}
		return mcpgo.NewOAuthStreamableHttpClient(baseURL, oauthCfg, opts...)
	}

	// handleAuth inspects err for an OAuth-authorization-required error.
	// If authorize is non-nil and the error is auth-required, it calls
	// authorize and returns nil on success (signaling a retry).
	handleAuth := func(err error) error {
		if !mcpgo.IsOAuthAuthorizationRequiredError(err) {
			return err
		}
		if authorize == nil {
			return err
		}
		h := mcpgo.GetOAuthHandler(err)
		if h == nil {
			return err
		}
		return authorize(ctx, h)
	}

	// startInit starts the client, initializes the MCP session, and
	// handles OAuth auth-required errors by running the authorize flow
	// and retrying once.
	startInit := func(c *mcpgo.Client) error {
		if err := c.Start(ctx); err != nil {
			if aerr := handleAuth(err); aerr != nil {
				c.Close()
				return aerr
			}
			// authorize succeeded — retry Start.
			if err2 := c.Start(ctx); err2 != nil {
				c.Close()
				return fmt.Errorf("starting OAuth client: %w", err2)
			}
		}

		if err := initClient(ctx, c); err != nil {
			if aerr := handleAuth(err); aerr != nil {
				c.Close()
				return aerr
			}
			// authorize succeeded — retry initClient.
			if err2 := initClient(ctx, c); err2 != nil {
				c.Close()
				return err2
			}
		}
		return nil
	}

	switch transport {
	case "sse":
		c, err := build(true)
		if err != nil {
			return nil, err
		}
		if err := startInit(c); err != nil {
			return nil, err
		}
		return &Client{inner: c}, nil

	case "streamable":
		c, err := build(false)
		if err != nil {
			return nil, err
		}
		if err := startInit(c); err != nil {
			return nil, err
		}
		return &Client{inner: c}, nil

	default: // "auto" — try streamable then fall back to SSE.
		c, err := build(false)
		if err == nil {
			if serr := startInit(c); serr == nil {
				return &Client{inner: c}, nil
			}
			c.Close()
		}

		c2, err2 := build(true)
		if err2 != nil {
			return nil, err2
		}
		if err := startInit(c2); err != nil {
			return nil, err
		}
		return &Client{inner: c2}, nil
	}
}
