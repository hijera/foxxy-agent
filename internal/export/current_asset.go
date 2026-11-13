package export

import (
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// currentAssetPath follows the saved copy when its session bundle moves.
func currentAssetPath(sessionDir, recorded string) string {
	if strings.TrimSpace(sessionDir) == "" || recorded == "" || filepath.Base(filepath.Dir(recorded)) != "assets" {
		return recorded
	}
	return filepath.Join(session.AssetsPath(sessionDir), filepath.Base(recorded))
}
