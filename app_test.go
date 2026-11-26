package main

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

// opaque returns the bounds of the pixels that aren't fully transparent.
func opaque(t *testing.T, data []byte) (image.Rectangle, image.Rectangle) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var r image.Rectangle
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return b, r
}

func TestMenuBarIconPadsTo22Points(t *testing.T) {
	inBounds, inGlyph := opaque(t, trayTemplateIcon)
	outBounds, outGlyph := opaque(t, menuBarIcon(trayTemplateIcon))
	if inBounds.Dx() != 32 || outBounds.Dx() != 44 || outBounds.Dy() != 44 {
		t.Fatalf("bounds %v -> %v, want 32 px -> 44 px (16 pt -> 22 pt at 2x)", inBounds, outBounds)
	}
	if want := inGlyph.Add(image.Pt(6, 6)); outGlyph != want {
		t.Errorf("glyph at %v, want %v: same size, centred", outGlyph, want)
	}
}

func TestMenuBarIconKeepsBadData(t *testing.T) {
	bad := []byte("not a png")
	if got := menuBarIcon(bad); !bytes.Equal(got, bad) {
		t.Errorf("got %q, want the input back", got)
	}
}
