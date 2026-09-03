package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// ClientCredentialsToken performs an OAuth2 client_credentials grant against tokenEndpoint.
func ClientCredentialsToken(ctx context.Context, hc *http.Client, tokenEndpoint, clientID, clientSecret string, scopes []string) (*mcptransport.Token, error) {
	if hc == nil {
		hc = http.DefaultClient
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if len(scopes) > 0 {
		form.Set("scope", strings.Join(scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned no access_token: %s", string(body))
	}

	tok := &mcptransport.Token{
		AccessToken:  tr.AccessToken,
		TokenType:    tr.TokenType,
		RefreshToken: tr.RefreshToken,
		ExpiresIn:    tr.ExpiresIn,
		Scope:        tr.Scope,
	}
	if tr.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tok, nil
}

// ClientCredentialsHeader discovers the token endpoint for serverURL, performs a
// client_credentials grant, and returns an Authorization bearer header pair.
func ClientCredentialsHeader(ctx context.Context, hc *http.Client, o Options, serverURL string) ([2]string, error) {
	tok, err := clientCredentialsTokenFor(ctx, hc, o, serverURL)
	if err != nil {
		return [2]string{}, err
	}
	return [2]string{"Authorization", "Bearer " + tok.AccessToken}, nil
}

// clientCredentialsTokenFor discovers the endpoint and runs the grant.
func clientCredentialsTokenFor(ctx context.Context, hc *http.Client, o Options, serverURL string) (*mcptransport.Token, error) {
	te, err := DiscoverTokenEndpoint(ctx, hc, serverURL)
	if err != nil {
		return nil, err
	}
	return ClientCredentialsToken(ctx, hc, te, o.ClientID, o.ClientSecret, o.Scopes())
}

// CachedClientCredentialsHeader returns a bearer header for a client_credentials
// grant, reusing a keyring-cached token while it is valid.
//
// A client_credentials grant has no refresh token, so previously every
// invocation re-minted one - an extra discovery round trip plus token request
// per command. Caching is safe because the token is short-lived by contract and
// the cached copy carries its own ExpiresAt. forceRefresh (from --refresh)
// bypasses the cache, and a cached token within expirySafety of expiry is
// renewed so a long-running command does not fail mid-flight with a 401.
func CachedClientCredentialsHeader(
	ctx context.Context,
	hc *http.Client,
	o Options,
	serverURL string,
	store *SecretStore,
	forceRefresh bool,
) ([2]string, error) {
	if store != nil && !forceRefresh {
		if tok, ok := store.LoadGrant(GrantClientCredentials); ok {
			return [2]string{"Authorization", "Bearer " + tok.AccessToken}, nil
		}
	}

	tok, err := clientCredentialsTokenFor(ctx, hc, o, serverURL)
	if err != nil {
		return [2]string{}, err
	}

	if store != nil {
		if err := store.SaveGrant(GrantClientCredentials, tok); err != nil {
			fmt.Fprintf(util.Err, "mcp2cli: warning: could not cache client_credentials token: %v\n", err)
		}
	}
	return [2]string{"Authorization", "Bearer " + tok.AccessToken}, nil
}

// DiscoverTokenEndpoint discovers the OAuth token endpoint for an MCP server URL.
// Tries RFC9728 protected-resource metadata first, then the authorization-server metadata.
func DiscoverTokenEndpoint(ctx context.Context, hc *http.Client, serverURL string) (string, error) {
	if hc == nil {
		hc = http.DefaultClient
	}

	u, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("parsing server URL: %w", err)
	}
	origin := u.Scheme + "://" + u.Host

	fetchJSON := func(fetchURL string) (map[string]any, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := hc.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("GET %s returned %d", fetchURL, resp.StatusCode)
		}
		var m map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			return nil, err
		}
		return m, nil
	}

	// 1) Try protected-resource metadata first.
	prmURL := origin + "/.well-known/oauth-protected-resource"
	if prm, err := fetchJSON(prmURL); err == nil {
		if authServers, ok := prm["authorization_servers"].([]any); ok && len(authServers) > 0 {
			if asURL, ok := authServers[0].(string); ok && asURL != "" {
				asmURL := strings.TrimRight(asURL, "/") + "/.well-known/oauth-authorization-server"
				if asm, err := fetchJSON(asmURL); err == nil {
					if te, ok := asm["token_endpoint"].(string); ok && te != "" {
						return te, nil
					}
				}
			}
		}
	}

	// 2) Fallback: try authorization-server metadata at the server's origin.
	asmURL := origin + "/.well-known/oauth-authorization-server"
	if asm, err := fetchJSON(asmURL); err == nil {
		if te, ok := asm["token_endpoint"].(string); ok && te != "" {
			return te, nil
		}
	}

	return "", fmt.Errorf("could not discover OAuth token endpoint for %s", serverURL)
}
