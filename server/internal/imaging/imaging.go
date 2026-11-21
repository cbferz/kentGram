// Package imaging decodes images safely and produces resized variants. It uses
// only the standard library; WebP support is added by the caller registering
// a decoder, so this package stays easy to test without third-party modules.
package imaging

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
)

// MaxPixels bounds the decoded size of an uploaded image. A small file can
// declare enormous dimensions, so the check is made on the header before any
// pixel memory is allocated.
const MaxPixels = 20_000_000

var ErrTooManyPixels = errors.New("imaging: image dimensions are too large")

// Config reads the header only and rejects images that would be too large to
// decode, returning the format name ("jpeg", "png", "gif", "webp").
func Config(r io.Reader) (image.Config, string, error) {
	cfg, format, err := image.DecodeConfig(r)
	if err != nil {
		return image.Config{}, "", err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return image.Config{}, "", errors.New("imaging: empty image")
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return image.Config{}, "", ErrTooManyPixels
	}
	return cfg, format, nil
}

// Decode fully decodes an image into straight RGBA pixels. Callers must have
// passed the same bytes through Config first.
func Decode(data []byte) (*image.RGBA, string, error) {
	if _, _, err := Config(bytes.NewReader(data)); err != nil {
		return nil, "", err
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	rgba := toRGBA(img)
	if format == "jpeg" {
		rgba = applyOrientation(rgba, ExifOrientation(data))
	}
	return rgba, format, nil
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

// Fit scales img down so that neither side exceeds maxDim, keeping the aspect
// ratio. Images that already fit are returned unchanged: variants never
// upscale.
func Fit(img *image.RGBA, maxDim int) *image.RGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if maxDim <= 0 || (w <= maxDim && h <= maxDim) {
		return img
	}
	nw, nh := maxDim, maxDim
	if w >= h {
		nh = max(1, h*maxDim/w)
	} else {
		nw = max(1, w*maxDim/h)
	}
	return boxResize(img, nw, nh)
}

// boxResize averages the source pixels covered by each destination pixel,
// which gives clean downscaling without ringing or aliasing. Premultiplied
// channels are averaged directly, which is the correct way to blend alpha.
func boxResize(src *image.RGBA, nw, nh int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := y*sh/nh, (y+1)*sh/nh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < nw; x++ {
			x0, x1 := x*sw/nw, (x+1)*sw/nw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, a, n uint32
			for sy := y0; sy < y1; sy++ {
				row := src.Pix[sy*src.Stride+x0*4 : sy*src.Stride+x1*4]
				for i := 0; i+3 < len(row); i += 4 {
					r += uint32(row[i])
					g += uint32(row[i+1])
					b += uint32(row[i+2])
					a += uint32(row[i+3])
					n++
				}
			}
			o := y*dst.Stride + x*4
			dst.Pix[o] = uint8(r / n)
			dst.Pix[o+1] = uint8(g / n)
			dst.Pix[o+2] = uint8(b / n)
			dst.Pix[o+3] = uint8(a / n)
		}
	}
	return dst
}

// HasAlpha reports whether any pixel is not fully opaque.
func HasAlpha(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff {
			return true
		}
	}
	return false
}

// Encode writes img as JPEG, or PNG when it has transparency. It returns the
// encoded bytes and their MIME type.
func Encode(img *image.RGBA) ([]byte, string, error) {
	var buf bytes.Buffer
	if HasAlpha(img) {
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/png", nil
	}
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}

// Variant is one resized rendition of a photo.
type Variant struct {
	Label         string
	MaxDim        int
	Width, Height int
	MIME          string
	Data          []byte
}

// Ladder lists the renditions generated for every photo, smallest first.
var Ladder = []struct {
	Label  string
	MaxDim int
}{{"s", 90}, {"m", 320}, {"x", 800}, {"y", 1280}}

// Variants renders every ladder step that is smaller than the source. Each
// step is derived from the previous larger one, so a 20 MP photo is only
// scanned in full once.
func Variants(src *image.RGBA) ([]Variant, error) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	longest := max(w, h)

	var steps []int
	for i := len(Ladder) - 1; i >= 0; i-- {
		if Ladder[i].MaxDim < longest {
			steps = append(steps, i)
		}
	}

	out := make([]Variant, 0, len(steps))
	cur := src
	for _, i := range steps {
		cur = Fit(cur, Ladder[i].MaxDim)
		data, mime, err := Encode(cur)
		if err != nil {
			return nil, fmt.Errorf("encode %s variant: %w", Ladder[i].Label, err)
		}
		out = append(out, Variant{
			Label: Ladder[i].Label, MaxDim: Ladder[i].MaxDim,
			Width: cur.Bounds().Dx(), Height: cur.Bounds().Dy(),
			MIME: mime, Data: data,
		})
	}
	// Smallest first reads naturally in API responses.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// FirstFrameGIF decodes only to learn whether a GIF is animated and to return
// its first frame for use as a preview.
func FirstFrameGIF(data []byte) (*image.RGBA, bool, error) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, false, err
	}
	if len(g.Image) == 0 {
		return nil, false, errors.New("imaging: gif has no frames")
	}
	return toRGBA(g.Image[0]), len(g.Image) > 1, nil
}
