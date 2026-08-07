package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/readeem/hostebin/internal/fsutil"
)

var (
	ErrNotFound = errors.New("bundle not found")
	ErrExpired  = errors.New("bundle expired")
)

type Store struct {
	dataDir    string
	bundlesDir string
}

// Scope makes ownership an explicit input to every metadata or mutation API.
// An empty OwnerID never matches; All must be requested deliberately.
type Scope struct {
	OwnerID string
	All     bool
}

func OwnedBy(ownerID string) Scope { return Scope{OwnerID: ownerID} }
func Everything() Scope            { return Scope{All: true} }

func (sc Scope) owns(meta *BundleMeta) bool {
	return sc.All || (sc.OwnerID != "" && meta.OwnerID == sc.OwnerID)
}

func New(dataDir string) (*Store, error) {
	if dataDir == "" {
		return nil, errors.New("data directory is required")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	bundles := filepath.Join(abs, "bundles")
	if err := os.MkdirAll(bundles, 0o700); err != nil {
		return nil, fmt.Errorf("create bundles directory: %w", err)
	}
	return &Store{dataDir: abs, bundlesDir: bundles}, nil
}

func (s *Store) DataDir() string { return s.dataDir }

func ValidateName(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\\') || strings.ContainsRune(name, '\x00') {
		return "", errors.New("invalid empty or backslash-containing file name")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("file names may not contain control characters")
		}
	}
	clean := path.Clean(name)
	if clean == "." || path.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe file name %q", name)
	}
	return clean, nil
}

func (s *Store) Create(opts Options, files []File) (*BundleMeta, error) {
	if opts.OwnerID == "" {
		return nil, errors.New("bundle owner is required")
	}
	for range 5 {
		id, err := NewID()
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(s.bundleDir(id)); errors.Is(err, os.ErrNotExist) {
			return s.write(id, opts, files, false, "replace")
		}
	}
	return nil, errors.New("could not allocate a unique bundle id")
}

func (s *Store) Update(id string, sc Scope, opts Options, files []File, mode string) (*BundleMeta, error) {
	if err := s.authorize(id, sc); err != nil {
		return nil, err
	}
	if mode != "merge" && mode != "replace" {
		return nil, errors.New("update mode must be merge or replace")
	}
	return s.write(id, opts, files, true, mode)
}

func (s *Store) write(id string, opts Options, files []File, updating bool, mode string) (_ *BundleMeta, retErr error) {
	if len(files) == 0 && !updating {
		return nil, errors.New("at least one file is required")
	}
	names := make(map[string]bool, len(files))
	for i := range files {
		name, err := ValidateName(files[i].Name)
		if err != nil {
			return nil, err
		}
		if names[name] {
			return nil, fmt.Errorf("duplicate file name %q", name)
		}
		names[name] = true
		files[i].Name = name
	}
	entrySet := opts.EntrySet || opts.Entry != ""
	if opts.Entry != "" {
		entry, err := ValidateName(opts.Entry)
		if err != nil {
			return nil, fmt.Errorf("invalid entry: %w", err)
		}
		opts.Entry = entry
	}
	var old *BundleMeta
	if updating {
		var err error
		old, err = s.Get(id)
		if err != nil {
			return nil, err
		}
	}
	tmp, err := os.MkdirTemp(s.bundlesDir, ".tmp-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(tmp)
		}
	}()
	if err := os.Mkdir(filepath.Join(tmp, "files"), 0o700); err != nil {
		return nil, err
	}
	meta := &BundleMeta{ID: id, OwnerID: opts.OwnerID, CreatedAt: time.Now().UTC(), ExpiresAt: opts.ExpiresAt, Title: opts.Title, Entry: opts.Entry, Uploader: opts.Uploader}
	if old != nil {
		// Ownership can never be reassigned through an upload request.
		meta.OwnerID = old.OwnerID
		meta.CreatedAt = old.CreatedAt
		if opts.Title == "" {
			meta.Title = old.Title
		}
		if mode == "merge" && !entrySet {
			meta.Entry = old.Entry
		}
		if !opts.ExpiresSet {
			meta.ExpiresAt = old.ExpiresAt
		}
		if opts.Uploader == "" {
			meta.Uploader = old.Uploader
		}
	}
	if old != nil && mode == "merge" {
		if err := copyKeptFiles(s.bundleDir(id), tmp, old.Files, names, &meta.Files); err != nil {
			return nil, err
		}
	}
	for _, file := range files {
		if err := writeFile(filepath.Join(tmp, "files"), file.Name, file.Reader, file.ContentType, &meta.Files); err != nil {
			return nil, err
		}
	}
	slices.SortFunc(meta.Files, func(a, b FileMeta) int { return strings.Compare(a.Name, b.Name) })
	for _, f := range meta.Files {
		meta.Bytes += f.Size
	}
	if meta.Entry == "" {
		meta.Entry = DetectEntry(meta.Files)
	}
	if meta.Entry != "" && !hasFile(meta.Files, meta.Entry) {
		return nil, fmt.Errorf("entry %q is not one of the uploaded files", meta.Entry)
	}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(tmp, "meta.json"), append(encoded, '\n'), 0o600); err != nil {
		return nil, err
	}
	final := s.bundleDir(id)
	if !updating {
		if err := os.Rename(tmp, final); err != nil {
			return nil, err
		}
		return meta, nil
	}
	trash := filepath.Join(s.bundlesDir, ".trash-"+id+"-"+filepath.Base(tmp))
	if err := os.Rename(final, trash); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Rename(trash, final)
		return nil, err
	}
	_ = os.RemoveAll(trash)
	return meta, nil
}

// copyKeptFiles stages the files a merge leaves untouched. It releases its
// handles on the live bundle before returning, because Windows refuses to
// rename a directory while anything inside it is still open, and the caller
// renames this bundle away immediately afterwards.
func copyKeptFiles(bundleDir, tmp string, existing []FileMeta, replaced map[string]bool, metas *[]FileMeta) error {
	root, err := os.OpenRoot(filepath.Join(bundleDir, "files"))
	if err != nil {
		return err
	}
	defer root.Close()
	for _, file := range existing {
		if replaced[file.Name] {
			continue
		}
		in, err := root.Open(file.Name)
		if err != nil {
			return fmt.Errorf("read existing %s: %w", file.Name, err)
		}
		err = writeFile(filepath.Join(tmp, "files"), file.Name, in, file.ContentType, metas)
		in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFile(root, name string, src io.Reader, contentType string, metas *[]FileMeta) error {
	dstPath := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
		return err
	}
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(dst, h), src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	*metas = append(*metas, FileMeta{Name: name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil)), ContentType: contentType})
	return nil
}

func DetectEntry(files []FileMeta) string {
	if len(files) == 1 {
		return files[0].Name
	}
	for _, preferred := range []string{"index.html", "index.htm"} {
		if hasFile(files, preferred) {
			return preferred
		}
	}
	for _, ext := range []string{".html", ".htm", ".md", ".markdown"} {
		for _, f := range files {
			if strings.EqualFold(path.Ext(f.Name), ext) {
				return f.Name
			}
		}
	}
	return ""
}

func hasFile(files []FileMeta, name string) bool {
	return slices.ContainsFunc(files, func(f FileMeta) bool { return f.Name == name })
}

func (s *Store) Get(id string) (*BundleMeta, error) {
	meta, err := s.readMeta(id)
	if err != nil {
		return nil, err
	}
	if meta.expired() {
		return nil, ErrExpired
	}
	return meta, nil
}

func (s *Store) readMeta(id string) (*BundleMeta, error) {
	if !validID(id) {
		return nil, ErrNotFound
	}
	b, err := os.ReadFile(filepath.Join(s.bundleDir(id), "meta.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var meta BundleMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, fmt.Errorf("decode bundle metadata: %w", err)
	}
	return &meta, nil
}

// authorize reports whether sc may act on the bundle. Expiry is deliberately
// not consulted: an expired bundle is still owned by somebody, and the callers
// that must reject expired content (Get, List) check that themselves.
func (s *Store) authorize(id string, sc Scope) error {
	meta, err := s.readMeta(id)
	if err != nil {
		return err
	}
	if !sc.owns(meta) {
		return ErrNotFound
	}
	return nil
}

// walk visits the stored metadata of every bundle, expired ones included.
func (s *Store) walk(visit func(*BundleMeta) error) error {
	entries, err := os.ReadDir(s.bundlesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		meta, err := s.readMeta(entry.Name())
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := visit(meta); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) List(sc Scope) ([]BundleMeta, error) {
	var out []BundleMeta
	err := s.walk(func(meta *BundleMeta) error {
		if meta.expired() || !sc.owns(meta) {
			return nil
		}
		out = append(out, *meta)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b BundleMeta) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

func (s *Store) Open(id, name string) (*os.File, error) {
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	clean, err := ValidateName(name)
	if err != nil {
		return nil, fs.ErrNotExist
	}
	root, err := os.OpenRoot(filepath.Join(s.bundleDir(id), "files"))
	if err != nil {
		return nil, err
	}
	f, err := root.Open(clean)
	root.Close()
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return nil, fs.ErrNotExist
	}
	return f, err
}

func (s *Store) Delete(id string, sc Scope) error {
	if err := s.authorize(id, sc); err != nil {
		return err
	}
	return s.remove(id)
}

// remove discards a bundle directory. Callers are responsible for authorizing
// the removal first.
func (s *Store) remove(id string) error {
	final := s.bundleDir(id)
	if _, err := os.Stat(final); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	trash := filepath.Join(s.bundlesDir, ".trash-"+id)
	if err := os.Rename(final, trash); err != nil {
		return err
	}
	return os.RemoveAll(trash)
}

// AdoptUnownedBundles assigns legacy metadata to the bootstrap admin. It is
// idempotent and intentionally includes expired bundles so GC can handle them.
func (s *Store) AdoptUnownedBundles(ownerID string) (int, error) {
	if ownerID == "" {
		return 0, errors.New("owner is required")
	}
	adopted := 0
	err := s.walk(func(meta *BundleMeta) error {
		if meta.OwnerID != "" {
			return nil
		}
		meta.OwnerID = ownerID
		if err := s.writeMeta(meta); err != nil {
			return err
		}
		adopted++
		return nil
	})
	return adopted, err
}

func (s *Store) writeMeta(meta *BundleMeta) error {
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	dir := s.bundleDir(meta.ID)
	tmp := filepath.Join(dir, "meta.json.tmp")
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
	if err := os.Rename(tmp, filepath.Join(dir, "meta.json")); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return fsutil.SyncDir(dir)
}

// ownedBy collects every bundle belonging to ownerID. Unlike List it includes
// expired bundles that the sweeper has not reached yet, so tearing down a user
// never leaves metadata pointing at an account that no longer exists.
func (s *Store) ownedBy(ownerID string) ([]BundleMeta, error) {
	if ownerID == "" {
		return nil, errors.New("owner is required")
	}
	var out []BundleMeta
	err := s.walk(func(meta *BundleMeta) error {
		if meta.OwnerID == ownerID {
			out = append(out, *meta)
		}
		return nil
	})
	return out, err
}

func (s *Store) CountOwned(ownerID string) (int, error) {
	metas, err := s.ownedBy(ownerID)
	return len(metas), err
}

func (s *Store) DeleteOwned(ownerID string) (int, error) {
	metas, err := s.ownedBy(ownerID)
	if err != nil {
		return 0, err
	}
	for i, meta := range metas {
		if err := s.remove(meta.ID); err != nil {
			return i, err
		}
	}
	return len(metas), nil
}

func (s *Store) ReassignOwned(from, to string) (int, error) {
	if to == "" {
		return 0, errors.New("destination owner is required")
	}
	metas, err := s.ownedBy(from)
	if err != nil {
		return 0, err
	}
	for i := range metas {
		metas[i].OwnerID = to
		if err := s.writeMeta(&metas[i]); err != nil {
			return i, err
		}
	}
	return len(metas), nil
}

func (s *Store) bundleDir(id string) string { return filepath.Join(s.bundlesDir, id) }

func validID(id string) bool {
	if len(id) < 13 || len(id) > 52 {
		return false
	}
	for _, r := range id {
		if !strings.ContainsRune(crockford, r) {
			return false
		}
	}
	return true
}
