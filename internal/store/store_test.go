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

func TestStoreLifecycle(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := s.Create(Options{Title: "demo", Entry: "index.html"}, []File{testFile("index.html", "hello"), testFile("img/note.txt", "note")})
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
	listed, err := s.List()
	if err != nil || len(listed) != 1 {
		t.Fatalf("List() = %#v, %v", listed, err)
	}
	if err := s.Delete(meta.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(meta.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}

func TestUpdateMergeAndReplace(t *testing.T) {
	s, _ := New(t.TempDir())
	meta, err := s.Create(Options{}, []File{testFile("a.txt", "a"), testFile("b.txt", "b")})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := s.Update(meta.ID, Options{}, []File{testFile("a.txt", "new")}, "merge")
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Files) != 2 || merged.Bytes != 4 {
		t.Fatalf("merge metadata = %#v", merged)
	}
	expires := time.Now().Add(time.Hour)
	if _, err := s.Update(meta.ID, Options{ExpiresAt: &expires, ExpiresSet: true}, []File{testFile("a.txt", "new")}, "merge"); err != nil {
		t.Fatal(err)
	}
	cleared, err := s.Update(meta.ID, Options{ExpiresSet: true}, []File{testFile("a.txt", "new")}, "merge")
	if err != nil || cleared.ExpiresAt != nil {
		t.Fatalf("clear expiry = %#v, %v", cleared, err)
	}
	replaced, err := s.Update(meta.ID, Options{}, []File{testFile("c.md", "# c")}, "replace")
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
		if _, err := s.Create(Options{}, []File{testFile(name, "x")}); err == nil {
			t.Errorf("Create accepted %q", name)
		}
	}
	meta, err := s.Create(Options{}, []File{testFile("safe.txt", "ok")})
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
	meta, err := s.Create(Options{ExpiresAt: &past, ExpiresSet: true}, []File{testFile("x.txt", "x")})
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
