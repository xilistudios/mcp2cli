package oauth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
)

// FileTokenStore persists OAuth tokens and client-info files on disk.
type FileTokenStore struct {
	dir string
}

// NewFileTokenStore returns a store rooted at dir, creating it if needed.
func NewFileTokenStore(dir string) *FileTokenStore {
	os.MkdirAll(dir, 0o700)
	return &FileTokenStore{dir: dir}
}

func (s *FileTokenStore) tokenPath() string      { return filepath.Join(s.dir, "token.json") }
func (s *FileTokenStore) clientInfoPath() string { return filepath.Join(s.dir, "client_info.json") }

// GetToken reads the persisted token from disk.
func (s *FileTokenStore) GetToken(ctx context.Context) (*mcptransport.Token, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	data, err := os.ReadFile(s.tokenPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, mcptransport.ErrNoToken
		}
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
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.tokenPath(), data, 0o600)
}

// SaveClientID persists the DCR client_id to client_info.json.
func (s *FileTokenStore) SaveClientID(id string) error {
	payload := map[string]string{"client_id": id}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.clientInfoPath(), data, 0o600)
}

// LoadClientID returns the persisted client_id, or ("", false) on any error.
func (s *FileTokenStore) LoadClientID() (string, bool) {
	data, err := os.ReadFile(s.clientInfoPath())
	if err != nil {
		return "", false
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return "", false
	}
	id, ok := m["client_id"]
	if !ok || id == "" {
		return "", false
	}
	return id, true
}
