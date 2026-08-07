package filestore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/readeem/hostebin/internal/fsutil"
	"github.com/readeem/hostebin/internal/users"
)

type diskFile struct {
	Version int        `json:"version"`
	Users   []diskUser `json:"users"`
}

type diskUser struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Role      users.Role `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
	Disabled  bool       `json:"disabled"`
	Token     *diskToken `json:"token,omitempty"`
}

type diskToken struct {
	ID         string     `json:"id"`
	Hash       string     `json:"hash"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type tokenRef struct{ user int }

type Store struct {
	mu      sync.RWMutex
	path    string
	dir     string
	data    diskFile
	digests map[users.Digest]tokenRef
}

func Open(dataDir string) (*Store, error) {
	if dataDir == "" {
		return nil, errors.New("data directory is required")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: abs, path: filepath.Join(abs, "users.json"), data: diskFile{Version: 1}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.digests = make(map[users.Digest]tokenRef)
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s.data); err != nil {
		return nil, fmt.Errorf("decode users.json: %w", err)
	}
	if s.data.Version != 1 {
		return nil, fmt.Errorf("unsupported users.json version %d", s.data.Version)
	}
	if s.digests, err = index(s.data); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return nil }

// index validates data and returns its token lookup table. It never mutates the
// receiver, so a rejected candidate cannot leave the store half-indexed.
func index(data diskFile) (map[users.Digest]tokenRef, error) {
	digests := make(map[users.Digest]tokenRef)
	names := make(map[string]bool)
	userIDs := make(map[string]bool)
	tokenIDs := make(map[string]bool)
	for ui := range data.Users {
		u := &data.Users[ui]
		name := strings.ToLower(u.Name)
		if names[name] {
			return nil, users.ErrDuplicateName
		}
		if userIDs[u.ID] {
			return nil, fmt.Errorf("duplicate user id %q", u.ID)
		}
		names[name], userIDs[u.ID] = true, true
		if u.Token != nil {
			t := u.Token
			if tokenIDs[t.ID] {
				return nil, fmt.Errorf("duplicate token id %q", t.ID)
			}
			d, err := users.ParseDigest(t.Hash)
			if err != nil {
				return nil, fmt.Errorf("token %s: %w", t.ID, err)
			}
			if _, exists := digests[d]; exists {
				return nil, users.ErrDuplicateToken
			}
			tokenIDs[t.ID] = true
			digests[d] = tokenRef{user: ui}
		}
	}
	return digests, nil
}

func clone(in diskFile) diskFile {
	out := in
	out.Users = make([]diskUser, len(in.Users))
	for i := range in.Users {
		out.Users[i] = in.Users[i]
		if in.Users[i].Token != nil {
			token := *in.Users[i].Token
			out.Users[i].Token = &token
		}
	}
	return out
}

// mutate applies fn to a copy of the current state and commits it only once the
// result both indexes cleanly and reaches disk, so a rejected or failed write
// leaves the in-memory store untouched.
func (s *Store) mutate(fn func(*diskFile) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.data)
	if err := fn(&next); err != nil {
		return err
	}
	digests, err := index(next)
	if err != nil {
		return err
	}
	if err := s.write(next); err != nil {
		return err
	}
	s.data, s.digests = next, digests
	return nil
}

func (s *Store) write(data diskFile) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(b, '\n'))
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return fsutil.SyncDir(s.dir)
}

func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func userFromDisk(u diskUser) users.User {
	return users.User{ID: u.ID, Name: u.Name, Role: u.Role, CreatedAt: u.CreatedAt, Disabled: u.Disabled}
}

func tokenFromDisk(userID string, t diskToken) (users.Token, error) {
	d, err := users.ParseDigest(t.Hash)
	if err != nil {
		return users.Token{}, err
	}
	return users.Token{ID: t.ID, UserID: userID, Label: t.Label, Digest: d, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt}, nil
}

func diskFromUser(u users.User) diskUser {
	return diskUser{ID: u.ID, Name: u.Name, Role: u.Role, CreatedAt: u.CreatedAt, Disabled: u.Disabled}
}

func diskFromToken(t users.Token) diskToken {
	return diskToken{ID: t.ID, Hash: t.Digest.String(), Label: t.Label, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt}
}

func findUser(data *diskFile, id string) int {
	return slices.IndexFunc(data.Users, func(u diskUser) bool { return u.ID == id })
}

func addUser(data *diskFile, user users.User) error {
	for _, u := range data.Users {
		if strings.EqualFold(u.Name, user.Name) {
			return users.ErrDuplicateName
		}
		if u.ID == user.ID {
			return fmt.Errorf("duplicate user id %q", user.ID)
		}
	}
	data.Users = append(data.Users, diskFromUser(user))
	return nil
}

func (s *Store) GetUser(ctx context.Context, id string) (users.User, error) {
	if err := checkContext(ctx); err != nil {
		return users.User{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	i := findUser(&s.data, id)
	if i < 0 {
		return users.User{}, users.ErrUserNotFound
	}
	return userFromDisk(s.data.Users[i]), nil
}

func (s *Store) ListUsers(ctx context.Context) ([]users.User, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]users.User, 0, len(s.data.Users))
	for _, u := range s.data.Users {
		out = append(out, userFromDisk(u))
	}
	slices.SortFunc(out, func(a, b users.User) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (s *Store) UpdateUser(ctx context.Context, user users.User) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return s.mutate(func(data *diskFile) error {
		i := findUser(data, user.ID)
		if i < 0 {
			return users.ErrUserNotFound
		}
		for j, u := range data.Users {
			if j != i && strings.EqualFold(u.Name, user.Name) {
				return users.ErrDuplicateName
			}
		}
		token := data.Users[i].Token
		data.Users[i] = diskFromUser(user)
		data.Users[i].Token = token
		return nil
	})
}

func setToken(data *diskFile, token users.Token) error {
	i := findUser(data, token.UserID)
	if i < 0 {
		return users.ErrUserNotFound
	}
	for ui, user := range data.Users {
		if ui == i || user.Token == nil {
			continue
		}
		if user.Token.ID == token.ID {
			return fmt.Errorf("duplicate token id %q", token.ID)
		}
		if user.Token.Hash == token.Digest.String() {
			return users.ErrDuplicateToken
		}
	}
	diskToken := diskFromToken(token)
	data.Users[i].Token = &diskToken
	return nil
}

func (s *Store) SetToken(ctx context.Context, token users.Token) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return s.mutate(func(data *diskFile) error { return setToken(data, token) })
}

func (s *Store) GetTokenForUser(ctx context.Context, userID string) (users.Token, error) {
	if err := checkContext(ctx); err != nil {
		return users.Token{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	i := findUser(&s.data, userID)
	if i < 0 {
		return users.Token{}, users.ErrUserNotFound
	}
	if s.data.Users[i].Token == nil {
		return users.Token{}, users.ErrTokenNotFound
	}
	return tokenFromDisk(userID, *s.data.Users[i].Token)
}

func (s *Store) DeleteTokenForUser(ctx context.Context, userID string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return s.mutate(func(data *diskFile) error {
		i := findUser(data, userID)
		if i < 0 {
			return users.ErrUserNotFound
		}
		if data.Users[i].Token == nil {
			return users.ErrTokenNotFound
		}
		data.Users[i].Token = nil
		return nil
	})
}

func (s *Store) LookupToken(ctx context.Context, digest users.Digest) (users.Token, users.User, error) {
	if err := checkContext(ctx); err != nil {
		return users.Token{}, users.User{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ref, ok := s.digests[digest]
	if !ok {
		return users.Token{}, users.User{}, users.ErrTokenNotFound
	}
	u := s.data.Users[ref.user]
	if u.Token == nil {
		return users.Token{}, users.User{}, users.ErrTokenNotFound
	}
	t, err := tokenFromDisk(u.ID, *u.Token)
	if err != nil {
		return users.Token{}, users.User{}, err
	}
	return t, userFromDisk(u), nil
}

// TouchToken deliberately remains a no-op in the v1 file backend. Persisting
// every request would turn last-used metadata into the storage hot path.
func (s *Store) TouchToken(context.Context, string, time.Time) error { return nil }

func (s *Store) CreateUserWithToken(ctx context.Context, user users.User, token users.Token) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if token.UserID != user.ID {
		return errors.New("token user does not match created user")
	}
	return s.mutate(func(data *diskFile) error {
		if err := addUser(data, user); err != nil {
			return err
		}
		return setToken(data, token)
	})
}

func (s *Store) DeleteUserWithToken(ctx context.Context, userID string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return s.mutate(func(data *diskFile) error {
		i := findUser(data, userID)
		if i < 0 {
			return users.ErrUserNotFound
		}
		data.Users = slices.Delete(data.Users, i, i+1)
		return nil
	})
}
