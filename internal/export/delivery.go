package export

// Delivery metadata: what a surface needs once a document is rendered.

// ContentType is the media type a rendered export is served with.
func (f ExportFormat) ContentType() string {
	switch f {
	case ExportFormatMarkdown:
		return "text/markdown; charset=utf-8"
	case ExportFormatHTML:
		return "text/html; charset=utf-8"
	case ExportFormatJSON:
		return "application/json; charset=utf-8"
	case ExportFormatJSONL:
		return "application/x-ndjson; charset=utf-8"
	case ExportFormatPDF:
		return "application/pdf"
	case ExportFormatDOCX:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	return "application/octet-stream"
}

// WithAssetsDir points the document renderers at the session's assets
// directory, so the readable formats can embed pictures the session stored on
// disk. A session that was never persisted simply has none to embed.
func (d ExportDocument) WithAssetsDir(dir string) ExportDocument {
	d.assetsDir = dir
	return d
}
