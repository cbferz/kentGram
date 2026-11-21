package mediatype

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/cbferz/kentgram/server/internal/fileid"
)

func ftyp(brand string) []byte {
	b := []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p'}
	b = append(b, []byte(brand)...)
	b = append(b, 0, 0, 0, 0, 'i', 's', 'o', 'm', 'm', 'p', '4', '2')
	return b
}

func gz(t *testing.T) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write([]byte(`{"v":"5.5.2"}`))
	_ = w.Close()
	return buf.Bytes()
}

func TestSniff(t *testing.T) {
	webp := append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
	cases := []struct {
		name string
		head []byte
		file string
		want string
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, "a.jpg", MIMEJPEG},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), "a.png", MIMEPNG},
		{"gif", []byte("GIF89a\x01\x00\x01\x00"), "a.gif", MIMEGIF},
		{"webp", webp, "a.webp", MIMEWebP},
		{"mp4", ftyp("isom"), "a.mp4", MIMEMP4},
		{"quicktime", ftyp("qt  "), "a.mov", MIMEQuickTime},
		{"m4a", ftyp("M4A "), "a.m4a", MIMEM4A},
		{"webm", []byte{0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81, 0x01}, "a.webm", MIMEWebM},
		{"mp3 id3", []byte("ID3\x03\x00\x00\x00\x00\x00\x00"), "a.mp3", MIMEMP3},
		{"ogg", []byte("OggS\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00"), "a.ogg", MIMEOgg},
		{"wav", []byte("RIFF\x24\x00\x00\x00WAVEfmt "), "a.wav", MIMEWav},
		{"flac", []byte("fLaC\x00\x00\x00\x22"), "a.flac", MIMEFlac},
		{"pdf", []byte("%PDF-1.7\n"), "a.pdf", "application/pdf"},
		{"zip", []byte("PK\x03\x04\x14\x00\x00\x00"), "a.zip", "application/zip"},
		{"tgs by extension", gz(t), "sticker.tgs", MIMETGS},
		{"plain gzip", gz(t), "data.gz", "application/gzip"},
		{"html is not an image", []byte("<html><script>alert(1)</script>"), "evil.png", "text/html"},
		{"svg script is text", []byte("<?xml version=\"1.0\"?><svg onload=alert(1)>"), "a.svg", "text/xml"},
	}
	for _, c := range cases {
		if got := Sniff(c.head, c.file); got != c.want {
			t.Errorf("%s: Sniff = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCheckKind(t *testing.T) {
	ok := []struct {
		k fileid.Kind
		m string
	}{
		{fileid.Photo, MIMEJPEG}, {fileid.Photo, MIMEPNG}, {fileid.Photo, MIMEWebP},
		{fileid.Video, MIMEMP4}, {fileid.Animation, MIMEGIF}, {fileid.Animation, MIMEMP4},
		{fileid.Voice, MIMEOgg}, {fileid.Audio, MIMEMP3}, {fileid.Sticker, MIMETGS},
		{fileid.Sticker, MIMEWebM}, {fileid.VideoNote, MIMEMP4},
		{fileid.Document, "text/html"}, {fileid.Document, "application/zip"},
	}
	for _, c := range ok {
		if err := CheckKind(c.k, c.m); err != nil {
			t.Errorf("%s as %s should be allowed: %v", c.m, c.k, err)
		}
	}
	bad := []struct {
		k fileid.Kind
		m string
	}{
		{fileid.Photo, MIMEGIF}, {fileid.Photo, "text/html"}, {fileid.Photo, "application/zip"},
		{fileid.Video, MIMEJPEG}, {fileid.Sticker, MIMEPNG}, {fileid.Voice, MIMEJPEG},
		{fileid.VideoNote, MIMEWebM}, {fileid.Kind("exe"), MIMEOctet},
	}
	for _, c := range bad {
		if err := CheckKind(c.k, c.m); err == nil {
			t.Errorf("%s as %s should be rejected", c.m, c.k)
		}
	}
}

func TestMaxSizeIsBoundedByUploadCeiling(t *testing.T) {
	kinds := []fileid.Kind{fileid.Photo, fileid.Video, fileid.Animation, fileid.Audio,
		fileid.Voice, fileid.VideoNote, fileid.Document, fileid.Sticker, fileid.Thumbnail}
	for _, k := range kinds {
		for _, m := range []string{MIMEJPEG, MIMEWebP, MIMETGS, MIMEWebM, MIMEMP4} {
			got := MaxSize(k, m)
			if got <= 0 || got > MaxUpload {
				t.Errorf("MaxSize(%s,%s) = %d outside (0,%d]", k, m, got, MaxUpload)
			}
		}
	}
	if MaxSize(fileid.Sticker, MIMETGS) >= MaxSize(fileid.Sticker, MIMEWebP) {
		t.Error("animated TGS stickers must have a tighter limit than static ones")
	}
}

func TestInlineSafetyForActiveContent(t *testing.T) {
	for _, m := range []string{"text/html", "image/svg+xml", "text/xml", "application/javascript",
		"application/pdf", "application/zip", MIMEOctet, MIMETGS} {
		if InlineSafe(m) {
			t.Errorf("%s must not be served inline", m)
		}
		if ServedType(m) != MIMEOctet {
			t.Errorf("%s must be served as an opaque download, got %q", m, ServedType(m))
		}
	}
	for _, m := range []string{MIMEJPEG, MIMEPNG, MIMEMP4, MIMEOgg} {
		if !InlineSafe(m) || ServedType(m) != m {
			t.Errorf("%s should be served as itself", m)
		}
	}
}

func TestSafeFileName(t *testing.T) {
	cases := map[string]string{
		"report.pdf":               "report.pdf",
		"../../etc/passwd":         "passwd",
		`C:\Users\a\b.txt`:         "b.txt",
		"a\r\nSet-Cookie: x=1.txt": "aSet-Cookie: x=1.txt",
		`quote".txt`:               "quote.txt",
		"semi;colon.txt":           "semicolon.txt",
		"..":                       "",
		"":                         "",
		"/":                        "",
	}
	for in, want := range cases {
		if got := SafeFileName(in); got != want {
			t.Errorf("SafeFileName(%q) = %q, want %q", in, got, want)
		}
	}
	long := bytes.Repeat([]byte("я"), 300)
	if got := SafeFileName(string(long)); len([]rune(got)) != 120 {
		t.Errorf("long names must be cut to 120 characters, got %d", len([]rune(got)))
	}
}
