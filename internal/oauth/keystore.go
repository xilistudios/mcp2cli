// Package oauth implements mcp2cli's OAuth support: token persistence in the
// OS keyring (with a file fallback), authorization-code + PKCE, dynamic client
// registration, a loopback callback server, and client_credentials grants.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/xilistudios/mcp2cli/internal/util"
	"github.com/zalando/go-keyring"
)

// ServiceName is the keyring namespace for mcp2cli's OAuth material.
const ServiceName = "mcp2cli-oauth"

// maxKeyringBytes bounds a keyring payload. Windows caps a credential at ~2560
// bytes and macOS caps service+user+secret at ~3000, so oversized tokens (long
// JWTs are common) go to the file backend instead of failing the grant.
const maxKeyringBytes = 2048

// expirySafety is how close to expiry a cached grant token must still be
// considered usable. Renewing slightly early keeps a long-running command from
// authenticating with a bearer that dies mid-flight.
const expirySafety = 30 * time.Second

// keyringTimeout caps how long we wait on the keyring. Secret-service agents
// can be absent, slow, or locked; a CLI must not hang on them.
const keyringTimeout = 3 * time.Second

// Backend names for the MCP2CLI_TOKEN_STORE override.
const (
	BackendAuto    = "auto"
	BackendKeyring = "keyring"
	BackendFile    = "file"
)

// EnvTokenStore selects the persistence backend; "file" is useful on headless
// hosts and in CI where no keyring daemon exists.
const EnvTokenStore = "MCP2CLI_TOKEN_STORE"

// ErrNoStoredToken is returned by a backend that has no entry. It is distinct
// from an operational failure so callers never mistake "unreadable" for
// "absent" - the old behaviour silently threw away refreshable tokens.
var ErrNoStoredToken = errors.New("no stored OAuth token")

// keyringBackend abstracts the OS keyring so tests can substitute a fake.
type keyringBackend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

// osKeyring is the real, process-wide go-keyring backend, wrapped with a
// timeout because go-keyring takes no context.
type osKeyring struct{}

func (osKeyring) Get(service, user string) (string, error) {
	return withTimeout(func() (string, error) { return keyring.Get(service, user) })
}

func (osKeyring) Set(service, user, password string) error {
	_, err := withTimeout(func() (string, error) { return "", keyring.Set(service, user, password) })
	return err
}

func (osKeyring) Delete(service, user string) error {
	_, err := withTimeout(func() (string, error) { return "", keyring.Delete(service, user) })
	return err
}

// withTimeout runs a blocking keyring call, giving up after keyringTimeout.
// A stranded goroutine is acceptable for a short-lived CLI process.
func withTimeout(fn func() (string, error)) (string, error) {
	type result struct {
		v   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := fn()
		ch <- result{v: v, err: err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(keyringTimeout):
		return "", errors.New("keyring did not respond within " + keyringTimeout.String())
	}
}

// defaultKeyring is swappable in tests.
var defaultKeyring keyringBackend = osKeyring{}

// SecretStore persists OAuth tokens and client credentials in the OS keyring,
// falling back to a 0600 file when no keyring is reachable.
//
// It implements mcptransport.TokenStore, so mcp-go keeps doing what it already
// does well: refresh an expired access token using the refresh token, and
// surface an authorization-required error - which mcp2cli turns into an
// interactive login - only when refreshing genuinely cannot work.
//
// Why the keyring: this material used to live under ~/.cache, a disposable
// directory. Cache cleaners, systemd-tmpfiles and ephemeral containers wipe
// it, and a wiped refresh token forces a full browser login.
type SecretStore struct {
	hash string

	mu       sync.Mutex
	keyring  keyringBackend
	disabled bool  // keyring proved unusable; stop probing it
	disErr   error // why the keyring was disabled, for --oauth-status
	warned   map[string]bool

	file     *FileTokenStore // durable fallback (and modern location)
	legacy   *FileTokenStore // read-only: pre-keyring location under ~/.cache
	strategy string
}

// NewSecretStore returns a store for one MCP server, keyed by that server's
// source hash. dir is the file fallback location; legacyDir (may be "") is the
// pre-keyring location whose contents are migrated on first read.
func NewSecretStore(hash, dir, legacyDir string) *SecretStore {
	return &SecretStore{
		hash:     hash,
		keyring:  defaultKeyring,
		warned:   map[string]bool{},
		file:     NewFileTokenStore(dir),
		legacy:   NewFileTokenStore(legacyDir),
		strategy: storeStrategy(),
	}
}

// storeStrategy reads the backend override at call time so tests can use
// t.Setenv.
func storeStrategy() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvTokenStore))) {
	case BackendFile:
		return BackendFile
	case BackendKeyring:
		return BackendKeyring
	default:
		return BackendAuto
	}
}

// account builds the keyring user name for a kind, e.g. "1a2b...:token".
func (s *SecretStore) account(kind string) string { return s.hash + ":" + kind }

// warnf prints a one-time notice per tag to stderr. Losing keyring persistence
// is worth saying out loud, but not on every request.
func (s *SecretStore) warnf(tag, format string, args ...any) {
	s.mu.Lock()
	if s.warned[tag] {
		s.mu.Unlock()
		return
	}
	s.warned[tag] = true
	s.mu.Unlock()
	fmt.Fprintf(util.Err, "mcp2cli: "+format+"\n", args...)
}

// keyringEnabled reports whether the keyring should be attempted.
func (s *SecretStore) keyringEnabled() bool {
	if s.strategy == BackendFile {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.disabled
}

// disableKeyring stops using the keyring for this process after a hard error.
// It is state-only; callers word the warning to match what they were doing.
// It reports whether this call was the one that disabled the keyring, so the
// warning is emitted once and not on every subsequent access.
func (s *SecretStore) disableKeyring(cause error) (firstTime bool) {
	s.mu.Lock()
	firstTime = !s.disabled
	s.disabled = true
	if s.disErr == nil {
		s.disErr = cause
	}
	s.mu.Unlock()
	return firstTime
}

// keyringIssue reports why the keyring is out of the picture, if it is.
func (s *SecretStore) keyringIssue() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disabled {
		return s.disErr
	}
	return nil
}

// get returns a stored payload. It reports ErrNoStoredToken when the entry is
// absent everywhere, and a real error only for operational failures - which
// must not be silently converted into "log in again".
//
// A payload found in the file backend is copied into the keyring so an upgrade
// stops depending on plaintext files, and the redundant copy is removed.
func (s *SecretStore) get(kind string) ([]byte, error) {
	if s.keyringEnabled() {
		v, err := s.keyring.Get(ServiceName, s.account(kind))
		switch {
		case err == nil && v != "":
			return []byte(v), nil
		case err != nil && !errors.Is(err, keyring.ErrNotFound):
			if s.disableKeyring(err) {
				s.warnf("kr-off", "OS keyring unavailable (%v); using OAuth files in %s", err, s.file.dir)
			}
		}
	}

	// Require a real keyring when asked; do not degrade quietly.
	if s.strategy == BackendKeyring && !s.keyringEnabled() {
		return nil, fmt.Errorf("%s: %w", s.account(kind), ErrKeyringRequired)
	}

	data, err := s.file.read(kind)
	if errors.Is(err, os.ErrNotExist) {
		// Migrate anything left in the disposable cache location.
		if data, err = s.legacy.read(kind); errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoStoredToken
		}
		if err == nil && len(data) > 0 {
			if perr := s.put(kind, data); perr == nil {
				s.legacy.remove(kind)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("reading OAuth %s: %w", kind, err)
	}
	if len(data) == 0 {
		return nil, ErrNoStoredToken
	}
	if perr := s.copyToKeyring(kind, data); perr == nil {
		s.file.remove(kind)
	}
	return data, nil
}

// put stores a payload in the keyring when possible, otherwise in the file.
// A successful keyring write removes the plaintext file copies so only one
// durable copy exists.
func (s *SecretStore) put(kind string, data []byte) error {
	if !s.keyringEnabled() {
		if s.strategy == BackendKeyring {
			return fmt.Errorf("storing OAuth %s: %w", kind, ErrKeyringRequired)
		}
		return s.file.write(kind, data)
	}

	if len(data) > maxKeyringBytes {
		if s.strategy == BackendKeyring {
			return fmt.Errorf("OAuth %s is %d bytes, over the %d-byte limit the keyring can hold (%s=keyring)",
				kind, len(data), maxKeyringBytes, EnvTokenStore)
		}
		s.warnf("kr-big", "OAuth %s is %d bytes, too large for the keyring; storing it in %s",
			kind, len(data), s.file.path(kind))
		return s.file.write(kind, data)
	}

	if err := s.keyring.Set(ServiceName, s.account(kind), string(data)); err == nil {
		s.file.remove(kind)
		s.legacy.remove(kind)
		return nil
	} else if s.strategy == BackendKeyring {
		return fmt.Errorf("storing OAuth %s in the keyring: %w", kind, err)
	} else {
		// The keyring cannot take writes either: stop probing it and live on
		// the file backend instead of failing on every save.
		if s.disableKeyring(err) {
			s.warnf("kr-off", "OS keyring unavailable (%v); storing OAuth tokens in %s", err, s.file.dir)
		}
		return s.file.write(kind, data)
	}
}

// copyToKeyring mirrors a file-sourced payload into the keyring without
// disturbing the file when the keyring is unavailable.
func (s *SecretStore) copyToKeyring(kind string, data []byte) error {
	if !s.keyringEnabled() || len(data) > maxKeyringBytes {
		return errors.New("keyring unavailable for this payload")
	}
	if v, err := s.keyring.Get(ServiceName, s.account(kind)); err == nil && v != "" {
		return nil // already present; nothing to promote
	}
	return s.keyring.Set(ServiceName, s.account(kind), string(data))
}

// ErrKeyringRequired is returned when MCP2CLI_TOKEN_STORE=keyring is set but no
// keyring is usable, so operators who demand keyring storage are not silently
// downgraded to plaintext files.
var ErrKeyringRequired = errors.New("OS keyring is required by " + EnvTokenStore + "=keyring but is unavailable")

// ---------------------------------------------------------------------------
// mcptransport.TokenStore
// ---------------------------------------------------------------------------

// GetToken returns the stored token, or mcptransport.ErrNoToken when nothing is
// persisted.
//
// An EXPIRED token is still returned: mcp-go only attempts a refresh when
// GetToken succeeds, so reporting an expired token as absent would turn every
// expiry into a full browser login instead of a silent renewal.
func (s *SecretStore) GetToken(ctx context.Context) (*mcptransport.Token, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := s.get(kindToken)
	if err != nil {
		if errors.Is(err, ErrNoStoredToken) {
			return nil, mcptransport.ErrNoToken
		}
		return nil, err
	}
	var tok mcptransport.Token
	if err := json.Unmarshal(raw, &tok); err != nil {
		// Corrupt data must not masquerade as a valid-but-expired token;
		// treat it as absent so the flow re-authorizes cleanly.
		s.warnf("tok-bad", "ignoring unreadable OAuth token for %s: %v", s.hash, err)
		return nil, mcptransport.ErrNoToken
	}
	if tok.AccessToken == "" {
		return nil, mcptransport.ErrNoToken
	}
	return &tok, nil
}

// SaveToken persists a token including its refresh token and expiry.
func (s *SecretStore) SaveToken(ctx context.Context, t *mcptransport.Token) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t == nil {
		return errors.New("oauth: SaveToken called with nil token")
	}
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("encoding token: %w", err)
	}
	return s.put(kindToken, data)
}

// ---------------------------------------------------------------------------
// Client credentials
// ---------------------------------------------------------------------------

// LoadClientInfo returns persisted client credentials, or (zero, false) when
// none exist.
func (s *SecretStore) LoadClientInfo() (ClientInfo, bool) {
	raw, err := s.get(kindClient)
	if err != nil {
		if !errors.Is(err, ErrNoStoredToken) {
			s.warnf("cid-read", "cannot read OAuth client info for %s: %v", s.hash, err)
		}
		return ClientInfo{}, false
	}
	var ci ClientInfo
	if err := json.Unmarshal(raw, &ci); err != nil || ci.ClientID == "" {
		return ClientInfo{}, false
	}
	return ci, true
}

// SaveClientInfo persists client credentials obtained from dynamic client
// registration or from flags.
//
// This is the fix for the second half of the token-loss loop: DCR mints a new
// client_id per run, but a refresh token is only valid for the client that was
// issued it. Without this, every run re-registered, and the stored refresh
// token died with `invalid_client` - forcing a browser login even when the
// token on disk was perfectly good.
func (s *SecretStore) SaveClientInfo(ci ClientInfo) error {
	if ci.ClientID == "" {
		return errors.New("oauth: refusing to save empty client_id")
	}
	data, err := json.Marshal(ci)
	if err != nil {
		return fmt.Errorf("encoding client info: %w", err)
	}
	return s.put(kindClient, data)
}

// Forget clears this server's stored token and client credentials. It backs
// --oauth-reset: a bad token cannot be fixed by retrying the login behind it.
func (s *SecretStore) Forget() error {
	kinds := []string{kindToken, kindClient, grantKind(GrantClientCredentials)}
	var problems []string
	if s.keyringEnabled() {
		for _, kind := range kinds {
			if !s.keyringEnabled() {
				break // disabled by the check below; stop probing
			}
			err := s.keyring.Delete(ServiceName, s.account(kind))
			if err == nil || errors.Is(err, keyring.ErrNotFound) {
				continue // deleted, or nothing was there
			}
			// Deletion failed. If the entry is still *readable*, the reset did
			// not really happen and the user must know. If it is not, the
			// keyring is unreachable and holds nothing the next run can load,
			// so drop it from the picture and clear the file copies below.
			if v, gerr := s.keyring.Get(ServiceName, s.account(kind)); gerr == nil && v != "" {
				problems = append(problems, fmt.Sprintf("%s: still readable in the keyring (%v)", s.account(kind), err))
			} else if s.disableKeyring(err) {
				s.warnf("kr-off", "cannot reach the OS keyring (%v); cleared file-stored OAuth material in %s",
					err, s.file.dir)
			}
		}
	}
	for _, dir := range []*FileTokenStore{s.file, s.legacy} {
		for _, kind := range kinds {
			if err := os.Remove(dir.path(kind)); err != nil && !os.IsNotExist(err) {
				problems = append(problems, err.Error())
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("failed to clear OAuth material: %s", strings.Join(problems, "; "))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Cached non-interactive grants
// ---------------------------------------------------------------------------

// LoadGrant returns a cached token for a named grant (e.g. client_credentials)
// when one exists and is still usable. Expiry is checked here because a grant
// has no refresh path: mcp-go's refresh logic only sees interactive tokens.
//
// A token within expirySafety of its expiry is treated as unusable so a
// long-running command never starts with a bearer that dies mid-flight.
func (s *SecretStore) LoadGrant(grant string) (*mcptransport.Token, bool) {
	raw, err := s.get(grantKind(grant))
	if err != nil {
		return nil, false
	}
	var tok mcptransport.Token
	if err := json.Unmarshal(raw, &tok); err != nil || tok.AccessToken == "" {
		return nil, false
	}
	if !tok.ExpiresAt.IsZero() && time.Now().Add(expirySafety).After(tok.ExpiresAt) {
		return nil, false
	}
	return &tok, true
}

// SaveGrant caches a token for a named grant.
func (s *SecretStore) SaveGrant(grant string, t *mcptransport.Token) error {
	if t == nil || t.AccessToken == "" {
		return errors.New("oauth: refusing to save an empty grant token")
	}
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("encoding grant token: %w", err)
	}
	return s.put(grantKind(grant), data)
}

// ---------------------------------------------------------------------------
// Introspection
// ---------------------------------------------------------------------------

// Backends a stored entry can live in.
const (
	LocKeyring = "keyring"
	LocFile    = "file"
	LocLegacy  = "cache-file"
	LocNone    = ""
)

// peek returns a payload and where it was found, without migrating or warning.
// It is the read-only counterpart of get, used by Status. A keyring error is
// returned as a note rather than aborting the lookup, so a locked keyring still
// reports whatever the file fallback holds instead of claiming "nothing here".
func (s *SecretStore) peek(kind string) (data []byte, loc string, note error) {
	if s.keyringEnabled() {
		v, kerr := s.keyring.Get(ServiceName, s.account(kind))
		switch {
		case kerr == nil && v != "":
			return []byte(v), LocKeyring, nil
		case kerr != nil && !errors.Is(kerr, keyring.ErrNotFound):
			note = fmt.Errorf("keyring unavailable (%v)", kerr)
		}
	}
	if data, err := s.file.read(kind); err == nil && len(data) > 0 {
		return data, LocFile, note
	}
	if data, err := s.legacy.read(kind); err == nil && len(data) > 0 {
		return data, LocLegacy, note
	}
	return nil, LocNone, note
}

// TokenStatus reports what is persisted for one server so users can tell
// whether their tokens survive a reboot, and where to look if they do not.
type TokenStatus struct {
	TokenLocation  string    `json:"token_location"`  // keyring | file | cache-file | ""
	ClientLocation string    `json:"client_location"` // keyring | file | cache-file | ""
	HasToken       bool      `json:"has_token"`
	HasRefresh     bool      `json:"has_refresh_token"`
	Expired        bool      `json:"expired"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	Scope          string    `json:"scope,omitempty"`
	ClientID       string    `json:"client_id,omitempty"`
	GrantLocation  string    `json:"grant_location,omitempty"`   // cached client_credentials token
	GrantExpiresAt time.Time `json:"grant_expires_at,omitempty"` //
	KeyringService string    `json:"keyring_service,omitempty"`
	KeyringAccount string    `json:"keyring_account,omitempty"`
	FileDir        string    `json:"file_dir"`
	Error          string    `json:"error,omitempty"`
}

// Status inspects stored material for this server. It never writes.
func (s *SecretStore) Status() TokenStatus {
	st := TokenStatus{
		KeyringService: ServiceName,
		KeyringAccount: s.hash + ":" + kindToken,
		FileDir:        s.file.dir,
	}
	// A keyring problem is reported but never stops the report: users need to
	// see what the file fallback holds precisely when the keyring is broken.
	note := s.keyringIssue()
	raw, loc, pnote := s.peek(kindToken)
	if note == nil {
		note = pnote
	}
	st.TokenLocation = loc
	if len(raw) > 0 {
		var tok mcptransport.Token
		if err := json.Unmarshal(raw, &tok); err != nil {
			st.Error = fmt.Sprintf("stored token is unreadable: %v", err)
		} else {
			st.HasToken = tok.AccessToken != ""
			st.HasRefresh = tok.RefreshToken != ""
			st.ExpiresAt = tok.ExpiresAt
			st.Expired = !tok.ExpiresAt.IsZero() && time.Now().After(tok.ExpiresAt)
			st.Scope = tok.Scope
		}
	}
	if raw, cloc, cerr := s.peek(kindClient); cerr != nil && st.Error == "" {
		note = cerr
	} else if len(raw) > 0 {
		var ci ClientInfo
		if json.Unmarshal(raw, &ci) == nil {
			st.ClientID = ci.ClientID
			st.ClientLocation = cloc
		}
	}
	if raw, gloc, gerr := s.peek(grantKind(GrantClientCredentials)); gerr == nil && len(raw) > 0 {
		var gt mcptransport.Token
		if json.Unmarshal(raw, &gt) == nil {
			st.GrantLocation = gloc
			st.GrantExpiresAt = gt.ExpiresAt
		}
	}
	if note != nil && st.Error == "" {
		st.Error = note.Error()
	}
	return st
}
