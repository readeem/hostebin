package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/readeem/hostebin/internal/store"
)

// serveBundle handles the path-based /b/{id}/{path...} route on the apex host.
// In subdomain mode it only redirects, so a bundle is reachable from exactly one
// origin and links shared before the switch keep working.
func (s *Server) serveBundle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.cfg.BundleHost != "" {
		target := s.bundleRoot(r, id) + "/" + r.PathValue("path")
		if q := r.URL.RawQuery; q != "" {
			target += "?" + q
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
		return
	}
	s.serveBundleContent(w, r, id, r.PathValue("path"))
}

func (s *Server) serveBundleContent(w http.ResponseWriter, r *http.Request, id, name string) {
	if s.cfg.CSP != "off" {
		w.Header().Set("Content-Security-Policy", s.cfg.CSP)
	}
	// Under subdomain hosting the bundle id *is* the origin, so a default
	// referrer policy would hand it to every third party the page loads from.
	w.Header().Set("Referrer-Policy", "no-referrer")
	meta, err := s.cfg.Store.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if name == "" {
		if meta.Entry != "" {
			s.serveFile(w, r, meta, meta.Entry)
			return
		}
		s.serveListing(w, r, meta)
		return
	}
	if strings.HasSuffix(name, "/") {
		index := path.Join(name, "index.html")
		if hasMetaFile(meta, index) {
			s.serveFile(w, r, meta, index)
			return
		}
	}
	s.serveFile(w, r, meta, name)
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, meta *store.BundleMeta, name string) {
	f, err := s.cfg.Store.Open(meta.ID, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, store.ErrExpired) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()
	info, statErr := f.Stat()
	if statErr != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	raw := r.URL.Query().Get("raw") == "1"
	ext := strings.ToLower(path.Ext(name))
	if !raw && (ext == ".md" || ext == ".markdown") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.renderer.renderMarkdown(w, name, f); err != nil {
			s.cfg.Logger.Error().Err(err).Msg("render markdown")
		}
		return
	}
	contentType := contentTypeFor(name)
	if raw {
		contentType = "text/plain; charset=utf-8"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(name)))
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, path.Base(name), info.ModTime(), f)
}

func (s *Server) serveListing(w http.ResponseWriter, _ *http.Request, meta *store.BundleMeta) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	files := make([]listingFile, 0, len(meta.Files))
	for _, f := range meta.Files {
		parts := strings.Split(f.Name, "/")
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		files = append(files, listingFile{Name: f.Name, URL: strings.Join(parts, "/"), Size: humanBytes(f.Size)})
	}
	description := fmt.Sprintf("%d files · %s", len(meta.Files), humanBytes(meta.Bytes))
	if err := s.renderer.renderListing(w, meta.Title, description, files); err != nil {
		s.cfg.Logger.Error().Err(err).Msg("render listing")
	}
}

func hasMetaFile(meta *store.BundleMeta, name string) bool {
	for _, f := range meta.Files {
		if f.Name == name {
			return true
		}
	}
	return false
}
