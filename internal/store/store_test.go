package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testFile(name, content string) File {
	return File{Name: name, Reader: bytes.NewBufferString(content), ContentType: "text/plain; charset=utf-8"}
}

var testScope = Scope{OwnerID: "u_test"}

func testOptions(opts Options) Options {
	opts.OwnerID = testScope.OwnerID
	return opts
}

func TestStoreLifecycle(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := s.Create(testOptions(Options{Title: "demo", Entry: "index.html"}), []File{testFile("index.html", "hello"), testFile("img/note.txt", "note")})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.ID) != 26 {
		t.Fatalf("ID length = %d, want 26", len(meta.ID))
	}
	if meta.Bytes != 9 || len(meta.Files) != 2 {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	wantHash := sha256.Sum256([]byte("hello"))
	if meta.Files[1].SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("sha256 = %s", meta.Files[1].SHA256)
	}
	f, err := s.Open(meta.ID, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	f.Close()
	if string(got) != "hello" {
		t.Fatalf("content = %q", got)
	}
	listed, err := s.List(testScope)
	if err != nil || len(listed) != 1 {
		t.Fatalf("List() = %#v, %v", listed, err)
	}
	if err := s.Delete(meta.ID, testScope); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(meta.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}

func TestUpdateMergeAndReplace(t *testing.T) {
	s, _ := New(t.TempDir())
	meta, err := s.Create(testOptions(Options{}), []File{testFile("a.txt", "a"), testFile("b.txt", "b")})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := s.Update(meta.ID, testScope, Options{}, []File{testFile("a.txt", "new")}, "merge")
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Files) != 2 || merged.Bytes != 4 {
		t.Fatalf("merge metadata = %#v", merged)
	}
	expires := time.Now().Add(time.Hour)
	if _, err := s.Update(meta.ID, testScope, Options{ExpiresAt: &expires, ExpiresSet: true}, []File{testFile("a.txt", "new")}, "merge"); err != nil {
		t.Fatal(err)
	}
	cleared, err := s.Update(meta.ID, testScope, Options{ExpiresSet: true}, []File{testFile("a.txt", "new")}, "merge")
	if err != nil || cleared.ExpiresAt != nil {
		t.Fatalf("clear expiry = %#v, %v", cleared, err)
	}
	replaced, err := s.Update(meta.ID, testScope, Options{}, []File{testFile("c.md", "# c")}, "replace")
	if err != nil {
		t.Fatal(err)
	}
	if len(replaced.Files) != 1 || replaced.Entry != "c.md" {
		t.Fatalf("replace metadata = %#v", replaced)
	}
	if _, err := s.Open(meta.ID, "b.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old file open = %v", err)
	}
}

func TestRejectsUnsafePathsAndSymlinkEscape(t *testing.T) {
	s, _ := New(t.TempDir())
	for _, name := range []string{"../secret", "/etc/passwd", "a/../../secret", `a\b`} {
		if _, err := s.Create(testOptions(Options{}), []File{testFile(name, "x")}); err == nil {
			t.Errorf("Create accepted %q", name)
		}
	}
	meta, err := s.Create(testOptions(Options{}), []File{testFile("safe.txt", "ok")})
	if err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(s.bundlesDir, meta.ID, "files", "safe.txt")
	if err := os.Remove(stored); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", stored); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(meta.ID, "safe.txt"); err == nil {
		t.Fatal("Open followed a symlink outside the bundle")
	}
}

func TestExpiryAndSweep(t *testing.T) {
	s, _ := New(t.TempDir())
	past := time.Now().Add(-time.Minute)
	meta, err := s.Create(testOptions(Options{ExpiresAt: &past, ExpiresSet: true}), []File{testFile("x.txt", "x")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(meta.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("Get = %v, want ErrExpired", err)
	}
	removed, err := s.SweepExpired()
	if err != nil || removed != 1 {
		t.Fatalf("SweepExpired = %d, %v", removed, err)
	}
	if _, err := os.Stat(s.bundleDir(meta.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bundle remains: %v", err)
	}
}

func TestOwnershipAndLegacyAdoption(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	alice := Scope{OwnerID: "u_alice"}
	bob := Scope{OwnerID: "u_bob"}
	meta, err := s.Create(Options{OwnerID: alice.OwnerID}, []File{testFile("x.txt", "x")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(meta.ID, bob, Options{}, []File{testFile("x.txt", "bob")}, "replace"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Bob Update = %v", err)
	}
	if err := s.Delete(meta.ID, bob); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Bob Delete = %v", err)
	}
	updated, err := s.Update(meta.ID, alice, Options{OwnerID: bob.OwnerID}, []File{testFile("x.txt", "alice")}, "replace")
	if err != nil {
		t.Fatal(err)
	}
	if updated.OwnerID != alice.OwnerID {
		t.Fatalf("owner changed to %q", updated.OwnerID)
	}
	listed, err := s.List(bob)
	if err != nil || len(listed) != 0 {
		t.Fatalf("Bob List = %#v, %v", listed, err)
	}

	// Simulate metadata written by a pre-multi-user release.
	updated.OwnerID = ""
	if err := s.writeMeta(updated); err != nil {
		t.Fatal(err)
	}
	if count, err := s.AdoptUnownedBundles("u_admin"); err != nil || count != 1 {
		t.Fatalf("AdoptUnownedBundles = %d, %v", count, err)
	}
	if count, err := s.AdoptUnownedBundles("u_other"); err != nil || count != 0 {
		t.Fatalf("second adoption = %d, %v", count, err)
	}
	adopted, err := s.List(Scope{OwnerID: "u_admin"})
	if err != nil || len(adopted) != 1 || adopted[0].ID != meta.ID {
		t.Fatalf("adopted meta = %#v, %v", adopted, err)
	}
}

// Expired bundles still belong to somebody: teardown and revocation paths must
// see them even though List hides them.
func TestOwnedTeardownIncludesExpiredBundles(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Minute)
	meta, err := s.Create(Options{OwnerID: "u_alice", ExpiresAt: &past, ExpiresSet: true}, []File{testFile("x.txt", "x")})
	if err != nil {
		t.Fatal(err)
	}
	if listed, err := s.List(Scope{OwnerID: "u_alice"}); err != nil || len(listed) != 0 {
		t.Fatalf("List = %#v, %v", listed, err)
	}
	if count, err := s.CountOwned("u_alice"); err != nil || count != 1 {
		t.Fatalf("CountOwned = %d, %v", count, err)
	}
	if err := s.Delete(meta.ID, Scope{OwnerID: "u_bob"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Bob Delete expired = %v", err)
	}
	if err := s.Delete(meta.ID, Scope{OwnerID: "u_alice"}); err != nil {
		t.Fatalf("owner Delete expired = %v", err)
	}
	if _, err := s.Get(meta.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bundle remains: %v", err)
	}
}
