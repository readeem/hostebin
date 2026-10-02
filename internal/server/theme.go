package server

import (
	"bytes"
	"crypto/rand"
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

func userThemeName(userID string) string {
	return filepath.Join("themes", userID+".css")
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
	root, err := os.OpenRoot(s.cfg.Store.DataDir())
	if err != nil {
		s.cfg.Logger.Warn().Err(err).Msg("open theme storage")
		return css
	}
	defer root.Close()
	own, err := root.ReadFile(userThemeName(ownerID))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.cfg.Logger.Warn().Err(err).Str("user_id", ownerID).Msg("load user theme")
		}
		return css
	}
	return theme.Layer(css, own)
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
	if bytes.ContainsRune(css, '<') {
		writeError(w, http.StatusBadRequest, `theme must not contain "<"`)
		return
	}
	if err := s.writeUserTheme(userID, css); err != nil {
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
	if err := s.removeUserTheme(userID); err != nil && !errors.Is(err, fs.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.cfg.Logger.Info().Str("action", "remove_theme").Str("user", principal.Name).Str("target_id", userID).Msg("theme removed")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeUserTheme(userID string, css []byte) error {
	root, err := os.OpenRoot(s.cfg.Store.DataDir())
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll("themes", 0o755); err != nil {
		return err
	}
	tmp := filepath.Join("themes", ".tmp-"+rand.Text())
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	defer f.Close()
	if _, err := f.Write(css); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return root.Rename(tmp, userThemeName(userID))
}

func (s *Server) removeUserTheme(userID string) error {
	root, err := os.OpenRoot(s.cfg.Store.DataDir())
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(userThemeName(userID))
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
