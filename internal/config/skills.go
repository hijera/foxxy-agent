package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Skills is the YAML skills section (key skills).
type Skills struct {
	Dirs []string `yaml:"dirs"`
}

// ManagedDir returns the directory used for foxxycode-managed skills (enable/disable state,
// UI-installed skills). Always resolves to ${FOXXYCODE_HOME}/skills or ~/.foxxycode/skills.
func (c *Skills) ManagedDir(foxxycodeHome string) string {
	if foxxycodeHome != "" {
		return filepath.Join(foxxycodeHome, "skills")
	}
	return expandSkillsHome("~/.foxxycode/skills")
}

// ApplyDefaults fills empty Dirs during config load.
func (c *Skills) ApplyDefaults(foxxycodeHome string, expandFOXXYCODEHome func(string) string) {
	if len(c.Dirs) == 0 {
		c.Dirs = []string{
			"~/.agents/skills",
			"${FOXXYCODE_HOME}/skills",
			"${CWD}/.foxxycode/skills",
		}
		return
	}
	for i := range c.Dirs {
		c.Dirs[i] = expandFOXXYCODEHome(c.Dirs[i])
	}
}

// Validate accepts any layout produced by ApplyDefaults.
func (c *Skills) Validate() error {
	return nil
}

func expandSkillsHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
