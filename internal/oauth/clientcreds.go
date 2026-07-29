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
