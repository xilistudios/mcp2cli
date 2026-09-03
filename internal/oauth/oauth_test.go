package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
)

func TestFileTokenStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFileTokenStore(dir)

	// No token yet → ErrNoToken.
	_, err := store.GetToken(context.Background())
	if !errors.Is(err, mcptransport.ErrNoToken) {
		t.Fatalf("expected ErrNoToken, got %v", err)
	}

	// Save and read back.
	in := &mcptransport.Token{AccessToken: "abc", ExpiresIn: 3600}
	if err := store.SaveToken(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	out, err := store.GetToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.AccessToken != "abc" {
		t.Fatalf("got AccessToken %q, want %q", out.AccessToken, "abc")
	}
}

func TestFileTokenStore_ClientInfo(t *testing.T) {
	dir := t.TempDir()
	store := NewFileTokenStore(dir)

	if _, ok := store.LoadClientInfo(); ok {
		t.Fatal("expected no client info in an empty store")
	}

	if err := store.SaveClientInfo(ClientInfo{ClientID: "cid", ClientSecret: "sec"}); err != nil {
		t.Fatal(err)
	}
	ci, ok := store.LoadClientInfo()
	if !ok || ci.ClientID != "cid" || ci.ClientSecret != "sec" {
		t.Fatalf("got %+v ok=%v", ci, ok)
	}

	// An empty client_id is meaningless and must not overwrite good data.
	if err := store.SaveClientInfo(ClientInfo{}); err == nil {
		t.Fatal("expected error saving empty client_id")
	}
}

// A store with no directory is inert: reads miss, writes are no-ops. This lets
// callers pass an optional legacy location without nil checks.
func TestFileTokenStore_Inert(t *testing.T) {
	store := NewFileTokenStore("")
	if _, err := store.GetToken(context.Background()); !errors.Is(err, mcptransport.ErrNoToken) {
		t.Fatalf("expected ErrNoToken, got %v", err)
	}
	if _, ok := store.LoadClientInfo(); ok {
		t.Fatal("expected no client info")
	}
	if err := store.SaveToken(context.Background(), &mcptransport.Token{AccessToken: "x"}); err != nil {
		t.Fatalf("inert store write should succeed as a no-op, got %v", err)
	}
}

func TestOptions_Validate(t *testing.T) {
	tests := []struct {
		uri     string
		wantErr string // empty = no error
	}{
		{"", ""},
		{"http://localhost:3334/oauth/callback", ""},
		{"https://localhost:3334/cb", "http://"},
		{"http://localhost/cb", "port"},
		{"http://example.com:3334/cb", "loopback"},
	}
	for _, tc := range tests {
		o := Options{RedirectURI: tc.uri}
		err := o.Validate()
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("Validate(%q) = %v, want nil", tc.uri, err)
			}
		} else {
			if err == nil {
				t.Errorf("Validate(%q) = nil, want error containing %q", tc.uri, tc.wantErr)
			} else if !contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate(%q) = %v, want error containing %q", tc.uri, err, tc.wantErr)
			}
		}
	}
}

func TestOptions_Scopes(t *testing.T) {
	o := Options{Scope: "mcp.read mcp.write, admin"}
	got := o.Scopes()
	want := []string{"mcp.read", "mcp.write", "admin"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, s := range got {
		if s != want[i] {
			t.Fatalf("scope[%d] = %q, want %q", i, s, want[i])
		}
	}
}

func TestBuildConfig(t *testing.T) {
	store := NewFileTokenStore(t.TempDir())
	o := Options{
		ClientID:     "cid",
		ClientSecret: "csec",
		RedirectURI:  "http://localhost:3334/cb",
		Scope:        "mcp.read mcp.write",
	}
	cfg := BuildConfig(o, store)
	if !cfg.PKCEEnabled {
		t.Error("expected PKCEEnabled=true")
	}
	if cfg.ClientID != "cid" {
		t.Errorf("ClientID = %q", cfg.ClientID)
	}
	if cfg.ClientSecret != "csec" {
		t.Errorf("ClientSecret = %q", cfg.ClientSecret)
	}
	if cfg.RedirectURI != "http://localhost:3334/cb" {
		t.Errorf("RedirectURI = %q", cfg.RedirectURI)
	}
	if len(cfg.Scopes) != 2 {
		t.Errorf("Scopes len = %d", len(cfg.Scopes))
	}
	if cfg.TokenStore == nil {
		t.Error("TokenStore is nil")
	}
}

func TestCallbackServer(t *testing.T) {
	cb, err := StartCallbackServer("http://127.0.0.1:0/oauth/callback")
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()

	addr := cb.Addr()

	go func() {
		http.Get("http://" + addr + "/oauth/callback?code=CODE&state=STATE")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	code, state, err := cb.WaitForCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if code != "CODE" {
		t.Errorf("code = %q, want %q", code, "CODE")
	}
	if state != "STATE" {
		t.Errorf("state = %q, want %q", state, "STATE")
	}
}

func TestClientCredentialsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("grant_type") != "client_credentials" {
			t.Errorf("grant_type = %q", r.FormValue("grant_type"))
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "cid" || pass != "csec" {
			t.Errorf("basic auth = %q/%q ok=%v", user, pass, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	tok, err := ClientCredentialsToken(context.Background(), nil, srv.URL, "cid", "csec", []string{"read"})
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "tok" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
	if tok.ExpiresAt.IsZero() {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestClientCredentialsToken_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()

	_, err := ClientCredentialsToken(context.Background(), nil, srv.URL, "x", "y", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "400") {
		t.Errorf("error = %v, want containing 400", err)
	}
}

func TestClientCredentialsHeader(t *testing.T) {
	var tokenURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"token_endpoint": tokenURL,
			})
		case "/token":
			if r.FormValue("grant_type") != "client_credentials" {
				t.Errorf("grant_type = %q", r.FormValue("grant_type"))
			}
			user, pass, ok := r.BasicAuth()
			if !ok || user != "id" || pass != "sec" {
				t.Errorf("basic auth = %q/%q ok=%v", user, pass, ok)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "TOK",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	tokenURL = srv.URL + "/token"

	hdr, err := ClientCredentialsHeader(context.Background(), nil, Options{ClientID: "id", ClientSecret: "sec"}, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if hdr[0] != "Authorization" || hdr[1] != "Bearer TOK" {
		t.Errorf("got %v, want {Authorization Bearer TOK}", hdr)
	}
}

func TestDiscoverTokenEndpoint(t *testing.T) {
	var tokenEndpoint string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"token_endpoint": tokenEndpoint,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	tokenEndpoint = srv.URL + "/token"

	endpoint, err := DiscoverTokenEndpoint(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != srv.URL+"/token" {
		t.Errorf("endpoint = %q, want %q", endpoint, srv.URL+"/token")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
