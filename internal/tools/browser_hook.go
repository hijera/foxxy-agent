//go:build browser

package tools

import (
	"github.com/hijera/foxxycode-agent/internal/config"
	toolbrowser "github.com/hijera/foxxycode-agent/internal/tools/browser"
)

func registerBrowserTools(r *Registry, cfg *config.Config) {
	toolbrowser.RegisterBuiltins(r.Register, cfg)
}
