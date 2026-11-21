// Package blobstore stores file contents on local disk, addressed by their
// SHA-256 digest. Identical content is stored once. It depends only on the
// standard library, and the Store interface lets an object-storage backend
// replace the disk later without touching callers.
package blobstore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var (
	ErrTooLarge   = errors.New("blobstore: content exceeds the size limit")
	ErrInvalidKey = errors.New("blobstore: invalid storage key")
	ErrNotFound   = errors.New("blobstore: blob not found")
)

// Blob describes stored content.
type Blob struct {
	Key    string   // "ab/<64 hex>", safe to use as a database value
	Size   int64    // bytes
	SHA256 [32]byte // content digest
}

// Store is the interface the rest of the server depends on.
type Store interface {
	// Put streams r into storage, failing with ErrTooLarge past maxBytes.
	Put(r io.Reader, maxBytes int64) (Blob, error)
	// Open returns the content for key. Callers must Close the result.
	Open(key string) (*os.File, error)
	// Delete removes the content for key. Deleting a missing key is not an error.
	Delete(key string) error
}

// Disk keeps blobs under a root directory, two levels deep to keep directory
// listings small.
type Disk struct {
	root string
	tmp  string
}

var keyPattern = regexp.MustCompile(`^[0-9a-f]{2}/[0-9a-f]{64}$`)

// ValidKey reports whether key has the exact shape this package generates.
// Checking the shape, rather than sanitising, rules out path traversal.
func ValidKey(key string) bool { return keyPattern.MatchString(key) }

func NewDisk(root string) (*Disk, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	d := &Disk{root: abs, tmp: filepath.Join(abs, ".tmp")}
	if err := os.MkdirAll(d.tmp, 0o750); err != nil {
		return nil, fmt.Errorf("create blob directory: %w", err)
	}
	return d, nil
}

func (d *Disk) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", ErrInvalidKey
	}
	return filepath.Join(d.root, filepath.FromSlash(key)), nil
}

func (d *Disk) Put(r io.Reader, maxBytes int64) (Blob, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Blob{}, err
	}
	tmpPath := filepath.Join(d.tmp, hex.EncodeToString(nonce[:]))
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return Blob{}, err
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmpPath)
	}

	hasher := sha256.New()
	// Read one byte past the limit so an over-long stream is detected rather
	// than silently truncated to exactly maxBytes.
	n, err := io.Copy(io.MultiWriter(f, hasher), io.LimitReader(r, maxBytes+1))
	if err != nil {
		cleanup()
		return Blob{}, err
	}
	if n > maxBytes {
		cleanup()
		return Blob{}, ErrTooLarge
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return Blob{}, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return Blob{}, err
	}

	var sum [32]byte
	copy(sum[:], hasher.Sum(nil))
	digest := hex.EncodeToString(sum[:])
	key := digest[:2] + "/" + digest

	final, err := d.path(key)
	if err != nil {
		_ = os.Remove(tmpPath)
		return Blob{}, err
	}
	if _, err := os.Stat(final); err == nil {
		// Identical content is already stored; keep a single copy.
		_ = os.Remove(tmpPath)
		return Blob{Key: key, Size: n, SHA256: sum}, nil
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o750); err != nil {
		_ = os.Remove(tmpPath)
		return Blob{}, err
	}
	if err := os.Rename(tmpPath, final); err != nil {
		_ = os.Remove(tmpPath)
		return Blob{}, err
	}
	return Blob{Key: key, Size: n, SHA256: sum}, nil
}

func (d *Disk) Open(key string) (*os.File, error) {
	p, err := d.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

func (d *Disk) Delete(key string) error {
	p, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
