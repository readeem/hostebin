package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrTokenNotFound  = errors.New("token not found")
	ErrDuplicateName  = errors.New("user name already exists")
	ErrDuplicateToken = errors.New("token digest already exists")
	ErrUnauthorized   = errors.New("valid bearer token required")
	ErrInvalidName    = errors.New("invalid user name")
	ErrInvalidRole    = errors.New("invalid user role")
	ErrLastAdmin      = errors.New("cannot remove the last admin")
	ErrLastAdminToken = errors.New("cannot revoke the last token of the last admin")
)

type AuthError struct{ Reason string }

func (e AuthError) Error() string { return "authentication failed: " + e.Reason }
func (e AuthError) Unwrap() error { return ErrUnauthorized }

type Principal struct {
	UserID     string `json:"id"`
	Name       string `json:"name"`
	TokenID    string `json:"token_id"`
	TokenLabel string `json:"token_label,omitempty"`
	Role       Role   `json:"role"`
}

func (p Principal) IsAdmin() bool { return p.Role == RoleAdmin }

type Digest [32]byte

func (d Digest) String() string { return hex.EncodeToString(d[:]) }

func ParseDigest(raw string) (Digest, error) {
	var d Digest
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != len(d) {
		return d, errors.New("invalid token digest")
	}
	copy(d[:], b)
	return d, nil
}

func HashToken(plaintext string) Digest { return sha256.Sum256([]byte(plaintext)) }

func NewToken() (plaintext string, d Digest, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", d, err
	}
	plaintext = "hbt_" + base64.RawURLEncoding.EncodeToString(b)
	return plaintext, HashToken(plaintext), nil
}

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	Disabled  bool      `json:"disabled"`
}

type Token struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Label      string     `json:"label"`
	Digest     Digest     `json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type Store interface {
	GetUser(context.Context, string) (User, error)
	ListUsers(context.Context) ([]User, error)
	UpdateUser(context.Context, User) error

	SetToken(context.Context, Token) error
	GetTokenForUser(context.Context, string) (Token, error)
	DeleteTokenForUser(context.Context, string) error
	LookupToken(context.Context, Digest) (Token, User, error)
	TouchToken(context.Context, string, time.Time) error

	CreateUserWithToken(context.Context, User, Token) error
	DeleteUserWithToken(context.Context, string) error

	io.Closer
}

type Service struct {
	store Store
	now   func() time.Time
	mu    sync.Mutex
}

func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Authenticate(ctx context.Context, plaintext string) (Principal, error) {
	digest := HashToken(plaintext)
	token, user, err := s.store.LookupToken(ctx, digest)
	matched := subtle.ConstantTimeCompare(digest[:], token.Digest[:])
	if err != nil || matched != 1 {
		return Principal{}, AuthError{Reason: "unknown token"}
	}
	now := s.now()
	if user.Disabled {
		return Principal{}, AuthError{Reason: "disabled user"}
	}
	if token.ExpiresAt != nil && !now.Before(*token.ExpiresAt) {
		return Principal{}, AuthError{Reason: "expired token"}
	}
	_ = s.store.TouchToken(ctx, token.ID, now)
	return Principal{UserID: user.ID, Name: user.Name, Role: user.Role, TokenID: token.ID, TokenLabel: token.Label}, nil
}

// Bootstrap creates the first admin and ensures a configured or legacy token
// remains valid. generated is non-empty only when Bootstrap generated the secret.
func (s *Service) Bootstrap(ctx context.Context, plaintext string) (adminID, generated string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.store.ListUsers(ctx)
	if err != nil {
		return "", "", err
	}
	if len(all) == 0 {
		if plaintext == "" {
			plaintext, _, err = NewToken()
			if err != nil {
				return "", "", err
			}
			generated = plaintext
		}
		now := s.now()
		userID, err := newID("u_")
		if err != nil {
			return "", "", err
		}
		tokenID, err := newID("t_")
		if err != nil {
			return "", "", err
		}
		user := User{ID: userID, Name: "admin", Role: RoleAdmin, CreatedAt: now}
		token := Token{ID: tokenID, UserID: user.ID, Label: "bootstrap", Digest: HashToken(plaintext), CreatedAt: now}
		if err := s.store.CreateUserWithToken(ctx, user, token); err != nil {
			return "", "", err
		}
		return user.ID, generated, nil
	}
	var admin User
	for _, user := range all {
		if user.Role == RoleAdmin {
			admin = user
			break
		}
	}
	if admin.ID == "" {
		return "", "", ErrLastAdmin
	}
	if plaintext != "" {
		d := HashToken(plaintext)
		_, owner, lookupErr := s.store.LookupToken(ctx, d)
		if lookupErr == nil && owner.Role != RoleAdmin {
			if err := s.store.DeleteTokenForUser(ctx, owner.ID); err != nil {
				return "", "", err
			}
			lookupErr = ErrTokenNotFound
		}
		if errors.Is(lookupErr, ErrTokenNotFound) {
			tokenID, err := newID("t_")
			if err != nil {
				return "", "", err
			}
			token := Token{ID: tokenID, UserID: admin.ID, Label: "bootstrap", Digest: d, CreatedAt: s.now()}
			if err := s.store.SetToken(ctx, token); err != nil {
				return "", "", err
			}
		} else if lookupErr != nil {
			return "", "", lookupErr
		}
	}
	return admin.ID, "", nil
}

var namePattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

func normalizeName(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !namePattern.MatchString(name) {
		return "", ErrInvalidName
	}
	return name, nil
}

func validRole(role Role) bool { return role == RoleAdmin || role == RoleUser }

// CreateUser registers a user together with their first token in one atomic
// store operation and returns the token's plaintext, which is never recoverable
// afterwards. An empty label and a zero ttl mean "default label, no expiry".
func (s *Service) CreateUser(ctx context.Context, name string, role Role, label string, ttl time.Duration) (User, Token, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, err := normalizeName(name)
	if err != nil {
		return User{}, Token{}, "", err
	}
	if !validRole(role) {
		return User{}, Token{}, "", ErrInvalidRole
	}
	userID, err := newID("u_")
	if err != nil {
		return User{}, Token{}, "", err
	}
	user := User{ID: userID, Name: name, Role: role, CreatedAt: s.now()}
	token, plaintext, err := s.mintToken(userID, label, "initial", ttl)
	if err != nil {
		return User{}, Token{}, "", err
	}
	if err := s.store.CreateUserWithToken(ctx, user, token); err != nil {
		return User{}, Token{}, "", err
	}
	return user, token, plaintext, nil
}

// mintToken builds an unsaved token and its plaintext. It does not touch the
// store, so callers stay free to persist it however they need to.
func (s *Service) mintToken(userID, label, defaultLabel string, ttl time.Duration) (Token, string, error) {
	plaintext, digest, err := NewToken()
	if err != nil {
		return Token{}, "", err
	}
	tokenID, err := newID("t_")
	if err != nil {
		return Token{}, "", err
	}
	now := s.now()
	token := Token{ID: tokenID, UserID: userID, Label: strings.TrimSpace(label), Digest: digest, CreatedAt: now}
	if token.Label == "" {
		token.Label = defaultLabel
	}
	if ttl > 0 {
		expires := now.Add(ttl)
		token.ExpiresAt = &expires
	}
	return token, plaintext, nil
}

func (s *Service) DeleteUser(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateDeleteUser(ctx, id); err != nil {
		return err
	}
	return s.store.DeleteUserWithToken(ctx, id)
}

// ValidateDeleteUser performs the policy check separately so callers can
// safely resolve owned resources before committing the user deletion.
func (s *Service) ValidateDeleteUser(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.validateDeleteUser(ctx, id)
}

func (s *Service) validateDeleteUser(ctx context.Context, id string) error {
	user, err := s.store.GetUser(ctx, id)
	if err != nil {
		return err
	}
	if user.Role != RoleAdmin {
		return nil
	}
	reachable, err := s.reachableAdmins(ctx, id)
	if err != nil {
		return err
	}
	if reachable == 0 {
		return ErrLastAdmin
	}
	return nil
}

func (s *Service) SetDisabled(ctx context.Context, id string, disabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, err := s.store.GetUser(ctx, id)
	if err != nil {
		return err
	}
	if disabled && !user.Disabled && user.Role == RoleAdmin {
		reachable, err := s.reachableAdmins(ctx, id)
		if err != nil {
			return err
		}
		if reachable == 0 {
			return ErrLastAdmin
		}
	}
	user.Disabled = disabled
	return s.store.UpdateUser(ctx, user)
}

// reachableAdmins counts the admins other than excludeUserID who could still
// administer this deployment: enabled, and holding an unexpired token. Every
// policy that could lock an operator out is expressed as "would this leave zero
// reachable admins?", so the answer is computed in exactly one place.
func (s *Service) reachableAdmins(ctx context.Context, excludeUserID string) (int, error) {
	all, err := s.store.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	now := s.now()
	count := 0
	for _, user := range all {
		if user.ID == excludeUserID || user.Role != RoleAdmin || user.Disabled {
			continue
		}
		token, err := s.store.GetTokenForUser(ctx, user.ID)
		if errors.Is(err, ErrTokenNotFound) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if token.ExpiresAt == nil || now.Before(*token.ExpiresAt) {
			count++
		}
	}
	return count, nil
}

func (s *Service) RotateToken(ctx context.Context, userID, label string, ttl time.Duration) (Token, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.store.GetUser(ctx, userID); err != nil {
		return Token{}, "", err
	}
	token, plaintext, err := s.mintToken(userID, label, "token", ttl)
	if err != nil {
		return Token{}, "", err
	}
	if err := s.store.SetToken(ctx, token); err != nil {
		return Token{}, "", err
	}
	return token, plaintext, nil
}

func (s *Service) RevokeToken(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.store.GetTokenForUser(ctx, userID); err != nil {
		return err
	}
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if user.Role == RoleAdmin {
		reachable, err := s.reachableAdmins(ctx, userID)
		if err != nil {
			return err
		}
		if reachable == 0 {
			return ErrLastAdminToken
		}
	}
	return s.store.DeleteTokenForUser(ctx, userID)
}

func (s *Service) ListUsers(ctx context.Context) ([]User, error) { return s.store.ListUsers(ctx) }
func (s *Service) GetUser(ctx context.Context, id string) (User, error) {
	return s.store.GetUser(ctx, id)
}
func (s *Service) GetToken(ctx context.Context, userID string) (Token, error) {
	return s.store.GetTokenForUser(ctx, userID)
}

func newID(prefix string) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}
