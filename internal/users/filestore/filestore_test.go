package filestore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/readeem/hostebin/internal/users"
	"github.com/readeem/hostebin/internal/users/filestore"
)

func TestBootstrapPersistenceAndRevocation(t *testing.T) {
	dataDir := t.TempDir()
	store, err := filestore.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	service := users.NewService(store)
	adminID, generated, err := service.Bootstrap(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if adminID == "" || !strings.HasPrefix(generated, "hbt_") {
		t.Fatalf("bootstrap = %q, %q", adminID, generated)
	}
	bytes, err := os.ReadFile(filepath.Join(dataDir, "users.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), generated) {
		t.Fatal("users.json contains the plaintext token")
	}
	if strings.Contains(string(bytes), `"tokens"`) || !strings.Contains(string(bytes), `"token"`) {
		t.Fatalf("users.json does not use a singular token field: %s", bytes)
	}
	info, err := os.Stat(filepath.Join(dataDir, "users.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("users.json mode = %v", info.Mode().Perm())
	}
	principal, err := service.Authenticate(context.Background(), generated)
	if err != nil || principal.UserID != adminID || !principal.IsAdmin() {
		t.Fatalf("Authenticate = %#v, %v", principal, err)
	}
	secondAdminID, secondGenerated, err := service.Bootstrap(context.Background(), "")
	if err != nil || secondAdminID != adminID || secondGenerated != "" {
		t.Fatalf("idempotent bootstrap = %q, %q, %v", secondAdminID, secondGenerated, err)
	}
	if err := service.RevokeToken(context.Background(), principal.UserID); !errors.Is(err, users.ErrLastAdminToken) {
		t.Fatalf("revoke last admin token = %v", err)
	}

	user, _, userToken, err := service.CreateUser(context.Background(), "Alice", users.RoleUser, "", 0)
	if err != nil || user.Name != "alice" {
		t.Fatalf("CreateUser = %#v, %v", user, err)
	}
	userPrincipal, err := service.Authenticate(context.Background(), userToken)
	if err != nil || userPrincipal.UserID != user.ID {
		t.Fatalf("user Authenticate = %#v, %v", userPrincipal, err)
	}
	if err := service.SetDisabled(context.Background(), user.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), userToken); !errors.Is(err, users.ErrUnauthorized) {
		t.Fatalf("disabled Authenticate = %v", err)
	}
	if err := service.SetDisabled(context.Background(), user.ID, false); err != nil {
		t.Fatal(err)
	}
	rotated, rotatedSecret, err := service.RotateToken(context.Background(), user.ID, "replacement", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), userToken); !errors.Is(err, users.ErrUnauthorized) {
		t.Fatalf("replaced token Authenticate = %v", err)
	}
	userPrincipal, err = service.Authenticate(context.Background(), rotatedSecret)
	if err != nil || userPrincipal.TokenID != rotated.ID {
		t.Fatalf("replacement Authenticate = %#v, %v", userPrincipal, err)
	}
	storedToken, err := service.GetToken(context.Background(), user.ID)
	if err != nil || storedToken.ID != rotated.ID {
		t.Fatalf("token after rotation = %#v, %v", storedToken, err)
	}
	if err := service.RevokeToken(context.Background(), userPrincipal.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), rotatedSecret); !errors.Is(err, users.ErrUnauthorized) {
		t.Fatalf("revoked Authenticate = %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := filestore.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, _, err := reopened.LookupToken(context.Background(), users.HashToken(generated)); err != nil {
		t.Fatalf("bootstrap token after reopen = %v", err)
	}
}

func TestOpenRejectsPluralTokensField(t *testing.T) {
	dataDir := t.TempDir()
	fixture := `{"version":1,"users":[{"id":"u_bad","name":"bad","role":"user","created_at":"2026-08-06T00:00:00Z","disabled":false,"tokens":[]}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "users.json"), []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := filestore.Open(dataDir); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Open plural token field = %v", err)
	}
}

func TestConfiguredBootstrapTokenAlwaysBelongsToAdmin(t *testing.T) {
	store, err := filestore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := users.NewService(store)
	adminID, _, err := service.Bootstrap(context.Background(), "old-configured-token")
	if err != nil {
		t.Fatal(err)
	}
	_, _, bobToken, err := service.CreateUser(context.Background(), "bob", users.RoleUser, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	gotAdminID, _, err := service.Bootstrap(context.Background(), bobToken)
	if err != nil || gotAdminID != adminID {
		t.Fatalf("configured bootstrap = %q, %v", gotAdminID, err)
	}
	principal, err := service.Authenticate(context.Background(), bobToken)
	if err != nil || !principal.IsAdmin() || principal.UserID != adminID {
		t.Fatalf("configured token principal = %#v, %v", principal, err)
	}
	if _, err := service.Authenticate(context.Background(), "old-configured-token"); !errors.Is(err, users.ErrUnauthorized) {
		t.Fatalf("old configured token = %v", err)
	}
}

func TestAdminPoliciesCannotOrphanDeployment(t *testing.T) {
	store, err := filestore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := users.NewService(store)
	_, _, err = service.Bootstrap(context.Background(), "first-admin-token")
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Authenticate(context.Background(), "first-admin-token")
	if err != nil {
		t.Fatal(err)
	}
	second, _, _, err := service.CreateUser(context.Background(), "second-admin", users.RoleAdmin, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeToken(context.Background(), first.UserID); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteUser(context.Background(), second.ID); !errors.Is(err, users.ErrLastAdmin) {
		t.Fatalf("delete only reachable admin = %v", err)
	}
	if err := service.SetDisabled(context.Background(), second.ID, true); !errors.Is(err, users.ErrLastAdmin) {
		t.Fatalf("disable only reachable admin = %v", err)
	}
}

func TestConcurrentAdminRevocationKeepsOneToken(t *testing.T) {
	store, err := filestore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := users.NewService(store)
	_, _, err = service.Bootstrap(context.Background(), "first-token")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := service.Authenticate(context.Background(), "first-token")
	_, _, secondSecret, err := service.CreateUser(context.Background(), "admin2", users.RoleAdmin, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := service.Authenticate(context.Background(), secondSecret)

	errs := make(chan error, 2)
	go func() { errs <- service.RevokeToken(context.Background(), first.UserID) }()
	go func() { errs <- service.RevokeToken(context.Background(), second.UserID) }()
	one, two := <-errs, <-errs
	failed := 0
	for _, err := range []error{one, two} {
		if errors.Is(err, users.ErrLastAdminToken) {
			failed++
		} else if err != nil {
			t.Fatalf("unexpected revoke error: %v", err)
		}
	}
	if failed != 1 {
		t.Fatalf("last-token failures = %d, want 1 (%v, %v)", failed, one, two)
	}
}

func TestExpiredTokenAndDuplicateNames(t *testing.T) {
	store, err := filestore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := users.NewService(store)
	if _, _, err := service.Bootstrap(context.Background(), "legacy-token"); err != nil {
		t.Fatal(err)
	}
	user, _, _, err := service.CreateUser(context.Background(), "bob", users.RoleUser, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.CreateUser(context.Background(), "BOB", users.RoleUser, "", 0); !errors.Is(err, users.ErrDuplicateName) {
		t.Fatalf("duplicate name = %v", err)
	}
	past := time.Now().Add(-time.Minute)
	plaintext := "hbt_expired"
	expired := users.Token{ID: "t_expired", UserID: user.ID, Label: "old", Digest: users.HashToken(plaintext), CreatedAt: past.Add(-time.Hour), ExpiresAt: &past}
	if err := store.SetToken(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), plaintext); !errors.Is(err, users.ErrUnauthorized) {
		t.Fatalf("expired Authenticate = %v", err)
	}
}
