// Package mediatype decides what an uploaded file really is by looking at its
// bytes, never at the content type or extension claimed by the client, and
// checks that it fits the kind of media it was uploaded as. It depends only on
// the standard library.
package mediatype

import (
	"bytes"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/cbferz/kentgram/server/internal/fileid"
)

const (
	MIMEJPEG      = "image/jpeg"
	MIMEPNG       = "image/png"
	MIMEWebP      = "image/webp"
	MIMEGIF       = "image/gif"
	MIMEMP4       = "video/mp4"
	MIMEWebM      = "video/webm"
	MIMEQuickTime = "video/quicktime"
	MIMEMP3       = "audio/mpeg"
	MIMEM4A       = "audio/mp4"
	MIMEOgg       = "audio/ogg"
	MIMEWav       = "audio/wav"
	MIMEFlac      = "audio/flac"
	MIMETGS       = "application/x-tgsticker"
	MIMEOctet     = "application/octet-stream"
)

// SniffLen is how many leading bytes Sniff needs.
const SniffLen = 512

// Sniff returns the canonical MIME type of the file whose first bytes are
// head. filename is only consulted to tell apart formats that share a
// container (a gzip file named *.tgs is a Lottie sticker).
func Sniff(head []byte, filename string) string {
	if m := sniffISOBMFF(head); m != "" {
		return m
	}
	if bytes.HasPrefix(head, []byte("fLaC")) {
		return MIMEFlac
	}
	mime, _, _ := strings.Cut(http.DetectContentType(head), ";")
	switch mime {
	case "application/ogg", "video/ogg":
		return MIMEOgg
	case "audio/wave":
		return MIMEWav
	case "application/x-gzip":
		if strings.EqualFold(path.Ext(filename), ".tgs") {
			return MIMETGS
		}
		return "application/gzip"
	}
	return mime
}

// sniffISOBMFF recognises MP4-family containers by their ftyp brand, which the
// standard library only partly understands (QuickTime and M4A are missed).
func sniffISOBMFF(head []byte) string {
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return ""
	}
	switch strings.TrimSpace(string(head[8:12])) {
	case "qt":
		return MIMEQuickTime
	case "M4A", "M4B":
		return MIMEM4A
	}
	return MIMEMP4
}

var allowed = map[fileid.Kind][]string{
	fileid.Photo:     {MIMEJPEG, MIMEPNG, MIMEWebP},
	fileid.Video:     {MIMEMP4, MIMEWebM, MIMEQuickTime},
	fileid.Animation: {MIMEGIF, MIMEMP4, MIMEWebM},
	fileid.Audio:     {MIMEMP3, MIMEM4A, MIMEOgg, MIMEWav, MIMEFlac, MIMEWebM},
	fileid.Voice:     {MIMEOgg, MIMEMP3, MIMEM4A, MIMEWebM},
	fileid.VideoNote: {MIMEMP4},
	fileid.Sticker:   {MIMEWebP, MIMETGS, MIMEWebM},
	fileid.Thumbnail: {MIMEJPEG, MIMEPNG, MIMEWebP},
}

// CheckKind reports whether a file sniffed as mime may be stored as kind.
// Documents accept anything because they are only ever served as downloads.
func CheckKind(kind fileid.Kind, mime string) error {
	if kind == fileid.Document {
		return nil
	}
	list, ok := allowed[kind]
	if !ok {
		return fmt.Errorf("unsupported media kind %q", kind)
	}
	for _, m := range list {
		if m == mime {
			return nil
		}
	}
	return fmt.Errorf("a %s cannot be stored as %s", mime, kind)
}

// MaxSize is the largest upload accepted for a kind and content type.
func MaxSize(kind fileid.Kind, mime string) int64 {
	const mb = 1 << 20
	switch {
	case kind == fileid.Thumbnail:
		return 512 << 10
	case kind == fileid.Photo:
		return 10 * mb
	case kind == fileid.Sticker && mime == MIMETGS:
		return 64 << 10
	case kind == fileid.Sticker && mime == MIMEWebM:
		return 256 << 10
	case kind == fileid.Sticker:
		return 512 << 10
	case kind == fileid.Voice:
		return 20 * mb
	case kind == fileid.VideoNote:
		return 20 * mb
	}
	return 50 * mb
}

// MaxUpload is the largest size any kind may reach; it bounds the request body
// before the kind-specific limit is known.
const MaxUpload int64 = 50<<20 + 1<<20

// InlineSafe reports whether a type can be rendered by clients and browsers
// without risk. Everything else is served as an opaque attachment, so an
// uploaded HTML page or script can never execute from our origin.
func InlineSafe(mime string) bool {
	switch mime {
	case MIMEJPEG, MIMEPNG, MIMEWebP, MIMEGIF,
		MIMEMP4, MIMEWebM, MIMEQuickTime,
		MIMEMP3, MIMEM4A, MIMEOgg, MIMEWav, MIMEFlac:
		return true
	}
	return false
}

// ServedType is the Content-Type to send for a stored file.
func ServedType(mime string) string {
	if InlineSafe(mime) {
		return mime
	}
	return MIMEOctet
}

// IsImage reports whether mime is a still or animated image format the
// imaging package can decode.
func IsImage(mime string) bool {
	switch mime {
	case MIMEJPEG, MIMEPNG, MIMEWebP, MIMEGIF:
		return true
	}
	return false
}

// SafeFileName strips directories and control characters and bounds the length
// so the name is safe to echo in a Content-Disposition header.
func SafeFileName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f || r == '"' || r == ';':
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "." || out == "/" || out == ".." {
		return ""
	}
	if r := []rune(out); len(r) > 120 {
		out = string(r[:120])
	}
	return out
}
