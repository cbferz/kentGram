package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestConfigRejectsDecompressionBombs(t *testing.T) {
	data := encodePNG(t, solid(1, 1, color.RGBA{A: 255}))
	// Rewrite the IHDR dimensions to 10000x10000 (100 MP) and fix the CRC, so the
	// file is tiny but claims an enormous canvas.
	binary.BigEndian.PutUint32(data[16:20], 10000)
	binary.BigEndian.PutUint32(data[20:24], 10000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))

	if _, _, err := Config(bytes.NewReader(data)); !errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("expected ErrTooManyPixels, got %v", err)
	}
	if _, _, err := Decode(data); !errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("Decode must apply the same limit, got %v", err)
	}

	ok := encodePNG(t, solid(40, 30, color.RGBA{A: 255}))
	cfg, format, err := Config(bytes.NewReader(ok))
	if err != nil || format != "png" || cfg.Width != 40 || cfg.Height != 30 {
		t.Fatalf("a normal image must pass: %+v %q %v", cfg, format, err)
	}
	if _, _, err := Config(bytes.NewReader([]byte("not an image at all"))); err == nil {
		t.Fatal("garbage must be rejected")
	}
}

func TestFitKeepsAspectAndNeverUpscales(t *testing.T) {
	cases := []struct{ w, h, max, wantW, wantH int }{
		{400, 200, 100, 100, 50},
		{200, 400, 100, 50, 100},
		{300, 300, 100, 100, 100},
		{1000, 1, 100, 100, 1},
		{50, 40, 100, 50, 40},
		{100, 100, 100, 100, 100},
	}
	for _, c := range cases {
		src := solid(c.w, c.h, color.RGBA{10, 20, 30, 255})
		got := Fit(src, c.max)
		if got.Bounds().Dx() != c.wantW || got.Bounds().Dy() != c.wantH {
			t.Errorf("Fit(%dx%d,%d) = %dx%d, want %dx%d", c.w, c.h, c.max,
				got.Bounds().Dx(), got.Bounds().Dy(), c.wantW, c.wantH)
		}
	}
	small := solid(10, 10, color.RGBA{A: 255})
	if Fit(small, 100) != small {
		t.Error("an image that already fits must be returned as is")
	}
}

func TestResizeAveragesPixels(t *testing.T) {
	flat := Fit(solid(400, 400, color.RGBA{200, 100, 50, 255}), 40)
	if c := flat.RGBAAt(5, 5); c != (color.RGBA{200, 100, 50, 255}) {
		t.Fatalf("a flat colour must survive resizing unchanged, got %v", c)
	}

	// A black/white checkerboard averages to mid-grey.
	board := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			v := uint8(0)
			if (x+y)%2 == 0 {
				v = 255
			}
			board.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	c := Fit(board, 10).RGBAAt(3, 3)
	if c.R < 110 || c.R > 145 {
		t.Fatalf("checkerboard should average to mid-grey, got %v", c)
	}
}

func TestVariantsLadder(t *testing.T) {
	cases := []struct {
		w, h int
		want []string
		dims [][2]int
	}{
		{2000, 1000, []string{"s", "m", "x", "y"}, [][2]int{{90, 45}, {320, 160}, {800, 400}, {1280, 640}}},
		{500, 500, []string{"s", "m"}, [][2]int{{90, 90}, {320, 320}}},
		{100, 60, []string{"s"}, [][2]int{{90, 54}}},
		{90, 90, nil, nil},
		{50, 50, nil, nil},
	}
	for _, c := range cases {
		vs, err := Variants(solid(c.w, c.h, color.RGBA{1, 2, 3, 255}))
		if err != nil {
			t.Fatal(err)
		}
		if len(vs) != len(c.want) {
			t.Fatalf("%dx%d: got %d variants, want %d", c.w, c.h, len(vs), len(c.want))
		}
		for i, v := range vs {
			if v.Label != c.want[i] || v.Width != c.dims[i][0] || v.Height != c.dims[i][1] {
				t.Errorf("%dx%d variant %d = %s %dx%d, want %s %v", c.w, c.h, i,
					v.Label, v.Width, v.Height, c.want[i], c.dims[i])
			}
			if v.MIME != "image/jpeg" || len(v.Data) == 0 {
				t.Errorf("variant %s should be a non-empty JPEG, got %s/%d bytes", v.Label, v.MIME, len(v.Data))
			}
			cfg, _, err := image.DecodeConfig(bytes.NewReader(v.Data))
			if err != nil || cfg.Width != v.Width || cfg.Height != v.Height {
				t.Errorf("variant %s bytes do not match its declared size: %v %+v", v.Label, err, cfg)
			}
		}
	}
}

func TestEncodeChoosesFormatByAlpha(t *testing.T) {
	_, mime, err := Encode(solid(8, 8, color.RGBA{9, 9, 9, 255}))
	if err != nil || mime != "image/jpeg" {
		t.Fatalf("opaque images should be JPEG: %q %v", mime, err)
	}
	data, mime, err := Encode(solid(8, 8, color.RGBA{9, 9, 9, 128}))
	if err != nil || mime != "image/png" {
		t.Fatalf("transparent images should stay PNG: %q %v", mime, err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("PNG output must decode: %v", err)
	}
}

// orientationFixture is a 2 wide, 3 tall image whose pixels are all distinct.
func orientationFixture() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(10 * (y*2 + x + 1)), 0, 0, 255})
		}
	}
	return img
}

func reds(img *image.RGBA) [][]uint8 {
	b := img.Bounds()
	out := make([][]uint8, b.Dy())
	for y := range out {
		for x := 0; x < b.Dx(); x++ {
			out[y] = append(out[y], img.RGBAAt(x, y).R/10)
		}
	}
	return out
}

func TestApplyOrientation(t *testing.T) {
	// Source rows:  1 2 / 3 4 / 5 6
	want := map[int][][]uint8{
		2: {{2, 1}, {4, 3}, {6, 5}},
		3: {{6, 5}, {4, 3}, {2, 1}},
		4: {{5, 6}, {3, 4}, {1, 2}},
		5: {{1, 3, 5}, {2, 4, 6}},
		6: {{5, 3, 1}, {6, 4, 2}},
		7: {{6, 4, 2}, {5, 3, 1}},
		8: {{2, 4, 6}, {1, 3, 5}},
	}
	for o, rows := range want {
		got := reds(applyOrientation(orientationFixture(), o))
		if len(got) != len(rows) {
			t.Fatalf("orientation %d: got %d rows, want %d", o, len(got), len(rows))
		}
		for y := range rows {
			for x := range rows[y] {
				if got[y][x] != rows[y][x] {
					t.Fatalf("orientation %d: got %v, want %v", o, got, rows)
				}
			}
		}
	}
	src := orientationFixture()
	for _, o := range []int{0, 1, 9, -1} {
		if applyOrientation(src, o) != src {
			t.Errorf("orientation %d must leave the image untouched", o)
		}
	}
}

// jpegWithOrientation encodes a w x h JPEG and splices an EXIF APP1 segment
// carrying the given orientation right after the SOI marker.
func jpegWithOrientation(t *testing.T, w, h, orientation int, order binary.AppendByteOrder, tag string) []byte {
	t.Helper()
	var plain bytes.Buffer
	if err := jpeg.Encode(&plain, solid(w, h, color.RGBA{90, 90, 90, 255}), nil); err != nil {
		t.Fatal(err)
	}
	tiff := []byte(tag)
	tiff = order.AppendUint16(tiff, 42)
	tiff = order.AppendUint32(tiff, 8)
	tiff = order.AppendUint16(tiff, 1) // one IFD entry
	tiff = order.AppendUint16(tiff, 0x0112)
	tiff = order.AppendUint16(tiff, 3) // SHORT
	tiff = order.AppendUint32(tiff, 1)
	tiff = order.AppendUint16(tiff, uint16(orientation))
	tiff = append(tiff, 0, 0)
	tiff = order.AppendUint32(tiff, 0)

	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2))
	seg = append(seg, payload...)

	b := plain.Bytes()
	return append(append(append([]byte{}, b[:2]...), seg...), b[2:]...)
}

func TestExifOrientationParsing(t *testing.T) {
	for o := 1; o <= 8; o++ {
		le := jpegWithOrientation(t, 8, 8, o, binary.LittleEndian, "II")
		be := jpegWithOrientation(t, 8, 8, o, binary.BigEndian, "MM")
		if got := ExifOrientation(le); got != o {
			t.Errorf("little-endian orientation %d parsed as %d", o, got)
		}
		if got := ExifOrientation(be); got != o {
			t.Errorf("big-endian orientation %d parsed as %d", o, got)
		}
	}
	var plain bytes.Buffer
	_ = jpeg.Encode(&plain, solid(8, 8, color.RGBA{A: 255}), nil)
	for name, data := range map[string][]byte{
		"no exif": plain.Bytes(), "empty": nil, "not a jpeg": []byte("GIF89a"),
		"truncated": jpegWithOrientation(t, 8, 8, 6, binary.LittleEndian, "II")[:20],
		"bad value": jpegWithOrientation(t, 8, 8, 42, binary.LittleEndian, "II"),
	} {
		if got := ExifOrientation(data); got != 1 {
			t.Errorf("%s: expected the neutral orientation 1, got %d", name, got)
		}
	}
}

func TestDecodeHonoursExifRotation(t *testing.T) {
	data := jpegWithOrientation(t, 40, 20, 6, binary.LittleEndian, "II")
	img, format, err := Decode(data)
	if err != nil || format != "jpeg" {
		t.Fatalf("decode failed: %q %v", format, err)
	}
	if img.Bounds().Dx() != 20 || img.Bounds().Dy() != 40 {
		t.Fatalf("a 40x20 JPEG tagged rotate-90 must decode as 20x40, got %dx%d",
			img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestFirstFrameGIF(t *testing.T) {
	frame := func(c uint8) *image.Paletted {
		p := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White})
		for i := range p.Pix {
			p.Pix[i] = c
		}
		return p
	}
	encode := func(frames ...*image.Paletted) []byte {
		g := &gif.GIF{}
		for _, f := range frames {
			g.Image = append(g.Image, f)
			g.Delay = append(g.Delay, 5)
		}
		var buf bytes.Buffer
		if err := gif.EncodeAll(&buf, g); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	img, animated, err := FirstFrameGIF(encode(frame(0), frame(1)))
	if err != nil || !animated || img.Bounds().Dx() != 4 {
		t.Fatalf("two frames must be reported as animated: %v %v", animated, err)
	}
	if c := img.RGBAAt(0, 0); c.R != 0 {
		t.Fatalf("the preview must be the first frame (black), got %v", c)
	}
	if _, animated, _ = FirstFrameGIF(encode(frame(1))); animated {
		t.Fatal("a single frame is not animated")
	}
	if _, _, err := FirstFrameGIF([]byte("nope")); err == nil {
		t.Fatal("garbage must fail")
	}
}
