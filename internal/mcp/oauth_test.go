package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
)

func TestConnectOAuth_Unreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ConnectOAuth(ctx, "http://127.0.0.1:1/x", nil,
		mcptransport.OAuthConfig{TokenStore: mcptransport.NewMemoryTokenStore()},
		"streamable", nil)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestConnectOAuth_AuthRequiredNoAuthorize(t *testing.T) {
	// Server that always returns 401 with a WWW-Authenticate header.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="http://example.com/meta"`)
		w.WriteHeader(401)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := ConnectOAuth(ctx, srv.URL, nil,
		mcptransport.OAuthConfig{TokenStore: mcptransport.NewMemoryTokenStore()},
		"streamable", nil)
	// We expect an error (auth required, no authorize callback).
	if err == nil {
		t.Fatal("expected error when auth required but no authorize function")
	}
}
