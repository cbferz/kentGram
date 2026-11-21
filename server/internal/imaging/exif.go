package imaging

import (
	"encoding/binary"
	"image"
)

// ExifOrientation returns the EXIF orientation (1-8) of a JPEG, or 1 when it
// is absent or unreadable. Phones usually store pixels in sensor order and
// record the intended rotation here, so thumbnails must honour it to match
// what clients show for the original.
func ExifOrientation(jpegData []byte) int {
	d := jpegData
	if len(d) < 4 || d[0] != 0xFF || d[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(d) {
		if d[i] != 0xFF {
			return 1
		}
		marker := d[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan / end of image
			return 1
		}
		size := int(binary.BigEndian.Uint16(d[i+2 : i+4]))
		if size < 2 || i+2+size > len(d) {
			return 1
		}
		if marker == 0xE1 { // APP1
			if o := orientationFromAPP1(d[i+4 : i+2+size]); o != 0 {
				return o
			}
		}
		i += 2 + size
	}
	return 1
}

func orientationFromAPP1(seg []byte) int {
	if len(seg) < 14 || string(seg[:6]) != "Exif\x00\x00" {
		return 0
	}
	tiff := seg[6:]
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 0
	}
	ifd := int(order.Uint32(tiff[4:8]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 0
	}
	count := int(order.Uint16(tiff[ifd : ifd+2]))
	for n := 0; n < count; n++ {
		off := ifd + 2 + n*12
		if off+12 > len(tiff) {
			return 0
		}
		if order.Uint16(tiff[off:off+2]) == 0x0112 { // Orientation
			v := int(order.Uint16(tiff[off+8 : off+10]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 0
		}
	}
	return 0
}

// applyOrientation rotates or mirrors pixels so the image is shown upright.
func applyOrientation(src *image.RGBA, o int) *image.RGBA {
	if o <= 1 || o > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if o >= 5 { // 90-degree variants swap the axes
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nx, ny int
			switch o {
			case 2: // mirror horizontal
				nx, ny = w-1-x, y
			case 3: // rotate 180
				nx, ny = w-1-x, h-1-y
			case 4: // mirror vertical
				nx, ny = x, h-1-y
			case 5: // transpose
				nx, ny = y, x
			case 6: // rotate 90 clockwise
				nx, ny = h-1-y, x
			case 7: // transverse
				nx, ny = h-1-y, w-1-x
			case 8: // rotate 90 counter-clockwise
				nx, ny = y, w-1-x
			}
			si := y*src.Stride + x*4
			di := ny*dst.Stride + nx*4
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}
