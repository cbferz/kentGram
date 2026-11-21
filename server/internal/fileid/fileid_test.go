package fileid

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func TestNewRoundTripsThroughParse(t *testing.T) {
	seen := map[string]bool{}
	for k := range prefixes {
		for i := 0; i < 50; i++ {
			id, err := New(k)
			if err != nil {
				t.Fatal(err)
			}
			if len(id) != idLen {
				t.Fatalf("unexpected length %d for %q", len(id), id)
			}
			got, ok := Parse(id)
			if !ok || got != k {
				t.Fatalf("Parse(%q) = %q,%v; want %q,true", id, got, ok, k)
			}
			if seen[id] {
				t.Fatalf("duplicate file_id %q", id)
			}
			seen[id] = true
		}
	}
}

func TestNewRejectsUnknownKind(t *testing.T) {
	if _, err := New(Kind("malware")); err == nil {
		t.Fatal("expected an error for an unknown kind")
	}
	if Kind("malware").Valid() || !Photo.Valid() {
		t.Fatal("Valid() disagrees with the known kinds")
	}
}

func TestParseRejectsMalformedIDs(t *testing.T) {
	good, _ := New(Photo)
	bad := []string{
		"",
		"ph_",
		good[:len(good)-1], // too short
		good + "A",         // too long
		"zz" + good[2:],    // unknown prefix
		"ph-" + good[3:],   // wrong separator
		"ph_" + strings.Repeat("!", 32),
		"ph_" + strings.Repeat("a", 31) + "/",
		"../../etc/passwd",
		"ph_" + strings.Repeat("a", 31) + "\x00",
	}
	for _, id := range bad {
		if _, ok := Parse(id); ok {
			t.Errorf("Parse accepted malformed id %q", id)
		}
	}
}

func TestUniqueIsStableKeyedAndShort(t *testing.T) {
	digest := sha256.Sum256([]byte("same content"))
	other := sha256.Sum256([]byte("other content"))

	a := Unique([]byte("key-one"), digest[:])
	if a != Unique([]byte("key-one"), digest[:]) {
		t.Fatal("same key and content must give the same file_unique_id")
	}
	if a == Unique([]byte("key-one"), other[:]) {
		t.Fatal("different content must give a different file_unique_id")
	}
	if a == Unique([]byte("key-two"), digest[:]) {
		t.Fatal("the server key must influence the result")
	}
	if !ValidUnique(a) {
		t.Fatalf("generated id %q does not validate", a)
	}
	if strings.Contains(a, "=") || len(a) != uniqueLen {
		t.Fatalf("unexpected format %q", a)
	}
	for _, bad := range []string{"", "x" + a[1:], a + "a", a[:5], "u" + strings.Repeat("!", 21)} {
		if ValidUnique(bad) {
			t.Errorf("ValidUnique accepted %q", bad)
		}
	}
}
