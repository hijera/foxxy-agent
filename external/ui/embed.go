//go:build http && ui

// Package ui holds static assets for coddy http (embedded into the binary).
package ui

import "embed"

//go:embed index.html styles.css app.js
var Assets embed.FS
