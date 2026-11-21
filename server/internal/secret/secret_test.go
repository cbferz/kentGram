package secret

import (
	"bytes"
	"strings"
	"testing"
)

func TestTokenFormatAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tok, err := Token("kgr", 32)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(tok, "kgr_") {
			t.Fatalf("missing prefix: %q", tok)
		}
		if len(tok) != len("kgr_")+43 { // 32 bytes -> 43 base64url chars
			t.Fatalf("unexpected length %d for %q", len(tok), tok)
		}
		if seen[tok] {
			t.Fatalf("duplicate token generated: %q", tok)
		}
		seen[tok] = true
	}
}

func TestTokenRejectsWeakSize(t *testing.T) {
	if _, err := Token("x", 8); err == nil {
		t.Fatal("expected an error for a token with too little entropy")
	}
}

func TestHashIsDeterministicAndSized(t *testing.T) {
	a, b := Hash("kgr_abc"), Hash("kgr_abc")
	if len(a) != 32 || !bytes.Equal(a, b) {
		t.Fatal("hash must be a stable 32-byte digest")
	}
	if bytes.Equal(a, Hash("kgr_abd")) {
		t.Fatal("different tokens must hash differently")
	}
}

func TestEqual(t *testing.T) {
	if !Equal(Hash("a"), Hash("a")) {
		t.Fatal("equal digests should compare equal")
	}
	if Equal(Hash("a"), Hash("b")) {
		t.Fatal("different digests should not compare equal")
	}
	if Equal([]byte{1}, []byte{1, 2}) {
		t.Fatal("different lengths must not compare equal")
	}
}

func TestHasPrefix(t *testing.T) {
	if !HasPrefix("kgr_abc", "kgr", 64) {
		t.Fatal("valid token rejected")
	}
	if HasPrefix("kgb_abc", "kgr", 64) || HasPrefix("kgrabc", "kgr", 64) {
		t.Fatal("wrong prefix accepted")
	}
	if HasPrefix("kgr_"+strings.Repeat("a", 100), "kgr", 64) {
		t.Fatal("oversized token accepted")
	}
}
