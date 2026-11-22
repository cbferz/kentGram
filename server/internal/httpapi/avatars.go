package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const maxAvatarBytes = 5 << 20

var avatarFilenamePattern = regexp.MustCompile(`^[a-f0-9]{32}\.(jpg|png)$`)

func (a *API) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+(1<<20))
	if err := r.ParseMultipartForm(maxAvatarBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_or_too_large_avatar")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, http.StatusBadRequest, "avatar_file_required")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxAvatarBytes {
		writeError(w, http.StatusBadRequest, "avatar_must_be_under_5mb")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxAvatarBytes {
		writeError(w, http.StatusBadRequest, "invalid_or_too_large_avatar")
		return
	}
	dimensions, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (dimensions.Width < 1 || dimensions.Height < 1) ||
		dimensions.Width > 4096 || dimensions.Height > 4096 ||
		int64(dimensions.Width)*int64(dimensions.Height) > 12_000_000 {
		writeError(w, http.StatusBadRequest, "unsupported_or_invalid_image")
		return
	}
	mime := http.DetectContentType(data)
	ext := ""
	switch {
	case mime == "image/jpeg":
		ext = ".jpg"
	case mime == "image/png":
		ext = ".png"
	default:
		writeError(w, http.StatusBadRequest, "avatar_format_must_be_jpeg_or_png")
		return
	}

	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		writeError(w, http.StatusInternalServerError, "avatar_storage_failed")
		return
	}
	filename := hex.EncodeToString(id[:]) + ext
	if err := os.MkdirAll(a.cfg.AvatarDir, 0750); err != nil {
		writeError(w, http.StatusInternalServerError, "avatar_storage_failed")
		return
	}
	path := filepath.Join(a.cfg.AvatarDir, filename)
	if err := os.WriteFile(path, data, 0640); err != nil {
		writeError(w, http.StatusInternalServerError, "avatar_storage_failed")
		return
	}

	user, err := a.db.UserByID(r.Context(), userID(r))
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	base := strings.TrimRight(a.cfg.PublicBaseURL, "/")
	avatarURL := fmt.Sprintf("%s/media/avatars/%s", base, filename)
	updated, err := a.db.UpdateProfile(r.Context(), user.ID, user.DisplayName, user.Bio, avatarURL)
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "avatar_profile_update_failed")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) avatarFile(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if !avatarFilenamePattern.MatchString(filename) {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(filepath.Join(a.cfg.AvatarDir, filename))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, "avatar_read_failed")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	contentType := "image/jpeg"
	if strings.HasSuffix(filename, ".png") {
		contentType = "image/png"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filename, info.ModTime().Round(time.Second), file)
}
