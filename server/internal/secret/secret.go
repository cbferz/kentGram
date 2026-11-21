// Package secret generates and hashes high-entropy bearer secrets such as
// refresh tokens and bot tokens. It depends only on the standard library.
package secret

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
)

// Token returns "<prefix>_<url-safe random>" built from n random bytes.
// The prefix makes leaked secrets recognisable and lets handlers reject
// obviously wrong values without touching the database.
func Token(prefix string, n int) (string, error) {
	if n < 16 {
		return "", errors.New("secret: token needs at least 16 random bytes")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// Hash returns the SHA-256 of a token. Tokens are random, so a fast hash is
// sufficient: there is no low-entropy input for an attacker to brute-force.
func Hash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Equal compares two secrets in constant time.
func Equal(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// HasPrefix reports whether token looks like one of ours and is a sane size.
func HasPrefix(token, prefix string, maxLen int) bool {
	return len(token) <= maxLen && strings.HasPrefix(token, prefix+"_")
}
