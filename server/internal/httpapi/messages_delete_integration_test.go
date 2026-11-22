package httpapi

import (
    "net/http"
    "strconv"
    "testing"
)

func messageID(v int64) string { return strconv.FormatInt(v, 10) }

func TestDeleteChatMessageIsScopedToSenderAndChat(t *testing.T) {
    h := newTestAPI(t).Routes()
    alice := registerUser(t, h)
    bob := registerUser(t, h)

    chatRec := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access,
        map[string]int64{"user_id": bob.ID})
    expectStatus(t, chatRec, http.StatusOK)
    chatID := int64(asMap(t, chatRec)["chat_id"].(float64))
    path := "/api/v1/chats/" + messageID(chatID) + "/messages"

    bobMessage := call(t, h, http.MethodPost, path, bob.Access, map[string]string{"text": "bob's message"})
    expectStatus(t, bobMessage, http.StatusCreated)
    bobMessageID := int64(asMap(t, bobMessage)["id"].(float64))
    expectStatus(t, call(t, h, http.MethodDelete, path+"/"+messageID(bobMessageID), alice.Access, nil),
        http.StatusNotFound)

    aliceMessage := call(t, h, http.MethodPost, path, alice.Access, map[string]string{"text": "alice's message"})
    expectStatus(t, aliceMessage, http.StatusCreated)
    aliceMessageID := int64(asMap(t, aliceMessage)["id"].(float64))

    deleted := call(t, h, http.MethodDelete, path+"/"+messageID(aliceMessageID), alice.Access, nil)
    expectStatus(t, deleted, http.StatusOK)
    if asMap(t, deleted)["deleted"] != true { t.Fatal("delete response must confirm deletion") }

    history := call(t, h, http.MethodGet, path, bob.Access, nil)
    expectStatus(t, history, http.StatusOK)
    for _, raw := range asMap(t, history)["messages"].([]any) {
        if int64(raw.(map[string]any)["id"].(float64)) == aliceMessageID {
            t.Fatal("deleted message is still visible in chat history")
        }
    }

    // Repeating the request is idempotent.
    expectStatus(t, call(t, h, http.MethodDelete, path+"/"+messageID(aliceMessageID), alice.Access, nil),
        http.StatusOK)
    expectStatus(t, call(t, h, http.MethodDelete, path+"/"+messageID(aliceMessageID), "", nil),
        http.StatusUnauthorized)
}

func TestDeleteChatMessageCannotCrossChatBoundary(t *testing.T) {
    h := newTestAPI(t).Routes()
    alice := registerUser(t, h)
    bob := registerUser(t, h)
    carol := registerUser(t, h)

    first := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access,
        map[string]int64{"user_id": bob.ID})
    second := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access,
        map[string]int64{"user_id": carol.ID})
    expectStatus(t, first, http.StatusOK)
    expectStatus(t, second, http.StatusOK)
    firstID := int64(asMap(t, first)["chat_id"].(float64))
    secondID := int64(asMap(t, second)["chat_id"].(float64))

    sent := call(t, h, http.MethodPost, "/api/v1/chats/"+messageID(firstID)+"/messages",
        alice.Access, map[string]string{"text": "only in first chat"})
    expectStatus(t, sent, http.StatusCreated)
    id := int64(asMap(t, sent)["id"].(float64))

    crossChat := call(t, h, http.MethodDelete,
        "/api/v1/chats/"+messageID(secondID)+"/messages/"+messageID(id), alice.Access, nil)
    expectStatus(t, crossChat, http.StatusNotFound)

    original := call(t, h, http.MethodGet, "/api/v1/chats/"+messageID(firstID)+"/messages", bob.Access, nil)
    expectStatus(t, original, http.StatusOK)
    if len(asMap(t, original)["messages"].([]any)) != 1 {
        t.Fatal("cross-chat delete changed the original message")
    }
}
