//go:build scheduler

package schedtools

import (
	"strings"

	"github.com/hijera/foxxy-agent/internal/tooling"
)

func toolEnvCWD(env *tooling.Env) string {
	if env == nil {
		return ""
	}
	return strings.TrimSpace(env.CWD)
}
