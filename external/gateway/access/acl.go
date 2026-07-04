//go:build gateway || gateway.telegram

// Package access implements access control for the messenger gateway.
package access

import (
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// CanAccess reports whether userID is allowed to interact given the effective access level.
func CanAccess(userID int64, level config.AccessLevel, cfg *config.TelegramGatewayConfig) bool {
	switch level {
	case config.AccessAdmins:
		return cfg.IsAdmin(userID)
	case config.AccessAll:
		return true
	default:
		// "group:<name>" prefix
		if name, ok := groupName(string(level)); ok {
			ids := cfg.UserGroupIDs(name)
			for _, id := range ids {
				if id == userID {
					return true
				}
			}
			// Admins always pass group checks too.
			return cfg.IsAdmin(userID)
		}
		return false
	}
}

// EffectiveAccess returns the per-chat access override or the global default.
func EffectiveAccess(chatID int64, cfg *config.TelegramGatewayConfig) config.AccessLevel {
	if cc := cfg.ChatConfig(chatID); cc != nil && cc.Access != "" {
		return cc.Access
	}
	return cfg.DefaultAccess
}

// EffectiveIsolation returns the per-chat isolation mode or the global default.
func EffectiveIsolation(chatID int64, cfg *config.TelegramGatewayConfig) config.IsolationMode {
	if cc := cfg.ChatConfig(chatID); cc != nil && cc.Isolation != "" {
		return cc.Isolation
	}
	return cfg.DefaultIsolation
}

func groupName(s string) (string, bool) {
	const prefix = "group:"
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):], true
	}
	return "", false
}
