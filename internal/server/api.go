package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/readeem/hostebin/internal/store"
)

type uploadResponse struct {
	ID        string         `json:"id"`
	URL       string         `json:"url"`
	EntryURL  string         `json:"entry_url,omitempty"`
	ExpiresAt *time.Time     `json:"expires_at"`
	Files     []responseFile `json:"files"`
}

type responseFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type parsedUpload struct {
	files             []store.File
	title, entry, ttl string
}

func (s *Server) createBundle(w http.ResponseWriter, r *http.Request) {
	upload, ok := s.parseUpload(w, r)
	if !ok {
		return
	}
	opts, ok := s.uploadOptions(w, r, upload)
	if !ok {
		return
	}
	meta, err := s.cfg.Store.Create(opts, upload.files)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.cfg.Logger.Info().
		Str("action", "create").
		Str("bundle_id", meta.ID).
		Int("file_count", len(meta.Files)).
		Int64("byte_count", meta.Bytes).
		Msg("bundle created")
	writeJSON(w, http.StatusCreated, makeUploadResponse(baseURL(r), meta))
}

func (s *Server) updateBundle(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "replace"
	}
	if mode != "merge" && mode != "replace" {
		writeError(w, http.StatusBadRequest, "mode must be merge or replace")
		return
	}
	upload, ok := s.parseUpload(w, r)
	if !ok {
		return
	}
	opts, ok := s.uploadOptions(w, r, upload)
	if !ok {
		return
	}
	meta, err := s.cfg.Store.Update(r.PathValue("id"), opts, upload.files, mode)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrExpired) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	s.cfg.Logger.Info().
		Str("action", "update").
		Str("bundle_id", meta.ID).
		Str("mode", mode).
		Int("file_count", len(meta.Files)).
		Int64("byte_count", meta.Bytes).
		Msg("bundle updated")
	writeJSON(w, http.StatusOK, makeUploadResponse(baseURL(r), meta))
}

func (s *Server) parseUpload(w http.ResponseWriter, r *http.Request) (parsedUpload, bool) {
	var out parsedUpload
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload+(1<<20))
	contentType := r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	remaining := s.cfg.MaxUpload
	readOne := func(name, declaredType string, src io.Reader) bool {
		if len(out.files) >= s.cfg.MaxFiles {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("upload contains more than %d files", s.cfg.MaxFiles))
			return false
		}
		data, err := io.ReadAll(io.LimitReader(src, remaining+1))
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			} else {
				writeError(w, http.StatusBadRequest, err.Error())
			}
			return false
		}
		if int64(len(data)) > remaining {
			writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			return false
		}
		remaining -= int64(len(data))
		out.files = append(out.files, store.File{Name: name, ContentType: safeContentType(name, declaredType), Reader: bytes.NewReader(data)})
		return true
	}
	if mediaType == "multipart/form-data" {
		mr, err := r.MultipartReader()
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid multipart upload")
			return out, false
		}
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
					writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
				} else {
					writeError(w, http.StatusBadRequest, "invalid multipart upload")
				}
				return out, false
			}
			disposition, params, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			filename := params["filename"]
			if disposition == "form-data" && part.FormName() == "file" && filename != "" {
				if !readOne(filename, part.Header.Get("Content-Type"), part) {
					part.Close()
					return out, false
				}
			} else {
				value, err := io.ReadAll(io.LimitReader(part, 4097))
				if err != nil || len(value) > 4096 {
					part.Close()
					writeError(w, http.StatusBadRequest, "multipart field too large")
					return out, false
				}
				switch part.FormName() {
				case "title":
					out.title = string(value)
				case "entry":
					out.entry = string(value)
				case "ttl":
					out.ttl = string(value)
				}
			}
			part.Close()
		}
	} else {
		name := r.Header.Get("X-Hostebin-Filename")
		if name == "" {
			writeError(w, http.StatusBadRequest, "X-Hostebin-Filename is required for raw uploads")
			return out, false
		}
		if !readOne(name, contentType, r.Body) {
			return out, false
		}
		out.title, out.entry, out.ttl = r.Header.Get("X-Hostebin-Title"), r.Header.Get("X-Hostebin-Entry"), r.Header.Get("X-Hostebin-TTL")
	}
	if len(out.files) == 0 {
		writeError(w, http.StatusBadRequest, "at least one file is required")
		return out, false
	}
	return out, true
}

func (s *Server) uploadOptions(w http.ResponseWriter, r *http.Request, upload parsedUpload) (store.Options, bool) {
	opts := store.Options{Title: upload.title, Entry: upload.entry, EntrySet: upload.entry != "", Uploader: r.RemoteAddr}
	if upload.ttl == "" {
		if s.cfg.DefaultTTL > 0 {
			expires := time.Now().UTC().Add(s.cfg.DefaultTTL)
			opts.ExpiresAt = &expires
			opts.ExpiresSet = true
		}
		return opts, true
	}
	if strings.EqualFold(upload.ttl, "never") {
		opts.ExpiresSet = true
		return opts, true
	}
	d, err := ParseDuration(upload.ttl)
	if err != nil || d <= 0 {
		writeError(w, http.StatusBadRequest, "ttl must be a positive duration or never")
		return opts, false
	}
	expires := time.Now().UTC().Add(d)
	opts.ExpiresAt = &expires
	opts.ExpiresSet = true
	return opts, true
}

// ParseDuration extends time.ParseDuration with the day/week units used by the CLI.
func ParseDuration(raw string) (time.Duration, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	for _, unit := range []struct {
		suffix string
		hours  float64
	}{{"w", 24 * 7}, {"d", 24}} {
		if before, ok := strings.CutSuffix(s, unit.suffix); ok {
			n, err := strconv.ParseFloat(strings.TrimSpace(before), 64)
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("invalid duration %q", raw)
			}
			return time.Duration(n * unit.hours * float64(time.Hour)), nil
		}
	}
	return time.ParseDuration(s)
}

func makeUploadResponse(base string, meta *store.BundleMeta) uploadResponse {
	resp := uploadResponse{ID: meta.ID, URL: fmt.Sprintf("%s/b/%s/", strings.TrimRight(base, "/"), meta.ID), ExpiresAt: meta.ExpiresAt}
	if meta.Entry != "" {
		resp.EntryURL = fileURL(base, meta.ID, meta.Entry)
	}
	for _, f := range meta.Files {
		resp.Files = append(resp.Files, responseFile{Name: f.Name, Size: f.Size, URL: fileURL(base, meta.ID, f.Name)})
	}
	return resp
}

func (s *Server) listBundles(w http.ResponseWriter, r *http.Request) {
	metas, err := s.cfg.Store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.cfg.Logger.Info().
		Str("action", "read").
		Int("bundle_count", len(metas)).
		Msg("bundles listed")
	writeJSON(w, http.StatusOK, metas)
}

func (s *Server) deleteBundle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.cfg.Store.Delete(id)
	if err != nil {
		writeError(w, statusForStoreError(err), err.Error())
		return
	}
	s.cfg.Logger.Info().
		Str("action", "delete").
		Str("bundle_id", id).
		Msg("bundle deleted")
	w.WriteHeader(http.StatusNoContent)
}

func safeContentType(name, declared string) string {
	if t := contentTypeFor(name); t != "" {
		return t
	}
	t, _, _ := mime.ParseMediaType(declared)
	if strings.HasPrefix(t, "text/") {
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

func contentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	case ".txt", ".log", ".csv":
		return "text/plain; charset=utf-8"
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".pdf":
		return "application/pdf"
	case ".xml":
		return "application/xml"
	case ".wasm":
		return "application/wasm"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	default:
		return ""
	}
}
