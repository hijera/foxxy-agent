//go:build http && ui

package httpserver

import (
	"net/http"

	"github.com/hijera/foxxy-agent/external/ui"
)

func mountEmbeddedSPARoot(mux *http.ServeMux) {
	mux.Handle("/", uiEmbeddedSPAHandler(http.FS(ui.Assets)))
}

// uiEmbeddedSPAHandler serves the bundled SPA and sets Cache-Control on fixed asset paths
// so browsers revalidate after rebuilds (URLs have no content hash).
func uiEmbeddedSPAHandler(root http.FileSystem) http.Handler {
	next := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/index.html", "/app.js", "/styles.css",
			"/coddy-favicon.svg", "/favicon-32.png", "/favicon.ico", "/apple-touch-icon.png":
			w.Header().Set("Cache-Control", "no-cache")
		default:
		}
		next.ServeHTTP(w, r)
	})
}
