//go:build http

package httpserver

import (
	"bytes"
	"image"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	// Registered for image.DecodeConfig so the renderers learn a picture's
	// natural size without decoding the pixels.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Image embedding for the readable export formats.
//
// Only pictures the server can resolve on its own disk are embedded: session
// assets (uploads, browser-tool screenshots) referenced either by the
// /foxxycode/sessions/{id}/assets/{name} URL the SPA uses, by a bare asset
// name, or by an absolute path inside the session's assets directory.
//
// Remote http(s) images are deliberately NOT fetched. A download request would
// otherwise make the server issue arbitrary outbound requests on the caller's
// behalf, and a slow or dead host would stall or fail the export. Those render
// as an italic caption plus a clickable link to the original URL instead.

// exportImage describes one picture referenced from a message. data is nil when
// the image could not be resolved locally, which every renderer treats as
// "caption and link only".
type exportImage struct {
	alt      string
	src      string
	mime     string
	data     []byte
	widthPx  int
	heightPx int
}

// embeddable reports whether this image has bytes a document can carry.
func (i *exportImage) embeddable() bool {
	return i != nil && len(i.data) > 0 && i.widthPx > 0 && i.heightPx > 0
}

// Bounds on what one document may carry. A transcript full of full-page
// screenshots would otherwise produce a DOCX nobody can mail.
const (
	exportImageMaxBytes = 4 << 20 // per image
	exportImageMaxCount = 20      // per document
)

// exportMediaResolver turns image destinations into bytes. A nil resolver, or
// one with no assets directory, resolves nothing — which is the correct
// behaviour for a session that was never persisted.
type exportMediaResolver struct {
	assetsDir string
	used      int
	cache     map[string]*exportImage
}

// newExportMediaResolver builds a resolver for one session's assets directory.
func newExportMediaResolver(assetsDir string) *exportMediaResolver {
	return &exportMediaResolver{assetsDir: assetsDir, cache: map[string]*exportImage{}}
}

// fill populates img's bytes and dimensions when its source resolves to a local
// asset within the caps. It leaves img untouched otherwise.
func (r *exportMediaResolver) fill(img *exportImage) {
	if r == nil || img == nil || r.assetsDir == "" {
		return
	}
	if cached, ok := r.cache[img.src]; ok {
		if cached != nil {
			img.data, img.mime, img.widthPx, img.heightPx = cached.data, cached.mime, cached.widthPx, cached.heightPx
		}
		return
	}
	if r.used >= exportImageMaxCount {
		r.cache[img.src] = nil
		return
	}
	loaded := r.load(img.src)
	r.cache[img.src] = loaded
	if loaded == nil {
		return
	}
	r.used++
	img.data, img.mime, img.widthPx, img.heightPx = loaded.data, loaded.mime, loaded.widthPx, loaded.heightPx
}

// load reads and measures one asset, or returns nil when src does not name a
// readable local image.
func (r *exportMediaResolver) load(src string) *exportImage {
	full := r.resolve(src)
	if full == "" {
		return nil
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() || info.Size() > exportImageMaxBytes {
		return nil
	}
	data, err := os.ReadFile(full) // #nosec G304 -- path is confined to the session assets dir by resolve
	if err != nil {
		return nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil
	}
	mime := imageMIME(format)
	if mime == "" {
		return nil
	}
	return &exportImage{data: data, mime: mime, widthPx: cfg.Width, heightPx: cfg.Height}
}

// resolve maps a markdown image destination onto an absolute path inside the
// session assets directory, or returns "" when it points anywhere else.
func (r *exportMediaResolver) resolve(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	// A data URL carries its own bytes and a remote one is not ours to read.
	if strings.HasPrefix(src, "data:") {
		return ""
	}

	name := src
	// An absolute filesystem path is checked before any URL parsing: on Windows
	// "C:\dir\shot.png" parses as a URL with scheme "c", and treating it as
	// remote would drop every uploaded file from the export.
	if !filepath.IsAbs(src) {
		if strings.Contains(src, "://") {
			return ""
		}
		if u, err := url.Parse(src); err == nil {
			if u.Scheme != "" && u.Scheme != "file" {
				return ""
			}
			if decoded, err := url.PathUnescape(u.Path); err == nil && u.Path != "" {
				name = decoded
			}
		}
	}
	// The SPA references assets as /foxxycode/sessions/{id}/assets/{name}; take
	// the trailing element of any path and let the containment check below decide
	// whether it really lives in this session's assets.
	name = path.Base(filepath.ToSlash(name))
	if name == "" || name == "." || name == string(filepath.Separator) || strings.HasPrefix(name, ".") {
		return ""
	}

	full := filepath.Join(r.assetsDir, name)
	rel, err := filepath.Rel(r.assetsDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return full
}

// imageMIME maps the format name image.DecodeConfig reports onto a media type.
// Only the three raster formats every target understands are accepted: fpdf can
// place PNG/JPEG/GIF, and Word needs a declared content type per extension.
func imageMIME(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	}
	return ""
}

// imageExt is the file extension for a media type, used for the DOCX media part
// name and its [Content_Types].xml default.
func imageExt(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpeg"
	case "image/gif":
		return "gif"
	}
	return "png"
}
