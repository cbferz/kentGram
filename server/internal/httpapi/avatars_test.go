package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cbferz/kentgram/server/internal/config"
)

func TestAvatarFileServesPersistentImage(t *testing.T) {
	dir := t.TempDir()
	name := "0123456789abcdef0123456789abcdef.png"
	body := []byte("avatar-test-payload")
	if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
		t.Fatal(err)
	}
	a := &API{cfg: config.Config{AvatarDir: dir}}
	req := httptest.NewRequest(http.MethodGet, "/media/avatars/"+name, nil)
	rec := httptest.NewRecorder()
	a.avatarFile(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q, want image/png", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache control = %q", got)
	}
	if rec.Body.String() != string(body) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), body)
	}
}

func TestAvatarFileRejectsUnsafeNames(t *testing.T) {
	a := &API{cfg: config.Config{AvatarDir: t.TempDir()}}
	for _, name := range []string{"../secret.png", "avatar.svg", "not-a-random-id.png"} {
		req := httptest.NewRequest(http.MethodGet, "/media/avatars/"+name, nil)
		rec := httptest.NewRecorder()
		a.avatarFile(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("name %q: status = %d, want 404", name, rec.Code)
		}
	}
}
