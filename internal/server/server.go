package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/readeem/hostebin/internal/logging"
	"github.com/readeem/hostebin/internal/store"
	"github.com/readeem/hostebin/internal/users"
	"github.com/rs/zerolog"
)

// DefaultCSP lets a page reach any HTTPS origin so reports can pull live data,
// but not plain HTTP — that keeps a published page from probing the reader's
// LAN (http://192.168.x.x, http://localhost:11434) from their browser.
const DefaultCSP = "default-src 'self' data: blob: https: 'unsafe-inline' 'unsafe-eval'; connect-src 'self' https:; form-action 'none'; frame-ancestors 'none'"

type Config struct {
	Store      *store.Store
	Users      *users.Service
	MaxUpload  int64
	MaxFiles   int
	DefaultTTL time.Duration
	CSP        string
	BundleHost string
	Logger     *zerolog.Logger
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
	if cfg.Users == nil {
		return nil, errors.New("users service is required")
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
	if cfg.BundleHost != "" {
		normalized, err := NormalizeBundleHost(cfg.BundleHost)
		if err != nil {
			return nil, err
		}
		cfg.BundleHost = normalized
	}
	if cfg.Logger == nil {
		cfg.Logger = logging.NewConsole(os.Stderr)
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
	mux.HandleFunc("GET /api/v1/whoami", s.auth(s.whoami))
	mux.HandleFunc("GET /api/v1/users", s.admin(s.listUsers))
	mux.HandleFunc("POST /api/v1/users", s.admin(s.createUser))
	mux.HandleFunc("PATCH /api/v1/users/{id}", s.admin(s.patchUser))
	mux.HandleFunc("DELETE /api/v1/users/{id}", s.admin(s.deleteUser))
	mux.HandleFunc("PUT /api/v1/users/{id}/token", s.auth(s.rotateToken))
	mux.HandleFunc("DELETE /api/v1/users/{id}/token", s.auth(s.revokeToken))
	mux.HandleFunc("GET /b/{id}/{path...}", s.serveBundle)

	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// A request that arrives on a bundle subdomain never reaches the mux, so
		// the API is unreachable from bundle content even if a page tries.
		if id, ok := s.bundleHostID(r.Host); ok {
			if hasTraversalSegment(r.URL.Path) {
				http.Error(w, "invalid bundle path", http.StatusBadRequest)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			s.serveBundleContent(w, r, id, strings.TrimPrefix(r.URL.Path, "/"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/b/") && hasTraversalSegment(r.URL.Path) {
			http.Error(w, "invalid bundle path", http.StatusBadRequest)
			return
		}
		mux.ServeHTTP(w, r)
	})
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func NormalizeBundleHost(pattern string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(pattern))
	h = strings.TrimSuffix(h, ".")
	rest, ok := strings.CutPrefix(h, "*.")
	if !ok {
		return "", fmt.Errorf("bundle host %q must be a wildcard such as *.example.com", pattern)
	}
	if rest == "" || strings.Contains(rest, "*") || strings.HasPrefix(rest, ".") {
		return "", fmt.Errorf("bundle host %q must be a wildcard such as *.example.com", pattern)
	}
	for _, r := range rest {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-') {
			return "", fmt.Errorf("bundle host %q contains an invalid character %q", pattern, r)
		}
	}
	return "*." + rest, nil
}

// bundleHostID returns the bundle id a request's Host addresses, if the server
// is running in subdomain mode and the host matches the configured wildcard.
func (s *Server) bundleHostID(host string) (string, bool) {
	if s.cfg.BundleHost == "" || host == "" {
		return "", false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	label, ok := strings.CutSuffix(host, s.cfg.BundleHost[1:])
	if !ok || label == "" || strings.Contains(label, ".") {
		return "", false
	}
	return label, true
}

// bundleRoot is the URL a bundle's files hang off, with no trailing slash.
func (s *Server) bundleRoot(r *http.Request, id string) string {
	base := baseURL(r)
	if s.cfg.BundleHost == "" {
		return base + "/b/" + id
	}
	scheme, port := "https", ""
	if u, err := url.Parse(base); err == nil {
		if u.Scheme != "" {
			scheme = u.Scheme
		}
		if p := u.Port(); p != "" {
			port = ":" + p
		}
	}
	return scheme + "://" + id + s.cfg.BundleHost[1:] + port
}

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

type principalHandler func(http.ResponseWriter, *http.Request, users.Principal)

func (s *Server) auth(next principalHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		provided, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || provided == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "valid bearer token required")
			return
		}
		principal, err := s.cfg.Users.Authenticate(r.Context(), provided)
		if err != nil {
			s.cfg.Logger.Warn().Str("action", "authenticate").Err(err).Msg("authentication failed")
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "valid bearer token required")
			return
		}
		next(w, r, principal)
	}
}

func (s *Server) admin(next principalHandler) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request, principal users.Principal) {
		if !principal.IsAdmin() {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, r, principal)
	})
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

// fileURL joins a bundle root and a stored file name into a fetchable URL.
func fileURL(root, name string) string {
	parts := strings.Split(name, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.TrimRight(root, "/") + "/" + strings.Join(parts, "/")
}
