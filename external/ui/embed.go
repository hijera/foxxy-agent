//go:build http && ui

// Package ui holds static assets for foxxycode http (embedded into the binary).
package ui

import "embed"

//go:embed index.html styles.css app.js foxxycode-favicon.svg favicon-32.png favicon.ico apple-touch-icon.png
var Assets embed.FS
