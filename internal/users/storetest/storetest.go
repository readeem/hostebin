// Package storetest contains the acceptance contract shared by user stores.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/readeem/hostebin/internal/users"
)

// Run exercises the observable users.Store contract. Backend packages should
// call it from a TestStoreConformance test with a fresh store factory.
func Run(t *testing.T, newStore func(*testing.T) users.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("sentinel errors and composite operations", func(t *testing.T) {
		store := newStore(t)
		defer store.Close()
		if _, err := store.GetUser(ctx, "missing"); !errors.Is(err, users.ErrUserNotFound) {
			t.Fatalf("GetUser missing = %v", err)
		}
		if _, err := store.GetTokenForUser(ctx, "missing"); !errors.Is(err, users.ErrUserNotFound) {
			t.Fatalf("GetTokenForUser missing = %v", err)
		}
		now := time.Now().UTC()
		user := users.User{ID: "u_one", Name: "one", Role: users.RoleAdmin, CreatedAt: now}
		token := users.Token{ID: "t_one", UserID: user.ID, Label: "initial", Digest: users.HashToken("secret"), CreatedAt: now}
		if err := store.CreateUserWithToken(ctx, user, token); err != nil {
			t.Fatal(err)
		}
		clash := users.User{ID: "u_two", Name: "ONE", Role: users.RoleUser, CreatedAt: now}
		clashToken := users.Token{ID: "t_clash", UserID: clash.ID, Label: "clash", Digest: users.HashToken("clash"), CreatedAt: now}
		if err := store.CreateUserWithToken(ctx, clash, clashToken); !errors.Is(err, users.ErrDuplicateName) {
			t.Fatalf("duplicate name = %v", err)
		}
		second := users.User{ID: "u_two", Name: "two", Role: users.RoleUser, CreatedAt: now}
		duplicate := users.Token{ID: "t_two", UserID: second.ID, Label: "duplicate", Digest: token.Digest, CreatedAt: now}
		if err := store.CreateUserWithToken(ctx, second, duplicate); !errors.Is(err, users.ErrDuplicateToken) {
			t.Fatalf("composite injected failure = %v", err)
		}
		if _, err := store.GetUser(ctx, second.ID); !errors.Is(err, users.ErrUserNotFound) {
			t.Fatalf("partially-created user after composite failure = %v", err)
		}
		if _, _, err := store.LookupToken(ctx, token.Digest); err != nil {
			t.Fatal(err)
		}
		another := users.Token{ID: "t_another", UserID: user.ID, Label: "another", Digest: users.HashToken("another"), CreatedAt: now}
		if err := store.SetToken(ctx, another); err != nil {
			t.Fatalf("set replacement token = %v", err)
		}
		if _, _, err := store.LookupToken(ctx, token.Digest); !errors.Is(err, users.ErrTokenNotFound) {
			t.Fatalf("replaced token still resolves = %v", err)
		}
		token = another
		if err := store.DeleteTokenForUser(ctx, user.ID); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.LookupToken(ctx, token.Digest); !errors.Is(err, users.ErrTokenNotFound) {
			t.Fatalf("lookup immediately after revoke = %v", err)
		}
		if err := store.DeleteUserWithToken(ctx, user.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetUser(ctx, user.ID); !errors.Is(err, users.ErrUserNotFound) {
			t.Fatalf("user after composite delete = %v", err)
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		store := newStore(t)
		defer store.Close()
		now := time.Now().UTC()
		user := users.User{ID: "u_concurrent", Name: "concurrent", Role: users.RoleUser, CreatedAt: now}
		seed := users.Token{ID: "t_seed", UserID: user.ID, Label: "seed", Digest: users.HashToken("seed"), CreatedAt: now}
		if err := store.CreateUserWithToken(ctx, user, seed); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				id := fmt.Sprintf("t_%d", i)
				digest := users.HashToken(id)
				_ = store.SetToken(ctx, users.Token{ID: id, UserID: user.ID, Label: id, Digest: digest, CreatedAt: now})
				_, _, _ = store.LookupToken(ctx, digest)
				_, _ = store.GetTokenForUser(ctx, user.ID)
			}()
		}
		wg.Wait()
		token, err := store.GetTokenForUser(ctx, user.ID)
		if err != nil || token.UserID != user.ID {
			t.Fatalf("GetTokenForUser = %#v, %v", token, err)
		}
	})
}
