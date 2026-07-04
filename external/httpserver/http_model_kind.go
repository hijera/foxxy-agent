//go:build http

package httpserver

import (
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const ownedByFoxxyCodeSession = "foxxycode"

// httpModelListed reports whether sel is accepted as POST /v1 model (profile or configured completion ref).
func httpModelListed(cfg *config.Config, sel string) bool {
	if cfg == nil {
		return false
	}
	switch sel {
	case string(session.ModeAgent), string(session.ModePlan):
		return true
	default:
		_, _, err := config.SplitModelRef(sel)
		if err != nil {
			return false
		}
		return cfg.FindModelEntry(sel) != nil
	}
}

// httpModelIsFoxxyCodeProfile reports whether sel is agent or plan (no provider/rest form).
func httpModelIsFoxxyCodeProfile(sel string) bool {
	switch sel {
	case string(session.ModeAgent), string(session.ModePlan):
		return true
	default:
		return false
	}
}
