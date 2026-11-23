package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestCommunityChatRolesAndChannelPosting(t *testing.T) {
	api := newTestAPI(t)
	h := api.Routes()
	owner := registerUser(t, h)
	member := registerUser(t, h)

	groupRec := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access, map[string]any{
		"kind": "group", "title": "Engineering", "description": "Private group",
	})
	expectStatus(t, groupRec, http.StatusCreated)
	groupID := int64(asMap(t, groupRec)["id"].(float64))

	addRec := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/members", groupID), owner.Access,
		map[string]int64{"user_id": member.ID})
	expectStatus(t, addRec, http.StatusCreated)

	membersRec := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/chats/%d/members", groupID), member.Access, nil)
	expectStatus(t, membersRec, http.StatusOK)
	members := asMap(t, membersRec)["members"].([]any)
	if len(members) != 2 {
		t.Fatalf("member count = %d, want 2", len(members))
	}

	channelRec := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access, map[string]any{
		"kind": "channel", "title": "Announcements",
	})
	expectStatus(t, channelRec, http.StatusCreated)
	channelID := int64(asMap(t, channelRec)["id"].(float64))

	addChannelMember := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/members", channelID), owner.Access,
		map[string]int64{"user_id": member.ID})
	expectStatus(t, addChannelMember, http.StatusCreated)

	blockedPost := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages", channelID), member.Access,
		map[string]any{"text": "Member cannot publish"})
	expectStatus(t, blockedPost, http.StatusNotFound)

	promote := call(t, h, http.MethodPatch,
		fmt.Sprintf("/api/v1/chats/%d/members/%d/role", channelID, member.ID), owner.Access,
		map[string]string{"role": "admin"})
	expectStatus(t, promote, http.StatusOK)

	adminPost := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages", channelID), member.Access,
		map[string]any{"text": "Admin announcement"})
	expectStatus(t, adminPost, http.StatusCreated)

	chatList := call(t, h, http.MethodGet, "/api/v1/chats", owner.Access, nil)
	expectStatus(t, chatList, http.StatusOK)
	chats := asMap(t, chatList)["chats"].([]any)
	if len(chats) != 2 {
		t.Fatalf("chat list length = %d, want 2 community chats", len(chats))
	}
}

func TestCommunityOwnershipTransfer(t *testing.T) {
	api := newTestAPI(t)
	h := api.Routes()
	owner := registerUser(t, h)
	member := registerUser(t, h)

	created := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access, map[string]any{
		"kind": "group", "title": "Ownership",
	})
	expectStatus(t, created, http.StatusCreated)
	chatID := int64(asMap(t, created)["id"].(float64))

	added := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/members", chatID), owner.Access,
		map[string]int64{"user_id": member.ID})
	expectStatus(t, added, http.StatusCreated)

	left := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/leave", chatID), owner.Access, nil)
	expectStatus(t, left, http.StatusConflict)

	transferred := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/transfer-ownership", chatID), owner.Access,
		map[string]int64{"user_id": member.ID})
	expectStatus(t, transferred, http.StatusOK)

	left = call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/leave", chatID), owner.Access, nil)
	expectStatus(t, left, http.StatusOK)

	details := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/chats/%d", chatID), member.Access, nil)
	expectStatus(t, details, http.StatusOK)
	if asMap(t, details)["role"] != "owner" {
		t.Fatalf("new owner role = %v, want owner", asMap(t, details)["role"])
	}
}
