package oauth

import (
	"context"
	"fmt"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// Authorize drives the interactive authorization-code + PKCE flow using the handler
// surfaced by an OAuthAuthorizationRequiredError. It starts a local callback server,
// performs DCR if no client_id is configured, opens the browser, waits for the callback,
// and exchanges the code for tokens.
func Authorize(ctx context.Context, handler *mcptransport.OAuthHandler, o Options) error {
	cb, err := StartCallbackServer(o.RedirectURI)
	if err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}
	defer cb.Close()

	codeVerifier, err := mcptransport.GenerateCodeVerifier()
	if err != nil {
		return err
	}
	codeChallenge := mcptransport.GenerateCodeChallenge(codeVerifier)

	state, err := mcptransport.GenerateState()
	if err != nil {
		return err
	}

	if o.ClientID == "" {
		if err := handler.RegisterClient(ctx, o.EffectiveClientName()); err != nil {
			return fmt.Errorf("dynamic client registration failed: %w", err)
		}
	}

	authURL, err := handler.GetAuthorizationURL(ctx, state, codeChallenge)
	if err != nil {
		return fmt.Errorf("building authorization URL: %w", err)
	}

	fmt.Fprintln(util.Err, "Opening your browser to authorize...")
	fmt.Fprintf(util.Err, "If it does not open, visit:\n  %s\n", authURL)
	_ = OpenBrowser(authURL)

	code, gotState, err := cb.WaitForCode(ctx)
	if err != nil {
		return fmt.Errorf("waiting for authorization callback: %w", err)
	}

	if err := handler.ProcessAuthorizationResponse(ctx, code, gotState, codeVerifier); err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	fmt.Fprintln(util.Err, "Authorization successful.")
	return nil
}
