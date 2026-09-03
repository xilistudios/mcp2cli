package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/zalando/go-keyring"

	"github.com/xilistudios/mcp2cli/internal/util"
)

// TestMain silences stderr so the store's one-time keyring warnings do not
// pollute test output; the assertions below check storage locations directly.
func TestMain(m *testing.M) {
	util.Err = io.Discard
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// fake keyring backend
// ---------------------------------------------------------------------------

// fakeKeyring is an in-memory stand-in for the OS keyring. It counts calls so
// tests can assert the keyring was, or was not, consulted.
type fakeKeyring struct {
	data      map[string]string // service\x00user -> payload
	getErr    error
	setErr    error
	deleteErr error
	gets      int
	sets      int
	deletes   int
}

func newFakeKeyring() *fakeKeyring {
	return &fakeKeyring{data: map[string]string{}}
}

func (f *fakeKeyring) key(service, user string) string { return service + "\x00" + user }

func (f *fakeKeyring) Get(service, user string) (string, error) {
	f.gets++
	if f.getErr != nil {
		return "", f.getErr
	}
	if v, ok := f.data[f.key(service, user)]; ok {
		return v, nil
	}
	return "", keyring.ErrNotFound
}

func (f *fakeKeyring) Set(service, user, password string) error {
	f.sets++
	if f.setErr != nil {
		return f.setErr
	}
	f.data[f.key(service, user)] = password
	return nil
}

func (f *fakeKeyring) Delete(service, user string) error {
	f.deletes++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.data[f.key(service, user)]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.data, f.key(service, user))
	return nil
}

func (f *fakeKeyring) get(user string) (string, bool) {
	v, ok := f.data[f.key(ServiceName, user)]
	return v, ok
}

// newStore returns a store over a healthy, empty fake keyring.
func newStore(t *testing.T) (*SecretStore, *fakeKeyring) {
	t.Helper()
	kr := newFakeKeyring()
	return newStoreWithKeyring(t, kr), kr
}

// newStoreWithKeyring returns a store over a caller-configured fake keyring.
func newStoreWithKeyring(t *testing.T, kr *fakeKeyring) *SecretStore {
	t.Helper()
	s := NewSecretStore("hash123", t.TempDir(), t.TempDir())
	s.keyring = kr
	return s
}

// newProcess returns a store as a *new* process would see it: same keyring and
// directories, none of the in-memory state.
func newProcess(t *testing.T, s *SecretStore, kr *fakeKeyring) *SecretStore {
	t.Helper()
	fresh := NewSecretStore("hash123", s.file.dir, s.legacy.dir)
	fresh.keyring = kr
	return fresh
}

func mustToken() *mcptransport.Token {
	return &mcptransport.Token{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(time.Hour),
		Scope:        "mcp.read",
	}
}

// listJSON returns the *.json names in dir, or nil if dir does not exist.
func listJSON(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// core behaviour
// ---------------------------------------------------------------------------

func TestSecretStore_RoundTripUsesKeyring(t *testing.T) {
	s, kr := newStore(t)

	if err := s.SaveToken(context.Background(), mustToken()); err != nil {
		t.Fatal(err)
	}
	if kr.sets != 1 {
		t.Fatalf("expected 1 keyring write, got %d", kr.sets)
	}
	// The whole point: nothing plaintext left on disk.
	if entries := listJSON(t, s.file.dir); len(entries) != 0 {
		t.Fatalf("expected no token file, found %v", entries)
	}

	got, err := s.GetToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "access-1" || got.RefreshToken != "refresh-1" {
		t.Fatalf("round trip lost material: %+v", got)
	}
	if got.ExpiresAt.IsZero() {
		t.Error("ExpiresAt must survive the round trip; mcp-go needs it to refresh")
	}
}

// The regression this change exists for: a token left behind in the disposable
// cache directory must be picked up and moved into durable storage.
func TestSecretStore_MigratesLegacyCacheToken(t *testing.T) {
	s, kr := newStore(t)

	legacyFile := filepath.Join(s.legacy.dir, "token.json")
	data, _ := json.Marshal(mustToken())
	if err := os.WriteFile(legacyFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetToken(context.Background())
	if err != nil {
		t.Fatalf("legacy token should be readable: %v", err)
	}
	if got.AccessToken != "access-1" {
		t.Fatalf("got %q", got.AccessToken)
	}
	if _, ok := kr.get("hash123:token"); !ok {
		t.Fatal("legacy token was not promoted to the keyring")
	}
	if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
		t.Fatal("legacy plaintext copy should be deleted after migration")
	}
}

// Wiping ~/.cache must no longer mean losing the refresh token.
func TestSecretStore_SurvivesCacheWipe(t *testing.T) {
	s, kr := newStore(t)
	if err := s.SaveToken(context.Background(), mustToken()); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(s.legacy.dir)
	os.RemoveAll(s.file.dir)

	got, err := newProcess(t, s, kr).GetToken(context.Background())
	if err != nil {
		t.Fatalf("token lost when the cache directory was wiped: %v", err)
	}
	if got.RefreshToken != "refresh-1" {
		t.Fatalf("refresh token lost: %+v", got)
	}
}

func TestSecretStore_NoTokenIsErrNoToken(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.GetToken(context.Background()); !errors.Is(err, mcptransport.ErrNoToken) {
		t.Fatalf("expected ErrNoToken, got %v", err)
	}
}

// An expired token must still be handed back, because mcp-go only attempts a
// refresh when GetToken succeeds. Reporting it absent would force a browser
// login on every expiry - the exact complaint being fixed.
func TestSecretStore_ReturnsExpiredTokenForRefresh(t *testing.T) {
	s, _ := newStore(t)
	expired := mustToken()
	expired.ExpiresAt = time.Now().Add(-time.Hour)
	if err := s.SaveToken(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsExpired() {
		t.Fatal("expected an expired token so the library attempts a refresh")
	}
	if got.RefreshToken == "" {
		t.Fatal("refresh token must be present for the silent renewal")
	}
}

// Keyring unavailable (headless box, no D-Bus, locked wallet) must degrade to
// the file fallback rather than break the CLI.
func TestSecretStore_FallsBackToFileWhenKeyringBroken(t *testing.T) {
	kr := newFakeKeyring()
	kr.getErr = errors.New("no session bus")
	kr.setErr = errors.New("no session bus")
	s := newStoreWithKeyring(t, kr)

	if err := s.SaveToken(context.Background(), mustToken()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.file.dir, "token.json")); err != nil {
		t.Fatalf("expected a file-backed token, got %v", err)
	}
	got, err := s.GetToken(context.Background())
	if err != nil || got.AccessToken != "access-1" {
		t.Fatalf("file fallback broken: token=%+v err=%v", got, err)
	}

	// After the first hard failure the keyring is not probed again.
	setsAfterFirst := kr.sets
	getsAfterFirst := kr.gets
	for i := 0; i < 3; i++ {
		if _, err := s.GetToken(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if kr.sets != setsAfterFirst || kr.gets != getsAfterFirst {
		t.Fatalf("a disabled keyring should not be retried (gets %d->%d)", getsAfterFirst, kr.gets)
	}
}

// A token too large for the Windows/macOS keyring limits must not be dropped.
func TestSecretStore_OversizedTokenGoesToFile(t *testing.T) {
	kr := newFakeKeyring()
	s := newStoreWithKeyring(t, kr)

	big := mustToken()
	big.AccessToken = strings.Repeat("x", maxKeyringBytes+100)
	if err := s.SaveToken(context.Background(), big); err != nil {
		t.Fatal(err)
	}
	if kr.sets != 0 {
		t.Fatal("an oversized payload should never be sent to the keyring")
	}
	if _, err := os.Stat(filepath.Join(s.file.dir, "token.json")); err != nil {
		t.Fatalf("oversized token should be file-backed: %v", err)
	}
	got, err := s.GetToken(context.Background())
	if err != nil || len(got.AccessToken) != len(big.AccessToken) {
		t.Fatalf("oversized token lost: err=%v", err)
	}
}

// Unparseable JSON must read as "absent" so the flow re-authorizes cleanly
// instead of failing on every subsequent request.
func TestSecretStore_CorruptTokenReadsAsAbsent(t *testing.T) {
	s, _ := newStore(t)
	if err := os.WriteFile(filepath.Join(s.file.dir, "token.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetToken(context.Background()); !errors.Is(err, mcptransport.ErrNoToken) {
		t.Fatalf("expected ErrNoToken for corrupt data, got %v", err)
	}
}

func TestSecretStore_HonoursContextCancellation(t *testing.T) {
	s, _ := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.GetToken(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if err := s.SaveToken(ctx, mustToken()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSecretStore_SaveTokenNil(t *testing.T) {
	s, _ := newStore(t)
	if err := s.SaveToken(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil token")
	}
}

// The store must satisfy the interface mcp-go expects, so a signature drift
// fails here rather than at the call site.
var _ mcptransport.TokenStore = (*SecretStore)(nil)
