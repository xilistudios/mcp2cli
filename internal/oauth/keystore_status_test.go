package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
)

// ---------------------------------------------------------------------------
// Forget (--oauth-reset)
// ---------------------------------------------------------------------------

func TestSecretStore_ForgetClearsEverything(t *testing.T) {
	s, kr := newStore(t)
	ctx := context.Background()
	if err := s.SaveToken(ctx, mustToken()); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveClientInfo(ClientInfo{ClientID: "cid"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveGrant(GrantClientCredentials, mustToken()); err != nil {
		t.Fatal(err)
	}

	if err := s.Forget(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetToken(ctx); !errors.Is(err, mcptransport.ErrNoToken) {
		t.Fatalf("token survived reset: %v", err)
	}
	if _, ok := s.LoadClientInfo(); ok {
		t.Fatal("client info survived reset")
	}
	if _, ok := s.LoadGrant(GrantClientCredentials); ok {
		t.Fatal("grant survived reset")
	}
	for _, acct := range []string{"hash123:token", "hash123:client", "hash123:token:client_credentials"} {
		if _, ok := kr.get(acct); ok {
			t.Fatalf("keyring still holds %s", acct)
		}
	}
}

// Reset must also clear a file-backed token, not just keyring entries.
func TestSecretStore_ForgetClearsFileFallback(t *testing.T) {
	kr := newFakeKeyring()
	kr.getErr = errors.New("no bus")
	s := newStoreWithKeyring(t, kr)
	if err := s.SaveToken(context.Background(), mustToken()); err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(); err != nil {
		t.Fatal(err)
	}
	if entries := listJSON(t, s.file.dir); len(entries) != 0 {
		t.Fatalf("files survived reset: %v", entries)
	}
}

func TestSecretStore_ForgetIsIdempotent(t *testing.T) {
	s, _ := newStore(t)
	if err := s.Forget(); err != nil {
		t.Fatalf("resetting an empty store should succeed: %v", err)
	}
}

// A machine whose keyring is dead (no D-Bus, locked wallet) must still be able
// to reset: the keyring cannot hold anything the next run would load, so the
// file sweep is enough.
func TestSecretStore_ForgetWithDeadKeyring(t *testing.T) {
	kr := newFakeKeyring()
	kr.getErr = errors.New("no bus")
	kr.setErr = errors.New("no bus")
	kr.deleteErr = errors.New("no bus")
	s := newStoreWithKeyring(t, kr)

	if err := s.SaveToken(context.Background(), mustToken()); err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(); err != nil {
		t.Fatalf("reset must not fail on an unreachable keyring: %v", err)
	}
	if entries := listJSON(t, s.file.dir); len(entries) != 0 {
		t.Fatalf("files survived reset: %v", entries)
	}
}

// If deletion fails but the entry is still readable, the secret survived the
// reset and the user must be told loudly.
func TestSecretStore_ForgetReportsSurvivingSecret(t *testing.T) {
	kr := newFakeKeyring()
	s := newStoreWithKeyring(t, kr)
	if err := s.SaveToken(context.Background(), mustToken()); err != nil {
		t.Fatal(err)
	}
	kr.deleteErr = errors.New("access denied")

	if err := s.Forget(); err == nil {
		t.Fatal("expected an error when a readable secret survives reset")
	}
}

// ---------------------------------------------------------------------------
// Status (--oauth-status)
// ---------------------------------------------------------------------------

func TestSecretStore_StatusValidToken(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if err := s.SaveToken(ctx, mustToken()); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveClientInfo(ClientInfo{ClientID: "cid"}); err != nil {
		t.Fatal(err)
	}

	st := s.Status()
	if st.TokenLocation != LocKeyring || st.ClientLocation != LocKeyring {
		t.Fatalf("locations = %q / %q", st.TokenLocation, st.ClientLocation)
	}
	if !st.HasToken || !st.HasRefresh || st.Expired {
		t.Fatalf("unexpected flags: %+v", st)
	}
	if st.ClientID != "cid" || st.Scope != "mcp.read" {
		t.Fatalf("got %+v", st)
	}
	if st.KeyringService != ServiceName {
		t.Fatalf("service = %q", st.KeyringService)
	}
	if st.Error != "" {
		t.Fatalf("unexpected error %q", st.Error)
	}
}

func TestSecretStore_StatusExpiredWithRefresh(t *testing.T) {
	s, _ := newStore(t)
	tok := mustToken()
	tok.ExpiresAt = time.Now().Add(-time.Minute)
	if err := s.SaveToken(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(); !st.HasToken || !st.HasRefresh || !st.Expired {
		t.Fatalf("expected expired-but-refreshable: %+v", st)
	}
}

// No refresh token means the next run must ask the user to log in; status
// should say so rather than look healthy.
func TestSecretStore_StatusExpiredWithoutRefresh(t *testing.T) {
	s, _ := newStore(t)
	tok := mustToken()
	tok.RefreshToken = ""
	tok.ExpiresAt = time.Now().Add(-time.Minute)
	if err := s.SaveToken(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(); !st.Expired || st.HasRefresh {
		t.Fatalf("expected expired and unrefreshable: %+v", st)
	}
}

func TestSecretStore_StatusEmpty(t *testing.T) {
	s, _ := newStore(t)
	st := s.Status()
	if st.HasToken || st.TokenLocation != LocNone {
		t.Fatalf("expected nothing stored: %+v", st)
	}
	if st.Error != "" {
		t.Fatalf("an empty store is not an error: %q", st.Error)
	}
}

func TestSecretStore_StatusReportsFileLocation(t *testing.T) {
	kr := newFakeKeyring()
	kr.getErr = errors.New("no bus")
	kr.setErr = errors.New("no bus")
	s := newStoreWithKeyring(t, kr)
	if err := s.SaveToken(context.Background(), mustToken()); err != nil {
		t.Fatal(err)
	}
	st := s.Status()
	if st.TokenLocation != LocFile {
		t.Fatalf("location = %q, want %q", st.TokenLocation, LocFile)
	}
	if !st.HasToken {
		t.Fatal("status must see the file-backed token")
	}
	if st.Error == "" {
		t.Fatal("status must mention the broken keyring")
	}
}

// Status is read-only: it must not migrate, rewrite or delete anything, so
// `--oauth-status` can never be the thing that breaks a working login.
func TestSecretStore_StatusDoesNotWrite(t *testing.T) {
	s, kr := newStore(t)
	s.strategy = BackendFile
	legacyFile := filepath.Join(s.legacy.dir, "token.json")
	data, _ := json.Marshal(mustToken())
	if err := os.WriteFile(legacyFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	st := s.Status()
	if st.TokenLocation != LocLegacy {
		t.Fatalf("location = %q, want %q", st.TokenLocation, LocLegacy)
	}
	if kr.sets != 0 || kr.deletes != 0 {
		t.Fatalf("Status wrote to the keyring (sets=%d deletes=%d)", kr.sets, kr.deletes)
	}
	if _, err := os.Stat(legacyFile); err != nil {
		t.Fatal("Status must not delete the legacy file")
	}
}

// ---------------------------------------------------------------------------
// Authorize end to end: DCR client credentials must be persisted
// ---------------------------------------------------------------------------

// freePort grabs an unused loopback port for the callback server.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// authServer is a stub OAuth authorization server. Its /authorize endpoint acts
// as the user's browser: it immediately calls back to the redirect URI with the
// issued code, so the interactive flow can be tested without a browser.
func authServer(t *testing.T) (srvURL string, dcrID string, issued *atomic.Int64) {
	t.Helper()
	var base string
	count := &atomic.Int64{}
	mux := http.NewServeMux()

	metadata := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                base,
			"authorization_endpoint":                base + "/authorize",
			"token_endpoint":                        base + "/token",
			"registration_endpoint":                 base + "/register",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none"},
		})
	}
	mux.HandleFunc("/.well-known/oauth-authorization-server", metadata)
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"authorization_servers": []string{base}})
	})

	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"client_id": "dcr-client-42"})
	})

	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// Behave like a browser completing the redirect.
		cb, err := url.ParseRequestURI(q.Get("redirect_uri"))
		if err != nil {
			t.Errorf("bad redirect_uri %q: %v", q.Get("redirect_uri"), err)
			return
		}
		cb.RawQuery = url.Values{"code": {"AUTH-CODE"}, "state": {q.Get("state")}}.Encode()
		resp, err := http.Get(cb.String())
		if err != nil {
			t.Errorf("callback request failed: %v", err)
			return
		}
		resp.Body.Close()
		w.WriteHeader(http.StatusFound)
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if got := r.FormValue("client_id"); got != "dcr-client-42" {
			t.Errorf("token exchange used client_id %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "issued-access",
			"refresh_token": "issued-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	base = srv.URL
	return base, "dcr-client-42", count
}

// TestAuthorize_PersistsDCRClientAndToken is the end-to-end guard for the
// frequent-login bug: after an interactive login that used dynamic client
// registration, both the client credentials and the tokens must be durable.
// If the client_id is not stored, the stored refresh token cannot be used by
// the next run and the user is asked to log in again.
func TestAuthorize_PersistsDCRClientAndToken(t *testing.T) {
	base, _, issued := authServer(t)
	s, kr := newStore(t)

	redirectURI := "http://127.0.0.1:" + strconv.Itoa(freePort(t)) + "/oauth/callback"
	handler := mcptransport.NewOAuthHandler(mcptransport.OAuthConfig{
		TokenStore:  s,
		PKCEEnabled: true,
		RedirectURI: redirectURI,
		Scopes:      []string{"mcp.read"},
	})
	handler.SetBaseURL(base)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// No client id supplied, so Authorize must register one and persist it.
	if err := Authorize(ctx, handler, Options{RedirectURI: redirectURI}, s); err != nil {
		t.Fatalf("Authorize failed: %v", err)
	}

	if got := issued.Load(); got != 1 {
		t.Fatalf("expected 1 token exchange, got %d", got)
	}

	// The registered client must survive into durable storage...
	if _, ok := kr.get("hash123:client"); !ok {
		t.Fatal("dynamic client registration did not persist the client_id")
	}
	ci, ok := s.LoadClientInfo()
	if !ok || ci.ClientID != "dcr-client-42" {
		t.Fatalf("stored client info = %+v ok=%v", ci, ok)
	}
	// ...and the handler agrees with what we stored, so refresh will use both.
	if handler.GetClientID() != ci.ClientID {
		t.Fatalf("handler client %q != stored %q", handler.GetClientID(), ci.ClientID)
	}

	// The token, including its refresh token, must be readable by a new process.
	tok, err := s.GetToken(ctx)
	if err != nil {
		t.Fatalf("token not persisted: %v", err)
	}
	if tok.AccessToken != "issued-access" || tok.RefreshToken != "issued-refresh" {
		t.Fatalf("unexpected token: %+v", tok)
	}
	if tok.ExpiresAt.IsZero() {
		t.Fatal("ExpiresAt must be set so the next run knows when to refresh")
	}
	fresh := newProcess(t, s, kr)
	if got, err := fresh.GetToken(ctx); err != nil || got.RefreshToken != "issued-refresh" {
		t.Fatalf("a new process cannot reuse the login: %+v err=%v", got, err)
	}
	if got, ok := fresh.LoadClientInfo(); !ok || got.ClientID != "dcr-client-42" {
		t.Fatalf("a new process cannot reuse the client_id: %+v ok=%v", got, ok)
	}
}

// Authorize must tolerate a nil store (used where persistence is not wanted)
// rather than panic after a successful login.
func TestAuthorize_NilStoreIsTolerated(t *testing.T) {
	base, _, _ := authServer(t)
	redirectURI := "http://127.0.0.1:" + strconv.Itoa(freePort(t)) + "/oauth/callback"
	handler := mcptransport.NewOAuthHandler(mcptransport.OAuthConfig{
		TokenStore:  mcptransport.NewMemoryTokenStore(),
		PKCEEnabled: true,
		RedirectURI: redirectURI,
	})
	handler.SetBaseURL(base)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := Authorize(ctx, handler, Options{RedirectURI: redirectURI}, nil); err != nil {
		t.Fatalf("Authorize with nil store failed: %v", err)
	}
}
