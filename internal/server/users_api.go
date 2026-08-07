package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/readeem/hostebin/internal/users"
)

type tokenMetadata struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id,omitempty"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type userWithToken struct {
	users.User
	Token *tokenMetadata `json:"token,omitempty"`
}

func publicToken(t users.Token) tokenMetadata {
	return tokenMetadata{ID: t.ID, UserID: t.UserID, Label: t.Label, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt}
}

func (s *Server) whoami(w http.ResponseWriter, _ *http.Request, principal users.Principal) {
	writeJSON(w, http.StatusOK, principal)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, _ users.Principal) {
	all, err := s.cfg.Users.ListUsers(r.Context())
	if err != nil {
		writeUsersError(w, err)
		return
	}
	out := make([]userWithToken, 0, len(all))
	for _, user := range all {
		item := userWithToken{User: user}
		token, err := s.cfg.Users.GetToken(r.Context(), user.ID)
		if err == nil {
			metadata := publicToken(token)
			item.Token = &metadata
		} else if !errors.Is(err, users.ErrTokenNotFound) {
			writeUsersError(w, err)
			return
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request, principal users.Principal) {
	var req struct {
		Name  string     `json:"name"`
		Role  users.Role `json:"role"`
		Label string     `json:"label"`
		TTL   string     `json:"ttl"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Role == "" {
		req.Role = users.RoleUser
	}
	ttl, ok := parseTokenTTL(w, req.TTL)
	if !ok {
		return
	}
	user, token, plaintext, err := s.cfg.Users.CreateUser(r.Context(), req.Name, req.Role, req.Label, ttl)
	if err != nil {
		writeUsersError(w, err)
		return
	}
	s.cfg.Logger.Info().Str("action", "create_user").Str("user", principal.Name).Str("target", user.Name).Str("token_id", token.ID).Msg("user created")
	writeJSON(w, http.StatusCreated, map[string]any{"user": user, "token": publicToken(token), "plaintext": plaintext})
}

func (s *Server) patchUser(w http.ResponseWriter, r *http.Request, principal users.Principal) {
	var req struct {
		Disabled *bool `json:"disabled"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Disabled == nil {
		writeError(w, http.StatusBadRequest, "disabled is required")
		return
	}
	if err := s.cfg.Users.SetDisabled(r.Context(), r.PathValue("id"), *req.Disabled); err != nil {
		writeUsersError(w, err)
		return
	}
	s.cfg.Logger.Info().Str("action", "set_user_disabled").Str("user", principal.Name).Str("target_id", r.PathValue("id")).Bool("disabled", *req.Disabled).Msg("user updated")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, principal users.Principal) {
	id := r.PathValue("id")
	target, err := s.cfg.Users.GetUser(r.Context(), id)
	if err != nil {
		writeUsersError(w, err)
		return
	}
	if err := s.cfg.Users.ValidateDeleteUser(r.Context(), id); err != nil {
		writeUsersError(w, err)
		return
	}
	count, err := s.cfg.Store.CountOwned(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	action := r.URL.Query().Get("bundles")
	if count > 0 && action == "" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "user owns bundles", "bundles": count})
		return
	}
	switch action {
	case "":
		// Nothing to disown: the conflict above already rejected count > 0.
	case "delete":
		if _, err := s.cfg.Store.DeleteOwned(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "reassign":
		destination, err := s.reassignDestination(r, principal, id)
		if err != nil {
			writeUsersError(w, err)
			return
		}
		if _, err := s.cfg.Store.ReassignOwned(id, destination); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "bundles must be delete or reassign")
		return
	}
	if err := s.cfg.Users.DeleteUser(r.Context(), id); err != nil {
		writeUsersError(w, err)
		return
	}
	s.cfg.Logger.Info().Str("action", "delete_user").Str("user", principal.Name).Str("target", target.Name).Msg("user deleted")
	w.WriteHeader(http.StatusNoContent)
}

// reassignDestination picks who inherits a deleted user's bundles: the caller,
// or another enabled admin when an admin is deleting their own account. It never
// returns deletedID, which would leave the bundles owned by nobody.
func (s *Server) reassignDestination(r *http.Request, principal users.Principal, deletedID string) (string, error) {
	if principal.UserID != deletedID {
		return principal.UserID, nil
	}
	all, err := s.cfg.Users.ListUsers(r.Context())
	if err != nil {
		return "", err
	}
	for _, candidate := range all {
		if candidate.ID != deletedID && candidate.Role == users.RoleAdmin && !candidate.Disabled {
			return candidate.ID, nil
		}
	}
	return "", users.ErrLastAdmin
}

func (s *Server) rotateToken(w http.ResponseWriter, r *http.Request, principal users.Principal) {
	userID := r.PathValue("id")
	if !principal.IsAdmin() && principal.UserID != userID {
		writeError(w, http.StatusForbidden, "may only rotate your own token")
		return
	}
	var req struct {
		Label string `json:"label"`
		TTL   string `json:"ttl"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ttl, ok := parseTokenTTL(w, req.TTL)
	if !ok {
		return
	}
	token, plaintext, err := s.cfg.Users.RotateToken(r.Context(), userID, req.Label, ttl)
	if err != nil {
		writeUsersError(w, err)
		return
	}
	s.cfg.Logger.Info().Str("action", "rotate_token").Str("user", principal.Name).Str("token_id", token.ID).Str("target_id", userID).Msg("token rotated")
	writeJSON(w, http.StatusOK, map[string]any{"token": publicToken(token), "plaintext": plaintext})
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request, principal users.Principal) {
	userID := r.PathValue("id")
	if !principal.IsAdmin() && userID != principal.UserID {
		writeError(w, http.StatusForbidden, "may only revoke your own token")
		return
	}
	token, err := s.cfg.Users.GetToken(r.Context(), userID)
	if err != nil {
		writeUsersError(w, err)
		return
	}
	if err := s.cfg.Users.RevokeToken(r.Context(), userID); err != nil {
		writeUsersError(w, err)
		return
	}
	s.cfg.Logger.Info().Str("action", "revoke_token").Str("user", principal.Name).Str("token_id", token.ID).Msg("token revoked")
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	// An absent body means "all defaults" — `curl -X PUT .../token` should not
	// have to send `{}` just to accept them.
	if err := dec.Decode(dst); errors.Is(err, io.EOF) {
		return true
	} else if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func parseTokenTTL(w http.ResponseWriter, raw string) (time.Duration, bool) {
	if raw == "" || strings.EqualFold(raw, "never") {
		return 0, true
	}
	ttl, err := ParseDuration(raw)
	if err != nil || ttl <= 0 {
		writeError(w, http.StatusBadRequest, "ttl must be a positive duration or never")
		return 0, false
	}
	return ttl, true
}

func writeUsersError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrUserNotFound), errors.Is(err, users.ErrTokenNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, users.ErrDuplicateName), errors.Is(err, users.ErrLastAdmin), errors.Is(err, users.ErrLastAdminToken):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, users.ErrInvalidName), errors.Is(err, users.ErrInvalidRole):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
