//go:build ignore

// Command gen rebuilds every brand asset from the one vector source.
//
//	go run scripts/brand/gen.go            # from the repository root
//
// The source is docs/assets/foxxycode-logo-glyph.svg, the fox traced from
// docs/assets/brand/foxxycode-mark-1024.png by scripts/brand/trace.py. This
// command does three things with it:
//
//  1. Injects the glyph into every SVG that marks a slot for it, between
//     <!-- foxxycode:glyph:start ... --> and <!-- foxxycode:glyph:end -->, so
//     the marks, the wordmarks, the social preview and the editor icons never
//     hold their own copy of the drawing.
//  2. Rasterises the compositions with headless Chrome (chromedp) and assembles
//     the two .ico files.
//  3. Writes docs/assets/brand.lock.json, which internal/brand's test reads to
//     catch a source edited without a re-export - the exact way the social
//     preview kept the upstream product's name after the rebrand.
//
// Chrome is required, so this is an authoring step and not part of the build;
// its output is committed. The Windows executable resource is a second step,
// `make icon`, because it needs github.com/akavel/rsrc as well.
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/brand"
	"github.com/hijera/foxxycode-agent/internal/desktop/icon"
)

// glyphSource is the single vector every other brand file is built from.
const glyphSource = "docs/assets/foxxycode-logo-glyph.svg"

// markPadding is the transparent margin left around the artwork in the Windows
// icon, as a fraction of the square side.
const markPadding = 0.05

// injectDirs are searched for SVGs carrying a glyph slot.
var injectDirs = []string{"docs/assets", "editors"}

// skipDirs are build output and dependency trees: Gradle copies the plugin's
// resources into editors/intellij/build, and rewriting those copies would put
// build artefacts in the lock file.
var skipDirs = map[string]bool{"build": true, "dist": true, "node_modules": true, "out": true}

// raster is one PNG rendered from one composition.
type raster struct {
	src           string
	out           string
	width, height int
}

var rasters = []raster{
	{src: "docs/assets/foxxycode-logo-mark-icon.svg", out: "docs/assets/favicon-32.png", width: 32, height: 32},
	{src: "docs/assets/foxxycode-logo-mark-icon.svg", out: "docs/assets/apple-touch-icon.png", width: 180, height: 180},
	{src: "docs/assets/foxxycode-logo-mark-icon.svg", out: "editors/vscode/media/icon.png", width: 128, height: 128},
	{src: "docs/assets/foxxycode-logo-social.svg", out: "docs/assets/foxxycode-logo-social-1280x640.png", width: 1280, height: 640},
	{src: "docs/assets/foxxycode-logo-social.svg", out: "docs/assets/foxxycode-logo-social-640x320.png", width: 640, height: 320},
}

// icoTarget is one .ico assembled from a composition rendered once at a
// generous size and downscaled to each entry.
type icoTarget struct {
	src    string
	out    string
	render int
	sizes  []int
	// trim squares the artwork and leaves a uniform margin, which the Windows
	// shell needs and a favicon (already drawn on a full-bleed plate) does not.
	trim bool
}

var icos = []icoTarget{
	{src: "docs/assets/foxxycode-logo-mark-icon.svg", out: "docs/assets/favicon.ico", render: 256, sizes: icon.FaviconSizes},
	{src: glyphSource, out: "build/foxxycode.ico", render: 1024, sizes: icon.WindowsSizes, trim: true},
}

// copies keep the Vite public directory in step with docs/assets. The copies at
// the external/ui root are written by `make ui-build`, not here.
var copies = []struct{ from, to string }{
	{"docs/assets/foxxycode-favicon.svg", "external/ui/public/foxxycode-favicon.svg"},
	{"docs/assets/favicon-32.png", "external/ui/public/favicon-32.png"},
	{"docs/assets/favicon.ico", "external/ui/public/favicon.ico"},
	{"docs/assets/apple-touch-icon.png", "external/ui/public/apple-touch-icon.png"},
}

// build/foxxycode.ico is a build artefact, not a committed file, so it stays
// out of the lock.
var unlocked = map[string]bool{"build/foxxycode.ico": true}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "brand:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(glyphSource))); err != nil {
		return fmt.Errorf("run me from the repository root: %w", err)
	}

	glyph, err := loadGlyph(root)
	if err != nil {
		return err
	}

	injected, err := injectAll(root, glyph)
	if err != nil {
		return err
	}
	for _, path := range injected {
		fmt.Println("glyph  ->", path)
	}

	ctx, cancel := newBrowser()
	defer cancel()

	lock := &brand.Lock{Sources: map[string]string{}, Outputs: map[string]brand.Output{}}
	record := func(out string, from ...string) error {
		if unlocked[out] {
			return nil
		}
		sum, err := brand.Hash(root, out)
		if err != nil {
			return err
		}
		for _, src := range from {
			if _, ok := lock.Sources[src]; !ok {
				srcSum, err := brand.Hash(root, src)
				if err != nil {
					return err
				}
				lock.Sources[src] = srcSum
			}
		}
		lock.Outputs[out] = brand.Output{SHA256: sum, From: from}
		return nil
	}

	for _, r := range rasters {
		img, err := render(ctx, root, r.src, r.width, r.height)
		if err != nil {
			return fmt.Errorf("render %s: %w", r.src, err)
		}
		if err := writePNG(root, r.out, img); err != nil {
			return err
		}
		fmt.Printf("raster -> %s (%dx%d)\n", r.out, r.width, r.height)
		if err := record(r.out, r.src); err != nil {
			return err
		}
	}

	for _, t := range icos {
		img, err := render(ctx, root, t.src, t.render, 0)
		if err != nil {
			return fmt.Errorf("render %s: %w", t.src, err)
		}
		if t.trim {
			img = icon.TrimAndPad(img, markPadding)
		}
		data, err := icon.BuildICO(img, t.sizes)
		if err != nil {
			return err
		}
		if err := writeFile(root, t.out, data); err != nil {
			return err
		}
		fmt.Printf("ico    -> %s (%v)\n", t.out, t.sizes)
		if err := record(t.out, t.src); err != nil {
			return err
		}
	}

	for _, c := range copies {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.from)))
		if err != nil {
			return err
		}
		if err := writeFile(root, c.to, data); err != nil {
			return err
		}
		fmt.Println("copy   ->", c.to)
		if err := record(c.to, c.from); err != nil {
			return err
		}
	}

	// The glyph is a source of everything, including the files the injection
	// step rewrote, so record it and them too.
	if err := record(glyphSource, glyphSource); err != nil {
		return err
	}
	for _, path := range injected {
		if path == glyphSource {
			continue
		}
		if err := record(path, glyphSource); err != nil {
			return err
		}
	}

	if err := lock.Save(root); err != nil {
		return err
	}
	fmt.Println("lock   ->", brand.LockPath)
	return nil
}

// ---- the glyph ----

type glyph struct {
	// minX, minY, width, height are the glyph's viewBox: tight around the
	// drawing, so a slot only has to say how wide it wants the fox to be.
	minX, minY, width, height float64
	layers                    map[string]string // id -> full <path .../> element
	order                     []string
}

var (
	viewBoxRe = regexp.MustCompile(`viewBox="\s*([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s*"`)
	pathRe    = regexp.MustCompile(`<path id="([a-z]+)"[^>]*\sd="([^"]+)"\s*/>`)
)

func loadGlyph(root string) (*glyph, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(glyphSource)))
	if err != nil {
		return nil, err
	}
	text := string(data)

	box := viewBoxRe.FindStringSubmatch(text)
	if box == nil {
		return nil, fmt.Errorf("%s has no viewBox", glyphSource)
	}
	num := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	g := &glyph{
		minX:   num(box[1]),
		minY:   num(box[2]),
		width:  num(box[3]),
		height: num(box[4]),
		layers: map[string]string{},
	}

	for _, m := range pathRe.FindAllStringSubmatch(text, -1) {
		g.layers[m[1]] = m[2]
		g.order = append(g.order, m[1])
	}
	for _, want := range []string{"outline", "fur", "markings"} {
		if _, ok := g.layers[want]; !ok {
			return nil, fmt.Errorf("%s has no %q layer; re-run scripts/brand/trace.py", glyphSource, want)
		}
	}
	return g, nil
}

// ---- injection ----

var (
	slotRe   = regexp.MustCompile(`(?s)(<!--\s*foxxycode:glyph:start([^>]*?)-->).*?(<!--\s*foxxycode:glyph:end\s*-->)`)
	optionRe = regexp.MustCompile(`(\w+)=("[^"]*"|\S+)`)
)

// injectAll rewrites every glyph slot found under injectDirs and returns the
// repository-relative paths it touched, in a stable order.
func injectAll(root string, g *glyph) ([]string, error) {
	var touched []string
	for _, dir := range injectDirs {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".svg") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !strings.Contains(string(data), "foxxycode:glyph:start") {
				return nil
			}
			out, err := inject(string(data), g)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if out != string(data) {
				if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
					return err
				}
			}
			touched = append(touched, rel)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return touched, nil
}

// inject replaces the body of every slot in one SVG.
//
// A slot declares how wide the fox should be in the host's own units, and
// optionally a single-colour mode:
//
//	<!-- foxxycode:glyph:start width="32" -->…<!-- foxxycode:glyph:end -->
//	<!-- foxxycode:glyph:start width="13" mode="mono" fill="currentColor" -->…
//
// In mono mode the silhouette and the white markings become one evenodd path,
// so the muzzle and the braces read as holes rather than as a second colour -
// which is what an editor's activity bar and tool window require.
func inject(doc string, g *glyph) (string, error) {
	var failure error
	out := slotRe.ReplaceAllStringFunc(doc, func(slot string) string {
		m := slotRe.FindStringSubmatch(slot)
		opts := parseOptions(m[2])

		width, err := strconv.ParseFloat(opts["width"], 64)
		if err != nil || width <= 0 {
			failure = fmt.Errorf(`glyph slot needs a positive width="…", got %q`, opts["width"])
			return slot
		}
		scale := width / g.width
		cx := g.minX + g.width/2
		cy := g.minY + g.height/2

		indent := slotIndent(doc, slot)
		var body strings.Builder
		body.WriteString("\n" + indent + "  ")
		fmt.Fprintf(&body, `<g transform="scale(%s) translate(%s %s)">`, trimFloat(scale), trimFloat(-cx), trimFloat(-cy))

		if opts["mode"] == "mono" {
			fill := opts["fill"]
			if fill == "" {
				fill = "currentColor"
			}
			fmt.Fprintf(&body, `<path fill="%s" fill-rule="evenodd" d="%s%s"/>`, fill, g.layers["outline"], g.layers["markings"])
		} else {
			for _, id := range g.order {
				fmt.Fprintf(&body, `<path fill="%s" fill-rule="evenodd" d="%s"/>`, layerFill(id), g.layers[id])
			}
		}
		body.WriteString("</g>\n" + indent)
		return m[1] + body.String() + m[3]
	})
	return out, failure
}

// layerFill is the colour each traced layer carries in a full-colour mark.
func layerFill(id string) string {
	switch id {
	case "outline":
		return "#20313f"
	case "fur":
		return "#fc7e05"
	default:
		return "#ffffff"
	}
}

func parseOptions(s string) map[string]string {
	opts := map[string]string{}
	for _, m := range optionRe.FindAllStringSubmatch(s, -1) {
		opts[m[1]] = strings.Trim(m[2], `"`)
	}
	return opts
}

// slotIndent returns the whitespace the slot's opening marker sits behind, so
// the injected group lines up with the hand-written markup around it.
func slotIndent(doc, slot string) string {
	idx := strings.Index(doc, slot)
	if idx < 0 {
		return ""
	}
	start := strings.LastIndexByte(doc[:idx], '\n') + 1
	return doc[start:idx]
}

func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// ---- rendering ----

func newBrowser() (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.Flag("hide-scrollbars", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	timeoutCtx, cancelTimeout := context.WithTimeout(browserCtx, 3*time.Minute)
	return timeoutCtx, func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	}
}

// render draws one SVG at an exact pixel size on a transparent background. A
// height of 0 keeps the source's aspect ratio.
//
// The SVG is inlined into the page rather than loaded from disk: an <img> to a
// file:// URL is subject to Chrome's file access rules, and the artwork has no
// external references, so there is nothing to lose by inlining it.
func render(ctx context.Context, root, src string, width, height int) (image.Image, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(src)))
	if err != nil {
		return nil, err
	}
	svg := string(data)
	if height == 0 {
		box := viewBoxRe.FindStringSubmatch(svg)
		if box == nil {
			return nil, fmt.Errorf("%s has no viewBox and no height was given", src)
		}
		w, _ := strconv.ParseFloat(box[3], 64)
		h, _ := strconv.ParseFloat(box[4], 64)
		height = int(float64(width)*h/w + 0.5)
	}

	html := fmt.Sprintf(`<!doctype html><meta charset="utf-8">`+
		`<style>html,body{margin:0;padding:0;background:transparent}`+
		`svg{display:block;width:%dpx;height:%dpx}</style>%s`, width, height, svg)

	var buf []byte
	err = chromedp.Run(ctx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			frame, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(frame.Frame.ID, html).Do(ctx)
		}),
		emulation.SetDeviceMetricsOverride(int64(width), int64(height), 1, false),
		emulation.SetDefaultBackgroundColorOverride().WithColor(&cdp.RGBA{}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(buf))
}

// ---- output ----

func writePNG(root, rel string, img image.Image) error {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return err
	}
	return writeFile(root, rel, buf.Bytes())
}

func writeFile(root, rel string, data []byte) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
