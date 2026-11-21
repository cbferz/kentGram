package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestFollowAndUnfollow(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice := registerUser(t, h)
	bob := registerUser(t, h)
	bobPath := "/api/v1/users/" + strconv.FormatInt(bob.ID, 10) + "/follow"

	expectStatus(t, call(t, h, http.MethodPost, bobPath, alice.Access, nil), http.StatusOK)
	// Following twice is harmless.
	expectStatus(t, call(t, h, http.MethodPost, bobPath, alice.Access, nil), http.StatusOK)

	profile := asMap(t, call(t, h, http.MethodGet, "/api/v1/users/"+bob.Username, alice.Access, nil))
	if profile["followers"] != float64(1) || profile["is_following"] != true {
		t.Fatalf("bob should have one follower, seen as followed by alice: %v", profile)
	}
	asBob := asMap(t, call(t, h, http.MethodGet, "/api/v1/users/"+bob.Username, bob.Access, nil))
	if asBob["is_following"] != false {
		t.Fatalf("bob does not follow himself: %v", asBob)
	}
	aliceProfile := asMap(t, call(t, h, http.MethodGet, "/api/v1/users/"+alice.Username, bob.Access, nil))
	if aliceProfile["following"] != float64(1) {
		t.Fatalf("alice should be following one account: %v", aliceProfile)
	}

	expectStatus(t, call(t, h, http.MethodDelete, bobPath, alice.Access, nil), http.StatusOK)
	after := asMap(t, call(t, h, http.MethodGet, "/api/v1/users/"+bob.Username, alice.Access, nil))
	if after["followers"] != float64(0) || after["is_following"] != false {
		t.Fatalf("unfollow did not take effect: %v", after)
	}

	selfPath := "/api/v1/users/" + strconv.FormatInt(alice.ID, 10) + "/follow"
	expectStatus(t, call(t, h, http.MethodPost, selfPath, alice.Access, nil), http.StatusBadRequest)
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/users/999999999/follow", alice.Access, nil), http.StatusNotFound)
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/users/0/follow", alice.Access, nil), http.StatusBadRequest)
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/users/x/follow", alice.Access, nil), http.StatusBadRequest)
	expectStatus(t, call(t, h, http.MethodPost, bobPath, "", nil), http.StatusUnauthorized)
}

func TestUserSearch(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice := registerUser(t, h)
	bob := registerUser(t, h)

	search := func(q string) []any {
		rec := call(t, h, http.MethodGet, "/api/v1/users/search?q="+url.QueryEscape(q), alice.Access, nil)
		expectStatus(t, rec, http.StatusOK)
		return asMap(t, rec)["users"].([]any)
	}

	found := search(bob.Username)
	if len(found) != 1 || found[0].(map[string]any)["username"] != bob.Username {
		t.Fatalf("expected to find exactly bob: %v", found)
	}
	if got := search(strings.ToUpper(bob.Username)); len(got) != 1 {
		t.Fatalf("search must be case-insensitive: %v", got)
	}
	// The searcher never appears in their own results.
	for _, u := range search(alice.Username) {
		if u.(map[string]any)["username"] == alice.Username {
			t.Fatal("search returned the requesting user")
		}
	}
	// LIKE wildcards are literal characters, not a way to list every account.
	for _, wildcard := range []string{"%%", "__", "%_%"} {
		if got := search(wildcard); len(got) != 0 {
			t.Fatalf("wildcard %q matched %d users", wildcard, len(got))
		}
	}

	for _, bad := range []string{"", "a", strings.Repeat("x", 65)} {
		rec := call(t, h, http.MethodGet, "/api/v1/users/search?q="+url.QueryEscape(bad), alice.Access, nil)
		expectStatus(t, rec, http.StatusBadRequest)
	}
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/users/search?q=ab", "", nil), http.StatusUnauthorized)
}

func TestProfileUpdate(t *testing.T) {
	h := newTestAPI(t).Routes()
	u := registerUser(t, h)

	rec := call(t, h, http.MethodPatch, "/api/v1/me/profile", u.Access, map[string]string{
		"display_name": "  Alice Liddell ", "bio": "hello", "avatar_url": "https://cdn.example.test/a.png"})
	expectStatus(t, rec, http.StatusOK)
	m := asMap(t, rec)
	if m["display_name"] != "Alice Liddell" || m["bio"] != "hello" {
		t.Fatalf("profile was not saved/trimmed: %v", m)
	}

	for name, body := range map[string]map[string]string{
		"plain http avatar": {"display_name": "x", "bio": "", "avatar_url": "http://example.test/a.png"},
		"javascript avatar": {"display_name": "x", "bio": "", "avatar_url": "javascript:alert(1)"},
		"bio too long":      {"display_name": "x", "bio": strings.Repeat("b", 161), "avatar_url": ""},
		"name too long":     {"display_name": strings.Repeat("n", 65), "bio": "", "avatar_url": ""},
	} {
		if rec := call(t, h, http.MethodPatch, "/api/v1/me/profile", u.Access, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
}
