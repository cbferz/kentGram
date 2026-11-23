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
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/cbferz/kentgram/server/internal/store"
	"github.com/go-chi/chi/v5"
)

const maxChatMediaBytes = 20 << 20

var chatMediaIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (a *API) uploadChatMedia(w http.ResponseWriter, r *http.Request) {
	chatID, ok := parseCommunityChatID(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxChatMediaBytes+(1<<20))
	if err := r.ParseMultipartForm(maxChatMediaBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_or_too_large_media")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "media_file_required")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxChatMediaBytes {
		writeError(w, http.StatusBadRequest, "media_must_be_under_20mb")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxChatMediaBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxChatMediaBytes {
		writeError(w, http.StatusBadRequest, "invalid_or_too_large_media")
		return
	}

	contentType := http.DetectContentType(data)
	extension := ""
	switch contentType {
	case "image/jpeg":
		extension = ".jpg"
		dimensions, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr != nil || dimensions.Width < 1 || dimensions.Height < 1 ||
			dimensions.Width > 4096 || dimensions.Height > 4096 ||
			int64(dimensions.Width)*int64(dimensions.Height) > 12_000_000 {
			writeError(w, http.StatusBadRequest, "invalid_image")
			return
		}
	case "image/png":
		extension = ".png"
		dimensions, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr != nil || dimensions.Width < 1 || dimensions.Height < 1 ||
			dimensions.Width > 4096 || dimensions.Height > 4096 ||
			int64(dimensions.Width)*int64(dimensions.Height) > 12_000_000 {
			writeError(w, http.StatusBadRequest, "invalid_image")
			return
		}
	case "application/pdf":
		extension = ".pdf"
	default:
		writeError(w, http.StatusBadRequest, "supported_media: jpeg_png_pdf")
		return
	}

	originalName := safeMediaName(header.Filename)
	if originalName == "" {
		originalName = "attachment" + extension
	}
	caption := strings.TrimSpace(r.FormValue("caption"))
	if len([]rune(caption)) > 4000 {
		writeError(w, http.StatusBadRequest, "caption_too_long")
		return
	}
	body := caption
	if body == "" {
		body = originalName
	}

	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		writeError(w, http.StatusInternalServerError, "media_storage_failed")
		return
	}
	mediaID := hex.EncodeToString(idBytes[:])
	storageName := mediaID + extension
	if err := os.MkdirAll(a.cfg.MediaDir, 0750); err != nil {
		writeError(w, http.StatusInternalServerError, "media_storage_failed")
		return
	}
	path := filepath.Join(a.cfg.MediaDir, storageName)
	if err := os.WriteFile(path, data, 0640); err != nil {
		writeError(w, http.StatusInternalServerError, "media_storage_failed")
		return
	}

	message, err := a.db.CreateChatMediaMessage(r.Context(), chatID, userID(r), body, store.ChatMedia{
		MediaID: mediaID,
		ChatID: chatID,
		UploaderID: userID(r),
		StorageName: storageName,
		OriginalName: originalName,
		ContentType: contentType,
		SizeBytes: int64(len(data)),
	})
	if err != nil {
		_ = os.Remove(path)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "chat_not_found_or_upload_forbidden")
			return
		}
		writeError(w, http.StatusInternalServerError, "media_message_create_failed")
		return
	}
	a.publishChatEvent(r.Context(), chatID, realtimeEvent{
		Type: "message.created", ChatID: chatID, MessageID: message.ID,
	}, 0)
	writeJSON(w, http.StatusCreated, message)
}

func (a *API) downloadChatMedia(w http.ResponseWriter, r *http.Request) {
	chatID, ok := parseCommunityChatID(w, r)
	if !ok {
		return
	}
	mediaID := chi.URLParam(r, "mediaID")
	if !chatMediaIDPattern.MatchString(mediaID) {
		http.NotFound(w, r)
		return
	}
	media, err := a.db.GetChatMedia(r.Context(), chatID, userID(r), mediaID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "media_lookup_failed")
		return
	}
	file, err := os.Open(filepath.Join(a.cfg.MediaDir, media.StorageName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, "media_read_failed")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != media.SizeBytes {
		http.NotFound(w, r)
		return
	}
	disposition := "attachment"
	if strings.HasPrefix(media.ContentType, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", media.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": media.OriginalName}))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, media.OriginalName, info.ModTime().Round(time.Second), file)
}

func safeMediaName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == ".." {
		return ""
	}
	runes := []rune(name)
	if len(runes) > 180 {
		runes = runes[:180]
		name = string(runes)
	}
	for len(name) > 255 && len(runes) > 0 {
		runes = runes[:len(runes)-1]
		name = string(runes)
	}
	return name
}
