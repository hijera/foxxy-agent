//go:build http && miniapps

package httpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Older sessions recorded turn snapshots. Keep their read adapter local to
// Mini Apps: current sessions rewind history without recording workspace diffs.
type miniAppsLegacyDiff struct {
	Changes []miniAppsLegacyChange `json:"changes"`
}

type miniAppsLegacyChange struct {
	Path  string              `json:"path"`
	After *miniAppsLegacyFile `json:"after,omitempty"`
}

type miniAppsLegacyFile struct {
	Content []byte `json:"content"`
}

func listMiniAppsLegacyDiffs(dir string) ([]int, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "diffs"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var turns []int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "turn_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "turn_"), ".json")); err == nil && n > 0 {
			turns = append(turns, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(turns)))
	return turns, nil
}

func loadMiniAppsLegacyDiff(dir string, turn int) (*miniAppsLegacyDiff, error) {
	data, err := os.ReadFile(filepath.Join(dir, "diffs", fmt.Sprintf("turn_%d.json", turn)))
	if err != nil {
		return nil, err
	}
	var diff miniAppsLegacyDiff
	if err := json.Unmarshal(data, &diff); err != nil {
		return nil, err
	}
	return &diff, nil
}
