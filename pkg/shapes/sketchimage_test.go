package shapes

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"testing"
)

// A picture is shrunk to MaxSketchImagePx on its longer side, keeping its
// shape, and kept as a JPEG.
func TestSketchImageLoadsShrunk(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3000, 1000))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	im, err := NewSketchImage(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if im.PW != MaxSketchImagePx || im.PH != MaxSketchImagePx/3 {
		t.Errorf("%d × %d", im.PW, im.PH)
	}
	px, err := im.Pixels()
	if err != nil || px.Bounds().Dx() != im.PW || px.Bounds().Dy() != im.PH {
		t.Errorf("pixels %v, err %v", px.Bounds(), err)
	}
	if _, err := NewSketchImage([]byte("not a picture")); err == nil {
		t.Error("nonsense loaded")
	}
}

// withOrientation is a JPEG with an EXIF orientation tag.
func withOrientation(jpg []byte, o uint16) []byte {
	var tiff bytes.Buffer
	tiff.WriteString("II*\x00")
	binary.Write(&tiff, binary.LittleEndian, uint32(8))
	binary.Write(&tiff, binary.LittleEndian, uint16(1)) // one entry
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0112))
	binary.Write(&tiff, binary.LittleEndian, uint16(3)) // SHORT
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, o)
	binary.Write(&tiff, binary.LittleEndian, uint16(0))
	binary.Write(&tiff, binary.LittleEndian, uint32(0)) // no next directory
	seg := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	var out bytes.Buffer
	out.Write(jpg[:2]) // SOI
	out.Write([]byte{0xFF, 0xE1})
	binary.Write(&out, binary.BigEndian, uint16(len(seg)+2))
	out.Write(seg)
	out.Write(jpg[2:])
	return out.Bytes()
}

// A phone's photo, held on its side, says so in its EXIF data: it is
// stood upright as it loads. Its top left (red) lands where it is seen.
func TestSketchImageUprightFromEXIF(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x < 20 && y < 10 {
				c = color.RGBA{255, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	red := func(c color.Color) bool { r, g, _, _ := c.RGBA(); return r > 0xC000 && g < 0x6000 }
	for _, c := range []struct {
		o          uint16
		w, h, x, y int // the size, and where the red corner is
	}{
		{1, 80, 40, 3, 3},
		{3, 80, 40, 76, 36}, // upside down
		{6, 40, 80, 36, 3},  // turned a quarter clockwise to stand up
		{8, 40, 80, 3, 76},  // a quarter anticlockwise
	} {
		im, err := NewSketchImage(withOrientation(buf.Bytes(), c.o))
		if err != nil {
			t.Fatal(err)
		}
		if im.PW != c.w || im.PH != c.h {
			t.Errorf("orientation %d: %d × %d, want %d × %d", c.o, im.PW, im.PH, c.w, c.h)
			continue
		}
		px, _ := im.Pixels()
		if !red(px.At(c.x, c.y)) {
			t.Errorf("orientation %d: the red corner isn't at (%d, %d)", c.o, c.x, c.y)
		}
	}
}

// The picture is never part of the shape: the key that says when a sketch
// must be rebuilt leaves it out, and a copy has its own placement.
func TestSketchImageIsNotGeometry(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{{Closed: true, Points: skPts(0, 0, 10, 0, 10, 10)}}}
	with := CopySketch(sk)
	with.Image = &SketchImage{Data: "abc", PW: 10, PH: 5, Width: 20, Opacity: 0.5}
	if SketchKey(sk) != SketchKey(with) {
		t.Error("a picture changed the sketch's key")
	}
	if SameImage(sk, with) || !SameImage(with, CopySketch(with)) {
		t.Error("SameImage")
	}
	c := CopySketch(with)
	c.Image.X = 5
	if with.Image.X != 0 {
		t.Error("a copy's picture moved the original's")
	}
	if h := with.Image.Height(); math.Abs(h-10) > 1e-12 {
		t.Errorf("height %v", h)
	}
	cs := (&SketchImage{X: 1, Y: 2, Width: 4, PW: 2, PH: 1, Angle: 90}).Corners()
	if math.Abs(cs[0][0]-0) > 1e-9 || math.Abs(cs[0][1]-0) > 1e-9 { // top left, turned a quarter: (1-1, 2-2)
		t.Errorf("corners %v", cs)
	}
	for _, bad := range []SketchImage{
		{Data: "", PW: 1, PH: 1, Width: 1},
		{Data: "x", PW: 0, PH: 1, Width: 1},
		{Data: "x", PW: 1, PH: 1, Width: 0},
		{Data: "x", PW: 1, PH: 1, Width: 1, Opacity: 2},
		{Data: "x", PW: 1, PH: 1, Width: 1, X: math.NaN()},
	} {
		bad := bad
		if CheckSketch(&Sketch{Image: &bad}) == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
