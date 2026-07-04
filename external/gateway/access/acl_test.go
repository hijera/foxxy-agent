//go:build gateway || gateway.telegram

package access_test

import (
	"testing"

	"github.com/hijera/foxxycode-agent/external/gateway/access"
	"github.com/hijera/foxxycode-agent/internal/config"
)

func cfg() *config.TelegramGatewayConfig {
	return &config.TelegramGatewayConfig{
		Admins:           []int64{100},
		DefaultAccess:    config.AccessAll,
		DefaultIsolation: config.IsolationIndividual,
		UserGroups: []config.TelegramUserGroup{
			{Name: "devs", UserIDs: []int64{200, 300}},
		},
	}
}

func TestCanAccess_All(t *testing.T) {
	c := cfg()
	if !access.CanAccess(999, config.AccessAll, c) {
		t.Fatal("everyone should pass AccessAll")
	}
}

func TestCanAccess_AdminsOnly(t *testing.T) {
	c := cfg()
	if !access.CanAccess(100, config.AccessAdmins, c) {
		t.Fatal("admin should pass AccessAdmins")
	}
	if access.CanAccess(200, config.AccessAdmins, c) {
		t.Fatal("non-admin should not pass AccessAdmins")
	}
}

func TestCanAccess_Group(t *testing.T) {
	c := cfg()
	if !access.CanAccess(200, "group:devs", c) {
		t.Fatal("group member should pass")
	}
	if !access.CanAccess(100, "group:devs", c) {
		t.Fatal("admin should always pass group check")
	}
	if access.CanAccess(999, "group:devs", c) {
		t.Fatal("outsider should not pass group check")
	}
}

func TestEffectiveIsolation_Override(t *testing.T) {
	c := cfg()
	c.Chats = []config.TelegramChatConfig{
		{ChatID: -1001, Isolation: config.IsolationShared, Access: config.AccessAll},
	}
	if got := access.EffectiveIsolation(-1001, c); got != config.IsolationShared {
		t.Fatalf("want shared got %s", got)
	}
	if got := access.EffectiveIsolation(-9999, c); got != config.IsolationIndividual {
		t.Fatalf("want individual got %s", got)
	}
}
