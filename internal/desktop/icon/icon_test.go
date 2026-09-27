package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// square returns a size x size image with an opaque block of side inner pixels
// in the top-left corner and transparent elsewhere.
func square(size, inner int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < inner; y++ {
		for x := 0; x < inner; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0xFC, G: 0x7E, B: 0x05, A: 0xFF})
		}
	}
	return img
}

func TestBuildICOEntriesAreLargestFirstAndPointAtTheirPayload(t *testing.T) {
	data, err := BuildICO(square(64, 64), []int{16, 32})
	if err != nil {
		t.Fatalf("BuildICO: %v", err)
	}

	if got := binary.LittleEndian.Uint16(data[0:2]); got != 0 {
		t.Errorf("reserved = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(data[2:4]); got != 1 {
		t.Errorf("type = %d, want 1 (icon)", got)
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	wantSides := []byte{32, 16} // largest first
	for i := 0; i < count; i++ {
		entry := data[6+16*i : 6+16*(i+1)]
		if entry[0] != wantSides[i] || entry[1] != wantSides[i] {
			t.Errorf("entry %d is %dx%d, want %d square", i, entry[0], entry[1], wantSides[i])
		}
		if bpp := binary.LittleEndian.Uint16(entry[6:8]); bpp != 32 {
			t.Errorf("entry %d bits per pixel = %d, want 32", i, bpp)
		}
		size := binary.LittleEndian.Uint32(entry[8:12])
		offset := binary.LittleEndian.Uint32(entry[12:16])
		if int(offset)+int(size) > len(data) {
			t.Fatalf("entry %d runs past the file: offset %d + size %d > %d", i, offset, size, len(data))
		}
		payload := data[offset : offset+size]
		cfg, err := png.DecodeConfig(bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("entry %d payload is not a PNG: %v", i, err)
		}
		if cfg.Width != int(wantSides[i]) || cfg.Height != int(wantSides[i]) {
			t.Errorf("entry %d payload is %dx%d, want %d square", i, cfg.Width, cfg.Height, wantSides[i])
		}
	}
}

func TestBuildICOEncodes256AsZero(t *testing.T) {
	data, err := BuildICO(square(256, 256), []int{256})
	if err != nil {
		t.Fatalf("BuildICO: %v", err)
	}
	entry := data[6:22]
	if entry[0] != 0 || entry[1] != 0 {
		t.Errorf("256px entry recorded as %dx%d, want 0x0 (the ICO encoding of 256)", entry[0], entry[1])
	}
}

func TestBuildICORejectsNoSizes(t *testing.T) {
	if _, err := BuildICO(square(8, 8), nil); err == nil {
		t.Fatal("BuildICO with no sizes: want an error, got nil")
	}
}

func TestTrimAndPadCentresTheArtworkAndLeavesTheAskedMargin(t *testing.T) {
	// A 40px block in the corner of a 200px canvas: after trimming, the block
	// must fill 1-2*pad of the result and sit in the middle.
	content, pad := 40, 0.05
	got := TrimAndPad(square(200, content), pad)
	side := got.Bounds().Dx()
	if side != got.Bounds().Dy() {
		t.Fatalf("result is %dx%d, want a square", side, got.Bounds().Dy())
	}
	if want := int(float64(content) / (1 - 2*pad)); side != want {
		t.Fatalf("side = %d, want %d (%dpx of content at %.0f%% padding)", side, want, content, pad*100)
	}

	nrgba, ok := got.(*image.NRGBA)
	if !ok {
		t.Fatalf("result is %T, want *image.NRGBA", got)
	}
	if a := nrgba.NRGBAAt(side/2, side/2).A; a == 0 {
		t.Error("the centre of the result is transparent; the artwork was not centred")
	}
	if a := nrgba.NRGBAAt(0, 0).A; a != 0 {
		t.Errorf("the corner has alpha %d, want 0; the padding was not applied", a)
	}
}

func TestTrimAndPadLeavesAFullyTransparentSourceAlone(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	if got := TrimAndPad(src, 0.05); got != image.Image(src) {
		t.Error("a fully transparent source was rewritten; want it returned unchanged")
	}
}
