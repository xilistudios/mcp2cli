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
//
// store may be nil (used by tests); when present, client credentials obtained via
// dynamic client registration are persisted so later runs can refresh the tokens
// those credentials were issued for.
func Authorize(ctx context.Context, handler *mcptransport.OAuthHandler, o Options, store *SecretStore) error {
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
		// Persist the freshly registered client. mcp-go keeps it only in the
		// handler's memory, so without this the next run registers again and
		// the stored refresh token becomes unusable (invalid_client).
		if store != nil {
			if cid := handler.GetClientID(); cid != "" {
				if err := store.SaveClientInfo(ClientInfo{ClientID: cid, ClientSecret: handler.GetClientSecret()}); err != nil {
					fmt.Fprintf(util.Err, "mcp2cli: warning: could not persist OAuth client_id: %v\n", err)
				}
			}
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
