// Package fileid creates and validates the identifiers used to refer to
// uploaded media. It depends only on the standard library.
//
// Two identifiers exist for every file, modelled on the Telegram Bot API:
//
//   - file_id is an opaque, unguessable handle. It is used to download a file
//     and to send it again without uploading it a second time. Possessing a
//     file_id is never sufficient on its own: the server still checks that the
//     caller is allowed to see the file.
//   - file_unique_id is stable for identical content across uploads and users.
//     It cannot be used to fetch anything and is only meant for recognising
//     "this is the same file". It is a keyed hash, so it does not reveal the
//     raw content digest.
package fileid

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

type Kind string

const (
	Photo     Kind = "photo"
	Video     Kind = "video"
	Animation Kind = "animation"
	Audio     Kind = "audio"
	Voice     Kind = "voice"
	VideoNote Kind = "video_note"
	Document  Kind = "document"
	Sticker   Kind = "sticker"
	Thumbnail Kind = "thumbnail"
)

var prefixes = map[Kind]string{
	Photo: "ph", Video: "vd", Animation: "an", Audio: "au", Voice: "vo",
	VideoNote: "vn", Document: "dc", Sticker: "st", Thumbnail: "th",
}

var kindByPrefix = func() map[string]Kind {
	m := make(map[string]Kind, len(prefixes))
	for k, p := range prefixes {
		m[p] = k
	}
	return m
}()

const (
	randomBytes = 24         // 24 bytes -> 32 base64url characters
	idLen       = 2 + 1 + 32 // "ph" + "_" + 32
	uniqueLen   = 1 + 21     // "u" + 21
	uniquePfx   = "u"
	alphabet    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
)

// Valid reports whether k is a known media kind.
func (k Kind) Valid() bool {
	_, ok := prefixes[k]
	return ok
}

// New returns a fresh random file_id such as "ph_AbC...". The two-letter prefix
// only aids debugging and cheap rejection of malformed input.
func New(k Kind) (string, error) {
	p, ok := prefixes[k]
	if !ok {
		return "", errors.New("fileid: unknown kind " + string(k))
	}
	buf := make([]byte, randomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return p + "_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// Parse validates the shape of a file_id and returns its kind. It performs no
// authorisation; it only lets handlers reject garbage before touching storage.
func Parse(id string) (Kind, bool) {
	if len(id) != idLen || id[2] != '_' {
		return "", false
	}
	k, ok := kindByPrefix[id[:2]]
	if !ok {
		return "", false
	}
	for i := 3; i < len(id); i++ {
		if !inAlphabet(id[i]) {
			return "", false
		}
	}
	return k, true
}

// Unique derives the stable file_unique_id from the content's SHA-256 digest.
// The key must be a server secret so the result cannot be used to confirm a
// guess about a file's contents.
func Unique(key, sha256Digest []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(sha256Digest)
	return uniquePfx + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:uniqueLen-1]
}

// ValidUnique reports whether s looks like a file_unique_id.
func ValidUnique(s string) bool {
	if len(s) != uniqueLen || s[:1] != uniquePfx {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !inAlphabet(s[i]) {
			return false
		}
	}
	return true
}

func inAlphabet(c byte) bool {
	for i := 0; i < len(alphabet); i++ {
		if alphabet[i] == c {
			return true
		}
	}
	return false
}
