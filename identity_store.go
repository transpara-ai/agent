package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// InMemoryIdentityStore is a process-local IdentityStore for tests and local
// harnesses. It is not durable across process restarts.
type InMemoryIdentityStore struct {
	mu         sync.RWMutex
	identities map[string]PersistentIdentity
}

func NewInMemoryIdentityStore() *InMemoryIdentityStore {
	return &InMemoryIdentityStore{identities: map[string]PersistentIdentity{}}
}

func (s *InMemoryIdentityStore) LoadIdentity(_ context.Context, ref string) (PersistentIdentity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	identity, ok := s.identities[ref]
	if !ok {
		return PersistentIdentity{}, ErrIdentityNotFound
	}
	return clonePersistentIdentity(identity), nil
}

func (s *InMemoryIdentityStore) SaveIdentity(_ context.Context, identity PersistentIdentity) error {
	if identity.Ref == "" {
		return ErrIdentityRefRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identities[identity.Ref] = clonePersistentIdentity(identity)
	return nil
}

// FileIdentityStore stores identities as one JSON file per ref under a local
// directory. It is intentionally local-only; cloud KMS and secret managers are
// outside this package boundary.
type FileIdentityStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileIdentityStore(dir string) *FileIdentityStore {
	return &FileIdentityStore{dir: dir}
}

func (s *FileIdentityStore) LoadIdentity(_ context.Context, ref string) (PersistentIdentity, error) {
	path, err := s.path(ref)
	if err != nil {
		return PersistentIdentity{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PersistentIdentity{}, ErrIdentityNotFound
		}
		return PersistentIdentity{}, err
	}
	var identity PersistentIdentity
	if err := json.Unmarshal(b, &identity); err != nil {
		return PersistentIdentity{}, fmt.Errorf("agent: decode persistent identity %s: %w", ref, err)
	}
	return identity, nil
}

func (s *FileIdentityStore) SaveIdentity(_ context.Context, identity PersistentIdentity) error {
	if identity.Ref == "" {
		return ErrIdentityRefRequired
	}
	path, err := s.path(identity.Ref)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("agent: encode persistent identity %s: %w", identity.Ref, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *FileIdentityStore) path(ref string) (string, error) {
	if s.dir == "" {
		return "", fmt.Errorf("agent: identity store directory is required")
	}
	if ref == "" {
		return "", ErrIdentityRefRequired
	}
	if strings.Contains(ref, "/") || strings.Contains(ref, "\\") || ref == "." || ref == ".." {
		return "", fmt.Errorf("agent: invalid persistent identity ref %q", ref)
	}
	return filepath.Join(s.dir, ref+".json"), nil
}

func clonePersistentIdentity(identity PersistentIdentity) PersistentIdentity {
	clone := identity
	clone.PrivateKey = append([]byte(nil), identity.PrivateKey...)
	clone.PublicKey = append([]byte(nil), identity.PublicKey...)
	clone.PreviousPublicKeys = append([]string(nil), identity.PreviousPublicKeys...)
	return clone
}
