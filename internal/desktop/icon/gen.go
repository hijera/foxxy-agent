//go:build ignore

// Command gen converts a source PNG into a multi-size Windows .ico file.
//
// Usage:
//
//	go run internal/desktop/icon/gen.go foxxycode2-Photoroom.png build/foxxycode.ico
//
// The generated .ico stores each size as a PNG-compressed entry (valid on
// Windows Vista+, supported by LoadImageW and by github.com/akavel/rsrc, which
// copies entry bytes verbatim when embedding the icon into a .syso resource).
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"

	xdraw "golang.org/x/image/draw"
)

// Standard Windows shell icon sizes, largest first.
var sizes = []int{256, 128, 64, 48, 40, 32, 24, 16}

// paddingFrac is the transparent margin left on each side after trimming, as a
// fraction of the final square. 0.05 => artwork fills ~90% of the icon.
const paddingFrac = 0.05

// alphaThreshold is the minimum alpha value treated as visible content when
// computing the artwork's bounding box.
const alphaThreshold = 16

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "icon gen:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	in := "foxxycode2-Photoroom.png"
	out := "build/foxxycode.ico"
	if len(args) > 0 && args[0] != "" {
		in = args[0]
	}
	if len(args) > 1 && args[1] != "" {
		out = args[1]
	}

	srcFile, err := os.Open(in)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	src, _, err := image.Decode(srcFile)
	if err != nil {
		return fmt.Errorf("decode png: %w", err)
	}

	// Source art is often exported on a large canvas with wide transparent
	// margins (e.g. the fox fills ~50% of a 1024px square). Trim the transparent
	// border and re-center the artwork with a small, uniform padding so the logo
	// fills the icon instead of appearing tiny.
	src = trimAndPad(src, paddingFrac)

	images := make([][]byte, 0, len(sizes))
	dims := make([]int, 0, len(sizes))
	for _, s := range sizes {
		dst := image.NewNRGBA(image.Rect(0, 0, s, s))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
		var buf bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, dst); err != nil {
			return fmt.Errorf("encode png %dx%d: %w", s, s, err)
		}
		images = append(images, buf.Bytes())
		dims = append(dims, s)
	}

	ico, err := buildICO(dims, images)
	if err != nil {
		return err
	}

	if dir := filepath.Dir(out); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(out, ico, 0o644); err != nil {
		return fmt.Errorf("write ico: %w", err)
	}
	fmt.Printf("wrote %s (%d entries, %d bytes)\n", out, len(images), len(ico))
	return nil
}

// trimAndPad crops the transparent border around the opaque artwork and returns
// a square image with the artwork centered and a uniform margin of `pad` (as a
// fraction of the square side) on each side. If no opaque pixels are found the
// source is returned unchanged.
func trimAndPad(src image.Image, pad float64) image.Image {
	b := src.Bounds()
	// Work in NRGBA so alpha is directly readable.
	nrgba, ok := src.(*image.NRGBA)
	if !ok {
		nrgba = image.NewNRGBA(b)
		xdraw.Copy(nrgba, b.Min, src, b, xdraw.Src, nil)
	}

	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if nrgba.NRGBAAt(x, y).A > alphaThreshold {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < minX || maxY < minY {
		return src // fully transparent; nothing to trim
	}

	cw, ch := maxX-minX+1, maxY-minY+1
	content := cw
	if ch > content {
		content = ch
	}
	// side chosen so the (square) content occupies (1 - 2*pad) of the square.
	side := int(float64(content) / (1 - 2*pad))
	if side < content {
		side = content
	}
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	offX := (side - cw) / 2
	offY := (side - ch) / 2
	srcRect := image.Rect(minX, minY, maxX+1, maxY+1)
	xdraw.Copy(dst, image.Pt(offX, offY), nrgba, srcRect, xdraw.Src, nil)
	return dst
}

// buildICO assembles an ICONDIR + ICONDIRENTRY headers followed by the PNG
// payloads. dims and payloads must be the same length.
func buildICO(dims []int, payloads [][]byte) ([]byte, error) {
	if len(dims) != len(payloads) {
		return nil, fmt.Errorf("dims/payloads length mismatch")
	}
	// Sort largest-first for a stable, conventional layout.
	idx := make([]int, len(dims))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return dims[idx[a]] > dims[idx[b]] })

	const dirEntrySize = 16
	const dirHeaderSize = 6
	offset := dirHeaderSize + dirEntrySize*len(payloads)

	var buf bytes.Buffer
	// ICONDIR header.
	binary.Write(&buf, binary.LittleEndian, uint16(0))             // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))             // type: 1 = icon
	binary.Write(&buf, binary.LittleEndian, uint16(len(payloads))) // count

	// ICONDIRENTRY per image.
	for _, i := range idx {
		s := dims[i]
		data := payloads[i]
		b := byte(s)
		if s >= 256 {
			b = 0 // 0 means 256 in the ICO format.
		}
		buf.WriteByte(b)                                           // width
		buf.WriteByte(b)                                           // height
		buf.WriteByte(0)                                           // color count (0 = 256+)
		buf.WriteByte(0)                                           // reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))         // color planes
		binary.Write(&buf, binary.LittleEndian, uint16(32))        // bits per pixel
		binary.Write(&buf, binary.LittleEndian, uint32(len(data))) // bytes in resource
		binary.Write(&buf, binary.LittleEndian, uint32(offset))    // image offset
		offset += len(data)
	}
	// Image payloads, in the same order as the entries.
	for _, i := range idx {
		buf.Write(payloads[i])
	}
	return buf.Bytes(), nil
}
