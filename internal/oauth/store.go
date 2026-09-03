package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
)

// Kinds are the file/keyring entry names this package persists. A kind maps to
// <dir>/<kind>.json on disk and to "<hash>:<kind>" in the OS keyring.
const (
	kindToken  = "token"
	kindClient = "client"

	// GrantClientCredentials names the cached token for a client_credentials
	// grant, which is re-minted rather than refreshed.
	GrantClientCredentials = "client_credentials"
)

// grantKind namespaces cached tokens for non-interactive grants so they never
// collide with an interactive authorization-code token for the same server.
func grantKind(grant string) string { return "token:" + grant }

// ClientInfo holds the OAuth client credentials for a server. Dynamic client
// registration mints a fresh client_id (and sometimes a client_secret) per
// run, so both must survive across invocations: a refresh token is only valid
// for the client it was issued to.
type ClientInfo struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// FileTokenStore persists OAuth tokens and client-info files on disk with
// 0600 permissions. It is the fallback backend for SecretStore on hosts
// without a usable keyring, and the migration source for pre-keyring data.
type FileTokenStore struct {
	dir string
}

// NewFileTokenStore returns a store rooted at dir. The directory is created on
// first write so that read-only or legacy locations can be opened safely.
// An empty dir yields an inert store: every read misses and every write is a
// no-op, which lets callers pass an optional legacy location.
func NewFileTokenStore(dir string) *FileTokenStore {
	return &FileTokenStore{dir: dir}
}

// inert reports whether this store has no backing directory.
func (s *FileTokenStore) inert() bool { return s.dir == "" }

func (s *FileTokenStore) path(kind string) string { return filepath.Join(s.dir, kind+".json") }

// read returns the raw JSON for a kind, or an error if it is absent.
func (s *FileTokenStore) read(kind string) ([]byte, error) {
	if s.inert() {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(s.path(kind))
}

// write stores raw JSON for a kind, creating the directory with 0700.
func (s *FileTokenStore) write(kind string, data []byte) error {
	if s.inert() {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path(kind), data, 0o600)
}

// remove deletes a kind's file, ignoring "not exist".
func (s *FileTokenStore) remove(kind string) {
	if s.inert() {
		return
	}
	os.Remove(s.path(kind))
}

// GetToken reads the persisted token from disk.
func (s *FileTokenStore) GetToken(ctx context.Context) (*mcptransport.Token, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	data, err := s.read(kindToken)
	if err != nil {
		return nil, mcptransport.ErrNoToken
	}
	var tok mcptransport.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, mcptransport.ErrNoToken
	}
	return &tok, nil
}

// SaveToken persists the token to disk with 0600 permissions.
func (s *FileTokenStore) SaveToken(ctx context.Context, t *mcptransport.Token) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return s.write(kindToken, data)
}

// SaveClientInfo persists the client credentials (from DCR or flags).
//
// An empty client_id is rejected here, the single write path, so no caller can
// clobber working credentials with a meaningless empty value.
func (s *FileTokenStore) SaveClientInfo(ci ClientInfo) error {
	if ci.ClientID == "" {
		return errors.New("oauth: refusing to save empty client_id")
	}
	data, err := json.MarshalIndent(ci, "", "  ")
	if err != nil {
		return err
	}
	return s.write(kindClient, data)
}

// LoadClientInfo returns the persisted client credentials, or (zero, false) on
// any error.
func (s *FileTokenStore) LoadClientInfo() (ClientInfo, bool) {
	data, err := s.read(kindClient)
	if err != nil {
		return ClientInfo{}, false
	}
	var ci ClientInfo
	if err := json.Unmarshal(data, &ci); err != nil {
		return ClientInfo{}, false
	}
	if ci.ClientID == "" {
		return ClientInfo{}, false
	}
	return ci, true
}
