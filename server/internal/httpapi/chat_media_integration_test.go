package httpapi

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func makeTestPNG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 40, G: 120, B: 220, A: 255})
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func uploadChatMediaRequest(t *testing.T, handler http.Handler, path, token, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("caption", "test attachment"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestChatMediaUploadAndMembershipScopedDownload(t *testing.T) {
	api := newTestAPI(t)
	handler := api.Routes()
	owner := registerUser(t, handler)
	peer := registerUser(t, handler)
	outsider := registerUser(t, handler)

	created := call(t, handler, http.MethodPost, "/api/v1/chats/direct", owner.Access,
		map[string]int64{"user_id": peer.ID})
	expectStatus(t, created, http.StatusOK)
	chatID := int64(asMap(t, created)["chat_id"].(float64))

	uploaded := uploadChatMediaRequest(t, handler, fmt.Sprintf("/api/v1/chats/%d/media", chatID),
		owner.Access, "photo.png", makeTestPNG(t))
	expectStatus(t, uploaded, http.StatusCreated)
	message := asMap(t, uploaded)
	mediaID, ok := message["media_id"].(string)
	if !ok || len(mediaID) != 32 {
		t.Fatalf("invalid media ID: %v", message["media_id"])
	}
	if message["media_mime"] != "image/png" || message["media_name"] != "photo.png" {
		t.Fatalf("unexpected media metadata: %+v", message)
	}

	downloadPath := fmt.Sprintf("/api/v1/chats/%d/media/%s", chatID, mediaID)
	download := call(t, handler, http.MethodGet, downloadPath, peer.Access, nil)
	expectStatus(t, download, http.StatusOK)
	if !bytes.Equal(download.Body.Bytes(), makeTestPNG(t)) {
		t.Fatal("downloaded media does not match uploaded content")
	}

	forbidden := call(t, handler, http.MethodGet, downloadPath, outsider.Access, nil)
	expectStatus(t, forbidden, http.StatusNotFound)
}
