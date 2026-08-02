package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hostebin/hostebin/internal/store"
)

const DefaultCSP = "default-src 'self' data: blob: https: 'unsafe-inline' 'unsafe-eval'; connect-src 'self'; form-action 'none'; frame-ancestors 'none'"

type Config struct {
	Store      *store.Store
	Token      string
	MaxUpload  int64
	MaxFiles   int
	DefaultTTL time.Duration
	CSP        string
	Logger     *slog.Logger
}

type Server struct {
	cfg      Config
	renderer *renderer
	handler  http.Handler
}

func New(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, errors.New("store is required")
	}
	if cfg.Token == "" {
		return nil, errors.New("upload token is required")
	}
	if cfg.MaxUpload <= 0 {
		cfg.MaxUpload = 32 << 20
	}
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = 64
	}
	if cfg.CSP == "" {
		cfg.CSP = DefaultCSP
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	renderer, err := newRenderer()
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, renderer: renderer}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /api/v1/bundles", s.auth(s.createBundle))
	mux.HandleFunc("GET /api/v1/bundles", s.auth(s.listBundles))
	mux.HandleFunc("PUT /api/v1/bundles/{id}", s.auth(s.updateBundle))
	mux.HandleFunc("DELETE /api/v1/bundles/{id}", s.auth(s.deleteBundle))
	mux.HandleFunc("GET /b/{id}/{path...}", s.serveBundle)
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/b/") && hasTraversalSegment(r.URL.Path) {
			http.Error(w, "invalid bundle path", http.StatusBadRequest)
			return
		}
		mux.ServeHTTP(w, r)
	})
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func hasTraversalSegment(name string) bool {
	for segment := range strings.SplitSeq(name, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

type baseURLKey struct{}

func WithBaseURL(base string, next http.Handler) http.Handler {
	base = strings.TrimRight(base, "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), baseURLKey{}, base)))
	})
}

func baseURL(r *http.Request) string {
	if base, ok := r.Context().Value(baseURLKey{}).(string); ok && base != "" {
		return base
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(s.cfg.Token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.Token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "valid bearer token required")
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func statusForStoreError(err error) int {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrExpired) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func fileURL(base, id, name string) string {
	parts := strings.Split(name, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return fmt.Sprintf("%s/b/%s/%s", strings.TrimRight(base, "/"), id, strings.Join(parts, "/"))
}
