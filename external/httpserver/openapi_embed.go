//go:build http

package httpserver

import "embed"

//go:embed swagger-static/*
var swaggerStatic embed.FS
