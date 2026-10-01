package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/readeem/hostebin/internal/theme"
	"github.com/readeem/hostebin/internal/users"
)

// ThemePath is where the server publishes its own theme. Inside a bundle, any
// path ending in ThemeFile serves the bundle owner's theme instead, so pages
// link it relatively and get the right one in every routing mode.
const (
	ThemePath = "/" + ThemeFile
	ThemeFile = ".hostebin/theme.css"
)

const maxThemeBytes = 64 << 10

func (s *Server) userThemePath(userID string) string {
	return filepath.Join(s.cfg.Store.DataDir(), "themes", userID+".css")
}

func (s *Server) themeCSS(ownerID string) []byte {
	css, err := s.cfg.Theme()
	if err != nil {
		s.cfg.Logger.Warn().Err(err).Msg("load theme; using the default")
		css = theme.Default
	}
	if ownerID == "" {
		return css
	}
	own, err := os.ReadFile(s.userThemePath(ownerID))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.cfg.Logger.Warn().Err(err).Str("user_id", ownerID).Msg("load user theme")
		}
		return css
	}
	return append(append(append([]byte{}, css...), '\n'), own...)
}

func (s *Server) serveTheme(w http.ResponseWriter, r *http.Request, ownerID string) {
	css := s.themeCSS(ownerID)
	sum := sha256.Sum256(css)
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:8])+`"`)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(css))
}

func (s *Server) setTheme(w http.ResponseWriter, r *http.Request, principal users.Principal) {
	userID, ok := s.themeTarget(w, r, principal)
	if !ok {
		return
	}
	css, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxThemeBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "theme is larger than 64 KiB")
		return
	}
	// Rendered pages inline the theme in a <style> element, which "<" could close.
	if bytes.ContainsRune(css, '<') {
		writeError(w, http.StatusBadRequest, `theme must not contain "<"`)
		return
	}
	path := s.userThemePath(userID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, css, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.cfg.Logger.Info().Str("action", "set_theme").Str("user", principal.Name).Str("target_id", userID).Msg("theme set")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeTheme(w http.ResponseWriter, r *http.Request, principal users.Principal) {
	userID, ok := s.themeTarget(w, r, principal)
	if !ok {
		return
	}
	if err := os.Remove(s.userThemePath(userID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.cfg.Logger.Info().Str("action", "remove_theme").Str("user", principal.Name).Str("target_id", userID).Msg("theme removed")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) themeTarget(w http.ResponseWriter, r *http.Request, principal users.Principal) (string, bool) {
	userID := r.PathValue("id")
	if !principal.IsAdmin() && principal.UserID != userID {
		writeError(w, http.StatusForbidden, "may only change your own theme")
		return "", false
	}
	if _, err := s.cfg.Users.GetUser(r.Context(), userID); err != nil {
		writeUsersError(w, err)
		return "", false
	}
	return userID, true
}
