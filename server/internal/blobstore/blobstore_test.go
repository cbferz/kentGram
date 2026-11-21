package blobstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newDisk(t *testing.T) *Disk {
	t.Helper()
	d, err := NewDisk(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func readAll(t *testing.T, d *Disk, key string) []byte {
	t.Helper()
	f, err := d.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPutOpenRoundTripAndDigest(t *testing.T) {
	d := newDisk(t)
	data := []byte("hello kentgram")
	blob, err := d.Put(bytes.NewReader(data), 1024)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(data)
	if blob.SHA256 != want || blob.Size != int64(len(data)) {
		t.Fatalf("unexpected blob metadata: %+v", blob)
	}
	hexDigest := hex.EncodeToString(want[:])
	if blob.Key != hexDigest[:2]+"/"+hexDigest || !ValidKey(blob.Key) {
		t.Fatalf("storage key is not content addressed: %q", blob.Key)
	}
	if got := readAll(t, d, blob.Key); !bytes.Equal(got, data) {
		t.Fatalf("content mismatch: %q", got)
	}
}

func TestIdenticalContentIsStoredOnce(t *testing.T) {
	d := newDisk(t)
	a, err := d.Put(strings.NewReader("same bytes"), 100)
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Put(strings.NewReader("same bytes"), 100)
	if err != nil {
		t.Fatal(err)
	}
	if a.Key != b.Key {
		t.Fatal("identical content must map to the same key")
	}
	tmp, _ := os.ReadDir(d.tmp)
	if len(tmp) != 0 {
		t.Fatalf("temporary files were left behind: %d", len(tmp))
	}
}

func TestPutEnforcesSizeLimitAndCleansUp(t *testing.T) {
	d := newDisk(t)
	if _, err := d.Put(strings.NewReader(strings.Repeat("x", 11)), 10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	if _, err := d.Put(strings.NewReader(strings.Repeat("x", 10)), 10); err != nil {
		t.Fatalf("content exactly at the limit must be accepted: %v", err)
	}
	// Nothing from the rejected upload may remain.
	tmp, _ := os.ReadDir(d.tmp)
	if len(tmp) != 0 {
		t.Fatalf("rejected upload left %d temporary files", len(tmp))
	}
	var count int
	_ = filepath.WalkDir(d.root, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			count++
		}
		return nil
	})
	if count != 1 {
		t.Fatalf("expected exactly one stored blob, found %d", count)
	}
}

type failingReader struct{ n int }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n == 0 {
		return 0, errors.New("connection reset")
	}
	f.n--
	p[0] = 'x'
	return 1, nil
}

func TestPutCleansUpWhenTheStreamBreaks(t *testing.T) {
	d := newDisk(t)
	if _, err := d.Put(&failingReader{n: 5}, 100); err == nil {
		t.Fatal("expected the read error to be returned")
	}
	if tmp, _ := os.ReadDir(d.tmp); len(tmp) != 0 {
		t.Fatalf("a broken upload left %d temporary files", len(tmp))
	}
}

func TestKeysAreValidatedAgainstTraversal(t *testing.T) {
	d := newDisk(t)
	good := strings.Repeat("a", 64)
	bad := []string{
		"", "../etc/passwd", "ab/../../etc/passwd", "/etc/passwd",
		"ab/" + good[:63], "ab/" + good + "0", "AB/" + strings.ToUpper(good),
		"ab\\" + good, "ab/" + good[:63] + "/", "a/" + good,
	}
	for _, k := range bad {
		if ValidKey(k) {
			t.Errorf("ValidKey accepted %q", k)
		}
		if _, err := d.Open(k); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Open(%q) error = %v, want ErrInvalidKey", k, err)
		}
		if err := d.Delete(k); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Delete(%q) error = %v, want ErrInvalidKey", k, err)
		}
	}
	if !ValidKey("ab/" + good) {
		t.Error("a well-formed key was rejected")
	}
}

func TestDeleteAndMissing(t *testing.T) {
	d := newDisk(t)
	blob, _ := d.Put(strings.NewReader("to delete"), 100)
	if err := d.Delete(blob.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Open(blob.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := d.Delete(blob.Key); err != nil {
		t.Fatalf("deleting a missing blob must not fail: %v", err)
	}
}

func TestConcurrentPutsOfTheSameContent(t *testing.T) {
	d := newDisk(t)
	var wg sync.WaitGroup
	keys := make(chan string, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := d.Put(strings.NewReader("racing writers"), 100)
			if err != nil {
				t.Error(err)
				return
			}
			keys <- b.Key
		}()
	}
	wg.Wait()
	close(keys)
	var first string
	for k := range keys {
		if first == "" {
			first = k
		}
		if k != first {
			t.Fatal("concurrent writers produced different keys")
		}
	}
	if got := readAll(t, d, first); string(got) != "racing writers" {
		t.Fatalf("content corrupted by concurrent writes: %q", got)
	}
}
