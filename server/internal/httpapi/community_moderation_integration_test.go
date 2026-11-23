package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestCommunityMemberRemovalHierarchy(t *testing.T) {
	api := newTestAPI(t)
	h := api.Routes()
	owner := registerUser(t, h)
	admin := registerUser(t, h)
	otherAdmin := registerUser(t, h)
	member := registerUser(t, h)

	created := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access,
		map[string]any{"kind": "group", "title": "Moderation"})
	expectStatus(t, created, http.StatusCreated)
	chatID := int64(asMap(t, created)["id"].(float64))

	for _, target := range []testUser{admin, otherAdmin, member} {
		rec := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/members", chatID), owner.Access,
			map[string]int64{"user_id": target.ID})
		expectStatus(t, rec, http.StatusCreated)
	}
	for _, target := range []testUser{admin, otherAdmin} {
		rec := call(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/chats/%d/members/%d/role", chatID, target.ID),
			owner.Access, map[string]string{"role": "admin"})
		expectStatus(t, rec, http.StatusOK)
	}

	removeMember := call(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/chats/%d/members/%d", chatID, member.ID), admin.Access, nil)
	expectStatus(t, removeMember, http.StatusOK)

	removeAdminAsAdmin := call(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/chats/%d/members/%d", chatID, otherAdmin.ID), admin.Access, nil)
	expectStatus(t, removeAdminAsAdmin, http.StatusNotFound)

	removeAdminAsOwner := call(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/chats/%d/members/%d", chatID, otherAdmin.ID), owner.Access, nil)
	expectStatus(t, removeAdminAsOwner, http.StatusOK)
}
