package shapes

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // (registered for image.Decode)

	xdraw "golang.org/x/image/draw"
)

// A sketch's picture is loaded once, from a PNG or JPEG: turned upright (a
// phone's photo says in its EXIF data which way up it was held), shrunk
// to no more than MaxSketchImagePx a side, laid on white (a tracing aid
// has no use for transparency) and kept as a JPEG.

// NewSketchImage makes a sketch's picture from a PNG or JPEG file's bytes,
// with no place yet (X, Y, Width, Angle and Opacity are the caller's).
func NewSketchImage(file []byte) (*SketchImage, error) {
	img, format, err := image.Decode(bytes.NewReader(file))
	if err != nil {
		return nil, fmt.Errorf("not a picture ScottiCAD can read (PNG or JPEG): %v", err)
	}
	if format == "jpeg" {
		img = uprightImage(img, exifOrientation(file))
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("the picture is empty")
	}
	if m := max(w, h); m > MaxSketchImagePx {
		w = max(1, w*MaxSketchImagePx/m)
		h = max(1, h*MaxSketchImagePx/m)
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, b, xdraw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return &SketchImage{Data: base64.StdEncoding.EncodeToString(buf.Bytes()), PW: w, PH: h, Opacity: 0.5}, nil
}

// Pixels decodes the picture, for drawing.
func (im *SketchImage) Pixels() (*image.RGBA, error) {
	raw, err := base64.StdEncoding.DecodeString(im.Data)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	xdraw.Draw(out, out.Bounds(), img, img.Bounds().Min, xdraw.Src)
	return out, nil
}

// exifOrientation is a JPEG's EXIF orientation (1 to 8; 1, upright, when
// it has none or it can't be read).
func exifOrientation(file []byte) int {
	p := 2
	if len(file) < 4 || file[0] != 0xFF || file[1] != 0xD8 {
		return 1
	}
	for p+4 <= len(file) {
		if file[p] != 0xFF {
			return 1
		}
		marker := file[p+1]
		if marker == 0xDA || marker == 0xD9 { // the image data, or the end: no EXIF before it
			return 1
		}
		n := int(binary.BigEndian.Uint16(file[p+2:]))
		if n < 2 || p+2+n > len(file) {
			return 1
		}
		seg := file[p+4 : p+2+n]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		p += 2 + n
	}
	return 1
}

// tiffOrientation reads the orientation tag (0x0112) of a TIFF header's
// first directory.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for i := 0; i < n; i++ {
		e := off + 2 + 12*i
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// uprightImage turns a picture taken with EXIF orientation o upright.
func uprightImage(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := w, h
	if o >= 5 { // a quarter turn: the sides swap
		ow, oh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2: // mirrored
				dx, dy = w-1-x, y
			case 3: // upside down
				dx, dy = w-1-x, h-1-y
			case 4: // mirrored upside down
				dx, dy = x, h-1-y
			case 5: // mirrored, a quarter turn
				dx, dy = y, x
			case 6: // a quarter turn clockwise to stand it up
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8: // a quarter turn anticlockwise
				dx, dy = y, w-1-x
			}
			out.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}
