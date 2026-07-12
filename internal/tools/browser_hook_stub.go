//go:build !browser

package tools

import "github.com/hijera/foxxycode-agent/internal/config"

func registerBrowserTools(_ *Registry, _ *config.Config) {}
