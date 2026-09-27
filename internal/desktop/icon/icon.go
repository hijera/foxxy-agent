// Package icon assembles Windows .ico files from rendered artwork.
//
// It exists as a package rather than as part of the generator command because
// two callers need it: the Windows executable resource (RT_GROUP_ICON id 1,
// embedded through cmd/foxxycode/rsrc_windows_amd64.syso and wired into the
// WebView2 window by internal/desktop) and the favicon.ico the web UI and the
// website serve. Both are produced by scripts/brand/gen.go.
package icon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"sort"

	xdraw "golang.org/x/image/draw"
)

// WindowsSizes are the standard Windows shell icon sizes, largest first.
var WindowsSizes = []int{256, 128, 64, 48, 40, 32, 24, 16}

// FaviconSizes are the sizes a browser picks from favicon.ico.
var FaviconSizes = []int{32, 16}

// AlphaThreshold is the minimum alpha treated as visible content when the
// artwork's bounding box is computed.
const AlphaThreshold = 8

// Resize scales src into a size x size NRGBA image.
func Resize(src image.Image, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

// TrimAndPad crops the transparent border around the opaque artwork and returns
// a square image with the artwork centred and a uniform margin of pad (as a
// fraction of the square side) on each side. Source art is usually exported on
// a large canvas with wide transparent margins, and without this the logo would
// appear tiny inside the icon. A fully transparent source is returned unchanged.
func TrimAndPad(src image.Image, pad float64) image.Image {
	b := src.Bounds()
	nrgba, ok := src.(*image.NRGBA)
	if !ok {
		nrgba = image.NewNRGBA(b)
		xdraw.Copy(nrgba, b.Min, src, b, xdraw.Src, nil)
	}

	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if nrgba.NRGBAAt(x, y).A > AlphaThreshold {
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
		return src
	}

	cw, ch := maxX-minX+1, maxY-minY+1
	content := cw
	if ch > content {
		content = ch
	}
	side := int(float64(content) / (1 - 2*pad))
	if side < content {
		side = content
	}
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	offX := (side - cw) / 2
	offY := (side - ch) / 2
	xdraw.Copy(dst, image.Pt(offX, offY), nrgba, image.Rect(minX, minY, maxX+1, maxY+1), xdraw.Src, nil)
	return dst
}

// BuildICO renders src at every requested size and assembles an ICONDIR with
// one PNG-compressed entry per size. PNG entries are valid on Windows Vista and
// later, are what LoadImageW reads, and are copied verbatim by
// github.com/akavel/rsrc when the icon is embedded into a .syso resource.
func BuildICO(src image.Image, sizes []int) ([]byte, error) {
	if len(sizes) == 0 {
		return nil, fmt.Errorf("icon: no sizes requested")
	}
	payloads := make([][]byte, 0, len(sizes))
	dims := make([]int, 0, len(sizes))
	for _, size := range sizes {
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, Resize(src, size)); err != nil {
			return nil, fmt.Errorf("icon: encode %dx%d: %w", size, size, err)
		}
		payloads = append(payloads, buf.Bytes())
		dims = append(dims, size)
	}
	return packICO(dims, payloads)
}

// packICO writes the ICONDIR header, one ICONDIRENTRY per image and then the
// payloads, largest entry first.
func packICO(dims []int, payloads [][]byte) ([]byte, error) {
	if len(dims) != len(payloads) {
		return nil, fmt.Errorf("icon: %d sizes but %d payloads", len(dims), len(payloads))
	}
	order := make([]int, len(dims))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return dims[order[a]] > dims[order[b]] })

	const headerSize, entrySize = 6, 16
	offset := headerSize + entrySize*len(payloads)

	var buf bytes.Buffer
	write := func(v any) {
		_ = binary.Write(&buf, binary.LittleEndian, v)
	}
	write(uint16(0))             // reserved
	write(uint16(1))             // type: 1 = icon
	write(uint16(len(payloads))) // count

	for _, i := range order {
		side := byte(dims[i])
		if dims[i] >= 256 {
			side = 0 // 0 means 256 in the ICO format.
		}
		buf.WriteByte(side)
		buf.WriteByte(side)
		buf.WriteByte(0) // colour count (0 = 256 or more)
		buf.WriteByte(0) // reserved
		write(uint16(1))
		write(uint16(32))
		write(uint32(len(payloads[i])))
		write(uint32(offset))
		offset += len(payloads[i])
	}
	for _, i := range order {
		buf.Write(payloads[i])
	}
	return buf.Bytes(), nil
}
