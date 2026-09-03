package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
)

// ---------------------------------------------------------------------------
// client credentials (the invalid_client half of the bug)
// ---------------------------------------------------------------------------

func TestSecretStore_ClientInfoRoundTrip(t *testing.T) {
	s, kr := newStore(t)

	if _, ok := s.LoadClientInfo(); ok {
		t.Fatal("expected empty client info")
	}
	if err := s.SaveClientInfo(ClientInfo{ClientID: "dcr-id", ClientSecret: "dcr-sec"}); err != nil {
		t.Fatal(err)
	}
	ci, ok := s.LoadClientInfo()
	if !ok || ci.ClientID != "dcr-id" || ci.ClientSecret != "dcr-sec" {
		t.Fatalf("got %+v ok=%v", ci, ok)
	}
	if _, ok := kr.get("hash123:client"); !ok {
		t.Fatal("client info should live in the keyring")
	}
	// Refusing an empty client_id protects previously stored, working data.
	if err := s.SaveClientInfo(ClientInfo{}); err == nil {
		t.Fatal("expected error saving an empty client_id")
	}
	if ci, ok := s.LoadClientInfo(); !ok || ci.ClientID != "dcr-id" {
		t.Fatalf("rejected write clobbered good data: %+v ok=%v", ci, ok)
	}
}

// A second run must see the same client_id, or its stored refresh token is dead
// on arrival with invalid_client. This was the main cause of frequent logins.
func TestSecretStore_ClientInfoSurvivesNewProcess(t *testing.T) {
	s, kr := newStore(t)
	if err := s.SaveClientInfo(ClientInfo{ClientID: "stable", ClientSecret: "sec"}); err != nil {
		t.Fatal(err)
	}
	ci, ok := newProcess(t, s, kr).LoadClientInfo()
	if !ok || ci.ClientID != "stable" || ci.ClientSecret != "sec" {
		t.Fatalf("client credentials lost between runs: %+v ok=%v", ci, ok)
	}
}

func TestSecretStore_ClientInfoMigratesFromLegacy(t *testing.T) {
	s, kr := newStore(t)
	data, _ := json.Marshal(ClientInfo{ClientID: "legacy-cid"})
	if err := os.WriteFile(filepath.Join(s.legacy.dir, "client.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	ci, ok := s.LoadClientInfo()
	if !ok || ci.ClientID != "legacy-cid" {
		t.Fatalf("legacy client info not read: %+v ok=%v", ci, ok)
	}
	if _, ok := kr.get("hash123:client"); !ok {
		t.Fatal("legacy client info was not promoted to the keyring")
	}
}

// ---------------------------------------------------------------------------
// backend selection
// ---------------------------------------------------------------------------

func TestSecretStore_FileStrategySkipsKeyring(t *testing.T) {
	kr := newFakeKeyring()
	s := newStoreWithKeyring(t, kr)
	s.strategy = BackendFile

	if err := s.SaveToken(context.Background(), mustToken()); err != nil {
		t.Fatal(err)
	}
	if kr.sets != 0 || kr.gets != 0 {
		t.Fatalf("file strategy must not touch the keyring (sets=%d gets=%d)", kr.sets, kr.gets)
	}
	if _, err := os.Stat(filepath.Join(s.file.dir, "token.json")); err != nil {
		t.Fatal("expected the token on disk")
	}
}

func TestSecretStore_KeyringStrategyDoesNotDowngradeSilently(t *testing.T) {
	kr := newFakeKeyring()
	kr.getErr = errors.New("locked")
	kr.setErr = errors.New("locked")
	s := newStoreWithKeyring(t, kr)
	s.strategy = BackendKeyring

	if err := s.SaveToken(context.Background(), mustToken()); err == nil {
		t.Fatal("expected an error rather than a silent plaintext fallback")
	}
	if _, err := os.Stat(filepath.Join(s.file.dir, "token.json")); err == nil {
		t.Fatal("keyring-only strategy must not write plaintext files")
	}
}

// Even an oversized payload must not sneak into a file when the operator
// demanded keyring storage.
func TestSecretStore_KeyringStrategyRejectsOversized(t *testing.T) {
	s, _ := newStore(t)
	s.strategy = BackendKeyring

	big := mustToken()
	big.AccessToken = strings.Repeat("y", maxKeyringBytes+100)
	if err := s.SaveToken(context.Background(), big); err == nil {
		t.Fatal("expected an error, not a plaintext fallback")
	}
	if entries := listJSON(t, s.file.dir); len(entries) != 0 {
		t.Fatalf("keyring-only strategy wrote %v", entries)
	}
}

func TestStoreStrategy_EnvOverride(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want string
	}{
		{"", BackendAuto},
		{"file", BackendFile},
		{"FILE", BackendFile},
		{" keyring ", BackendKeyring},
		{"nonsense", BackendAuto},
	} {
		t.Run("env="+tc.env, func(t *testing.T) {
			t.Setenv(EnvTokenStore, tc.env)
			if got := storeStrategy(); got != tc.want {
				t.Fatalf("storeStrategy() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// cached grants (client_credentials)
// ---------------------------------------------------------------------------

func TestSecretStore_GrantCache(t *testing.T) {
	s, _ := newStore(t)

	if _, ok := s.LoadGrant(GrantClientCredentials); ok {
		t.Fatal("expected no cached grant")
	}
	if err := s.SaveGrant(GrantClientCredentials, mustToken()); err != nil {
		t.Fatal(err)
	}
	got, ok := s.LoadGrant(GrantClientCredentials)
	if !ok || got.AccessToken != "access-1" {
		t.Fatalf("grant not cached: %+v ok=%v", got, ok)
	}
	// A grant token must never collide with the interactive token.
	if _, err := s.GetToken(context.Background()); !errors.Is(err, mcptransport.ErrNoToken) {
		t.Fatalf("grant leaked into the interactive token: %v", err)
	}
}

func TestSecretStore_GrantExpiredIsNotLoaded(t *testing.T) {
	s, _ := newStore(t)
	tok := mustToken()
	tok.ExpiresAt = time.Now().Add(-time.Minute)
	if err := s.SaveGrant(GrantClientCredentials, tok); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.LoadGrant(GrantClientCredentials); ok {
		t.Fatal("expired grant must be re-minted")
	}
}

// A grant close to expiry is renewed early so a long command cannot 401
// mid-flight with a bearer that expires while it runs.
func TestSecretStore_GrantNearExpiryIsNotLoaded(t *testing.T) {
	s, _ := newStore(t)
	tok := mustToken()
	tok.ExpiresAt = time.Now().Add(expirySafety / 2)
	if err := s.SaveGrant(GrantClientCredentials, tok); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.LoadGrant(GrantClientCredentials); ok {
		t.Fatal("grant inside the safety window must be renewed")
	}
}

func TestSecretStore_SaveGrantRejectsEmpty(t *testing.T) {
	s, _ := newStore(t)
	if err := s.SaveGrant(GrantClientCredentials, &mcptransport.Token{}); err == nil {
		t.Fatal("expected error for an empty access token")
	}
	if err := s.SaveGrant(GrantClientCredentials, nil); err == nil {
		t.Fatal("expected error for a nil token")
	}
}

// ---------------------------------------------------------------------------
// CachedClientCredentialsHeader
// ---------------------------------------------------------------------------

// tokenServer serves OAuth metadata plus a token endpoint, counting grants.
func tokenServer(t *testing.T, expiresIn int64) (*httptest.Server, *int) {
	t.Helper()
	var grants int
	var tokenURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"token_endpoint": tokenURL})
		case "/token":
			grants++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "TOK",
				"token_type":   "Bearer",
				"expires_in":   expiresIn,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	tokenURL = srv.URL + "/token"
	t.Cleanup(srv.Close)
	return srv, &grants
}

func TestCachedClientCredentialsHeader_CachesGrant(t *testing.T) {
	srv, grants := tokenServer(t, 3600)
	s, kr := newStore(t)
	o := Options{ClientID: "id", ClientSecret: "sec"}

	for i := 0; i < 3; i++ {
		hdr, err := CachedClientCredentialsHeader(context.Background(), nil, o, srv.URL, s, false)
		if err != nil {
			t.Fatal(err)
		}
		if hdr[0] != "Authorization" || hdr[1] != "Bearer TOK" {
			t.Fatalf("got %v", hdr)
		}
	}
	if *grants != 1 {
		t.Fatalf("expected 1 token request for 3 commands, got %d", *grants)
	}
	if _, ok := kr.get("hash123:token:client_credentials"); !ok {
		t.Fatal("grant token should be in the keyring")
	}
}

func TestCachedClientCredentialsHeader_ForceRefreshRemints(t *testing.T) {
	srv, grants := tokenServer(t, 3600)
	s, _ := newStore(t)
	o := Options{ClientID: "id", ClientSecret: "sec"}

	if _, err := CachedClientCredentialsHeader(context.Background(), nil, o, srv.URL, s, false); err != nil {
		t.Fatal(err)
	}
	if _, err := CachedClientCredentialsHeader(context.Background(), nil, o, srv.URL, s, true); err != nil {
		t.Fatal(err)
	}
	if *grants != 2 {
		t.Fatalf("--refresh should bypass the cache: got %d grants", *grants)
	}
}

// An already-expired cached grant must be re-minted instead of sent as a bearer
// the server will reject.
func TestCachedClientCredentialsHeader_RemintsExpiredGrant(t *testing.T) {
	srv, grants := tokenServer(t, 1)
	s, _ := newStore(t)
	o := Options{ClientID: "id", ClientSecret: "sec"}

	if _, err := CachedClientCredentialsHeader(context.Background(), nil, o, srv.URL, s, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	if _, err := CachedClientCredentialsHeader(context.Background(), nil, o, srv.URL, s, false); err != nil {
		t.Fatal(err)
	}
	if *grants != 2 {
		t.Fatalf("expired grant was reused: got %d grants", *grants)
	}
}

// A store that cannot persist must not break the grant.
func TestCachedClientCredentialsHeader_NilStoreIsFine(t *testing.T) {
	srv, _ := tokenServer(t, 3600)
	hdr, err := CachedClientCredentialsHeader(context.Background(), nil,
		Options{ClientID: "id", ClientSecret: "sec"}, srv.URL, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if hdr[1] != "Bearer TOK" {
		t.Fatalf("got %v", hdr)
	}
}
