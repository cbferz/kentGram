package httpapi

import (
    "net/http"
    "strconv"
    "testing"
)

func chatMessagePath(chatID int64) string {
    return "/api/v1/chats/" + strconv.FormatInt(chatID, 10) + "/messages"
}

func TestEditMessageIsOwnerAndChatScoped(t *testing.T) {
    h := newTestAPI(t).Routes()
    alice := registerUser(t, h)
    bob := registerUser(t, h)
    chat := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access, map[string]int64{"user_id": bob.ID})
    expectStatus(t, chat, http.StatusOK)
    chatID := int64(asMap(t, chat)["chat_id"].(float64))
    path := chatMessagePath(chatID)
    sent := call(t, h, http.MethodPost, path, alice.Access, map[string]string{"text":"before edit"})
    expectStatus(t, sent, http.StatusCreated)
    id := int64(asMap(t, sent)["id"].(float64))
    messagePath := path + "/" + strconv.FormatInt(id, 10)

    forbidden := call(t, h, http.MethodPut, messagePath, bob.Access, map[string]string{"text":"hijack"})
    expectStatus(t, forbidden, http.StatusNotFound)

    edited := call(t, h, http.MethodPut, messagePath, alice.Access, map[string]string{"text":"after edit"})
    expectStatus(t, edited, http.StatusOK)
    got := asMap(t, edited)
    if got["body"] != "after edit" { t.Fatalf("edited body = %#v", got["body"]) }
    if got["edited_at"] == nil { t.Fatal("edited_at must be returned") }

    history := call(t, h, http.MethodGet, path, bob.Access, nil)
    expectStatus(t, history, http.StatusOK)
    items := asMap(t, history)["messages"].([]any)
    if len(items) != 1 || items[0].(map[string]any)["body"] != "after edit" {
        t.Fatalf("edited message not reflected in history: %#v", items)
    }
}

func TestMessageReactionsToggleAndStayChatScoped(t *testing.T) {
    h := newTestAPI(t).Routes()
    alice := registerUser(t, h)
    bob := registerUser(t, h)
    chat := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access, map[string]int64{"user_id": bob.ID})
    expectStatus(t, chat, http.StatusOK)
    chatID := int64(asMap(t, chat)["chat_id"].(float64))
    path := chatMessagePath(chatID)
    sent := call(t, h, http.MethodPost, path, alice.Access, map[string]string{"text":"react here"})
    expectStatus(t, sent, http.StatusCreated)
    id := int64(asMap(t, sent)["id"].(float64))
    reactionPath := path + "/" + strconv.FormatInt(id, 10) + "/reactions"

    expectStatus(t, call(t, h, http.MethodPost, reactionPath, bob.Access, map[string]string{"emoji":"❤️"}), http.StatusOK)
    history := call(t, h, http.MethodGet, path, alice.Access, nil)
    expectStatus(t, history, http.StatusOK)
    items := asMap(t, history)["messages"].([]any)
    reactions := items[0].(map[string]any)["reactions"].([]any)
    if len(reactions) != 1 { t.Fatalf("expected one reaction group, got %#v", reactions) }
    group := reactions[0].(map[string]any)
    if group["emoji"] != "❤️" || int(group["count"].(float64)) != 1 || group["mine"] != false {
        t.Fatalf("unexpected reaction summary: %#v", group)
    }

    expectStatus(t, call(t, h, http.MethodPost, reactionPath, bob.Access, map[string]string{"emoji":"❤️"}), http.StatusOK)
    history = call(t, h, http.MethodGet, path, alice.Access, nil)
    items = asMap(t, history)["messages"].([]any)
    reactions = items[0].(map[string]any)["reactions"].([]any)
    if len(reactions) != 0 { t.Fatalf("reaction toggle should remove it: %#v", reactions) }

    invalid := call(t, h, http.MethodPost, reactionPath, bob.Access, map[string]string{"emoji":"not-emoji"})
    expectStatus(t, invalid, http.StatusBadRequest)
}
