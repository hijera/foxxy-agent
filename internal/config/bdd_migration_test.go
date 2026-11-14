package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cucumber/godog"
)

func TestBDDConfigMigrationSafety(t *testing.T) {
	var paths Paths
	var cfg *Config
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a private legacy configuration with an environment skill source and a merged zero step limit$`, func() error {
				home := t.TempDir()
				t.Setenv("FOXXY_TEST_SKILL_SOURCE", "example/skills")
				paths = Paths{Home: home, CWD: home, ConfigPath: filepath.Join(home, "config.yaml")}
				return os.WriteFile(paths.ConfigPath, []byte("agent:\n  <<: {max_turns: 0}\nmcp_servers:\n  - name: private\n    command: test\n    env:\n      - name: TOKEN\n        value: literal-test-token\nskills:\n  sources:\n    - ${FOXXY_TEST_SKILL_SOURCE}\n"), 0o600)
			})
			sc.Step(`^the configuration is loaded$`, func() error {
				var err error
				cfg, err = LoadWithPaths(paths)
				return err
			})
			sc.Step(`^the migrated declarations are private and keep their values$`, func() error {
				servers, err := ReadMCPJSONFile(GlobalMCPJSONPath(paths.Home))
				if err != nil {
					return err
				}
				if servers["private"].Env["TOKEN"] != "literal-test-token" {
					return fmt.Errorf("MCP credential lost")
				}
				file, err := ReadMarketplacesFile(GlobalMarketplacesPath(paths.Home))
				if err != nil {
					return err
				}
				if len(file.Sources) != 1 || file.Sources[0] != "example/skills" {
					return fmt.Errorf("source changed: %v", file.Sources)
				}
				if runtime.GOOS != "windows" {
					for _, path := range []string{GlobalMCPJSONPath(paths.Home), GlobalMarketplacesPath(paths.Home)} {
						st, err := os.Stat(path)
						if err != nil {
							return err
						}
						if st.Mode().Perm()&0o077 != 0 {
							return fmt.Errorf("declarations are not private: %s mode %04o", path, st.Mode().Perm())
						}
					}
				}
				return nil
			})
			sc.Step(`^the agent step limit remains unlimited$`, func() error {
				if cfg.Agent.MaxTurns != 0 {
					return fmt.Errorf("step limit became %d", cfg.Agent.MaxTurns)
				}
				return nil
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/config_migration_safety.feature"}, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("configuration migration scenarios failed")
	}
}
