//go:build http

package httpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The resolver is the security boundary of the image feature: it decides which
// bytes a download request is allowed to pull off the server's disk.
func TestExportMediaResolverAcceptsSessionAssets(t *testing.T) {
	dir := t.TempDir()
	abs := writePNGFixture(t, dir, "shot.png", 32, 16)
	r := newExportMediaResolver(dir)

	for _, src := range []string{
		"shot.png",                               // a bare asset name
		"./shot.png",                             // a relative reference
		"/foxxycode/sessions/s1/assets/shot.png", // the URL the SPA renders
		abs,                                      // the absolute path the uploads wrapper records
	} {
		img := &exportImage{src: src}
		r.fill(img)
		if !img.embeddable() {
			t.Errorf("%q did not resolve to an embeddable asset", src)
			continue
		}
		if img.mime != "image/png" || img.widthPx != 32 || img.heightPx != 16 {
			t.Errorf("%q resolved to %s %dx%d", src, img.mime, img.widthPx, img.heightPx)
		}
	}
}

func TestExportMediaResolverRefusesEverythingElse(t *testing.T) {
	dir := t.TempDir()
	writePNGFixture(t, dir, "shot.png", 8, 8)

	// A file next to the assets directory must stay unreachable.
	outside := filepath.Join(filepath.Dir(dir), "secret.png")
	if err := os.WriteFile(outside, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(outside) }()

	r := newExportMediaResolver(dir)
	for _, src := range []string{
		"https://example.com/chart.png", // remote: never fetched
		"http://10.0.0.1/internal.png",  // remote, including private ranges
		"data:image/png;base64,AAAA",    // already inline
		"../secret.png",                 // traversal
		outside,                         // an absolute path outside the assets dir
		"missing.png",                   // simply not there
		"",
	} {
		img := &exportImage{src: src}
		r.fill(img)
		if img.embeddable() {
			t.Errorf("%q should not have resolved", src)
		}
	}
}

// A non-image file with an image name must not be embedded: DecodeConfig is the
// gate, not the extension.
func TestExportMediaResolverRejectsNonImages(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.png"), []byte("plain text"), 0o644); err != nil {
		t.Fatal(err)
	}
	img := &exportImage{src: "notes.png"}
	newExportMediaResolver(dir).fill(img)
	if img.embeddable() {
		t.Error("a text file named .png was treated as an image")
	}
}

func TestExportMediaResolverEnforcesCaps(t *testing.T) {
	dir := t.TempDir()
	r := newExportMediaResolver(dir)
	for i := 0; i <= exportImageMaxCount; i++ {
		writePNGFixture(t, dir, "shot"+itoa(i)+".png", 8, 8)
	}
	embedded := 0
	for i := 0; i <= exportImageMaxCount; i++ {
		img := &exportImage{src: "shot" + itoa(i) + ".png"}
		r.fill(img)
		if img.embeddable() {
			embedded++
		}
	}
	if embedded != exportImageMaxCount {
		t.Errorf("embedded %d images, want the %d cap", embedded, exportImageMaxCount)
	}
}

// A nil resolver is the "session was never persisted" case and must be inert
// rather than a crash.
func TestExportMediaResolverNilIsInert(t *testing.T) {
	var r *exportMediaResolver
	img := &exportImage{src: "shot.png"}
	r.fill(img)
	if img.embeddable() {
		t.Error("a nil resolver resolved something")
	}
}

func TestParseExportAttachments(t *testing.T) {
	content := "Look\n\n<foxxycode_session_assets>Uploaded files saved to session assets:\n" +
		"- /tmp/a_1.png (a.png)\n" +
		"- /tmp/report.pdf\n" +
		"</foxxycode_session_assets>"

	atts := parseExportAttachments(content)
	if len(atts) != 2 {
		t.Fatalf("expected 2 attachments, got %+v", atts)
	}
	// A renamed upload keeps the name the user gave it and the path it was saved
	// under, so the document can show one and read the other.
	if atts[0].Name != "a.png" || atts[0].Path != "/tmp/a_1.png" {
		t.Errorf("renamed upload parsed as %+v", atts[0])
	}
	// Without a display name the file name stands in for it.
	if atts[1].Name != "report.pdf" || atts[1].Path != "/tmp/report.pdf" {
		t.Errorf("plain upload parsed as %+v", atts[1])
	}
	if got := parseExportAttachments("no wrapper here"); got != nil {
		t.Errorf("a turn without uploads reported %+v", got)
	}
}

func TestImageMIMEAndExt(t *testing.T) {
	for format, mime := range map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"} {
		if got := imageMIME(format); got != mime {
			t.Errorf("imageMIME(%q) = %q, want %q", format, got, mime)
		}
		if got := imageExt(mime); got != format {
			t.Errorf("imageExt(%q) = %q, want %q", mime, got, format)
		}
	}
	// Formats no target can place are refused rather than mislabelled.
	for _, format := range []string{"webp", "bmp", "tiff", ""} {
		if got := imageMIME(format); got != "" {
			t.Errorf("imageMIME(%q) = %q, want it refused", format, got)
		}
	}
	if !strings.HasPrefix(imageExt("image/png"), "png") {
		t.Error("imageExt lost the png mapping")
	}
}
