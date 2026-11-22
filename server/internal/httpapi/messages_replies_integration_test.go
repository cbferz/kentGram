package httpapi

import (
    "net/http"
    "strconv"
    "testing"
)

func replyPath(chatID int64) string {
    return "/api/v1/chats/" + strconv.FormatInt(chatID, 10) + "/messages"
}

func TestReplyMessageIncludesSameChatPreview(t *testing.T) {
    h := newTestAPI(t).Routes()
    alice := registerUser(t, h)
    bob := registerUser(t, h)

    chat := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access,
        map[string]int64{"user_id": bob.ID})
    expectStatus(t, chat, http.StatusOK)
    chatID := int64(asMap(t, chat)["chat_id"].(float64))
    path := replyPath(chatID)

    original := call(t, h, http.MethodPost, path, alice.Access, map[string]string{"text": "original text"})
    expectStatus(t, original, http.StatusCreated)
    originalID := int64(asMap(t, original)["id"].(float64))

    reply := call(t, h, http.MethodPost, path, bob.Access, map[string]any{
        "text": "reply text", "reply_to_message_id": originalID,
    })
    expectStatus(t, reply, http.StatusCreated)

    history := call(t, h, http.MethodGet, path, alice.Access, nil)
    expectStatus(t, history, http.StatusOK)
    messages := asMap(t, history)["messages"].([]any)
    if len(messages) != 2 { t.Fatalf("expected 2 messages, got %d", len(messages)) }
    got := messages[1].(map[string]any)
    if int64(got["reply_to_message_id"].(float64)) != originalID {
        t.Fatalf("wrong reply target: %#v", got["reply_to_message_id"])
    }
    if got["reply_to_body"] != "original text" {
        t.Fatalf("wrong reply preview: %#v", got["reply_to_body"])
    }
    if got["reply_to_sender_username"] != alice.Username {
        t.Fatalf("wrong reply sender: %#v", got["reply_to_sender_username"])
    }
}

func TestReplyMessageCannotCrossChatsOrQuoteDeletedMessage(t *testing.T) {
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

    original := call(t, h, http.MethodPost, replyPath(firstID), alice.Access,
        map[string]string{"text": "private original"})
    expectStatus(t, original, http.StatusCreated)
    originalID := int64(asMap(t, original)["id"].(float64))

    crossChat := call(t, h, http.MethodPost, replyPath(secondID), alice.Access,
        map[string]any{"text": "cross chat attempt", "reply_to_message_id": originalID})
    expectStatus(t, crossChat, http.StatusNotFound)

    expectStatus(t, call(t, h, http.MethodDelete,
        replyPath(firstID)+"/"+strconv.FormatInt(originalID, 10), alice.Access, nil), http.StatusOK)
    deletedReply := call(t, h, http.MethodPost, replyPath(firstID), bob.Access,
        map[string]any{"text": "reply to deleted", "reply_to_message_id": originalID})
    expectStatus(t, deletedReply, http.StatusNotFound)
}
