//go:build http

package httpserver

import (
	"archive/zip"
	"io"
)

// docxWriter wraps a zip.Writer that streams a minimal OOXML (.docx) package
// into an io.Writer. DOCX is a zip of well-known XML parts, so this is the only
// machinery the export needs beyond the document XML produced by the renderers.
type docxWriter struct {
	w *zip.Writer
}

func newDocxWriter(w io.Writer) *docxWriter {
	return &docxWriter{w: zip.NewWriter(w)}
}

func (d *docxWriter) write(name, content string) error {
	fw, err := d.w.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(fw, content)
	return err
}

func (d *docxWriter) close() error {
	return d.w.Close()
}
