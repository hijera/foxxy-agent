package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Sidecar filenames next to the active config.yaml.
const (
	lastGoodFileName = "config.lastgood.yaml"
	prevConfigName   = "config.prev.yaml"
)

// LastGoodPath returns the path to the last-known-good config backup for the given config file path.
func LastGoodPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), lastGoodFileName)
}

// PrevConfigPath returns the path to the rotated previous config copy.
func PrevConfigPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), prevConfigName)
}

// WriteLastGoodAtomic writes a copy of the primary config bytes to config.lastgood.yaml in the same directory.
func WriteLastGoodAtomic(configPath string, primaryBytes []byte) error {
	if strings.TrimSpace(configPath) == "" {
		return fmt.Errorf("config path is empty")
	}
	dst := LastGoodPath(configPath)
	return atomicWriteFile(dst, primaryBytes, 0o644)
}

// BackupConfigPrev copies the current config file to config.prev.yaml if it exists.
func BackupConfigPrev(configPath string) error {
	src := strings.TrimSpace(configPath)
	if src == "" {
		return fmt.Errorf("config path is empty")
	}
	st, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("config path is not a regular file")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return atomicWriteFile(PrevConfigPath(src), data, st.Mode().Perm()&0o666)
}

// AtomicWriteConfigYAML writes yamlBytes to configPath using a temp file and rename.
func AtomicWriteConfigYAML(configPath string, yamlBytes []byte) error {
	if strings.TrimSpace(configPath) == "" {
		return fmt.Errorf("config path is empty")
	}
	return atomicWriteFile(configPath, yamlBytes, 0o644)
}

func atomicWriteFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if perm == 0 {
		perm = 0o644
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// parseValidateYAMLBytes parses expanded YAML and validates (includes applyDefaults).
func parseValidateYAMLBytes(expanded string, paths Paths) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}
	cfg.Paths = paths
	applyDefaults(&cfg)
	if err := validateSubconfigs(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// tryRecoverFromLastGood loads config.lastgood.yaml; if valid, restores it over configPath and returns the config.
func tryRecoverFromLastGood(paths Paths) (*Config, error) {
	lg := LastGoodPath(paths.ConfigPath)
	raw, err := os.ReadFile(lg)
	if err != nil {
		return nil, err
	}
	expanded := os.ExpandEnv(ExpandPathVars(string(raw), paths))
	cfg, err := parseValidateYAMLBytes(expanded, paths)
	if err != nil {
		return nil, err
	}
	if err := AtomicWriteConfigYAML(paths.ConfigPath, raw); err != nil {
		return nil, err
	}
	return cfg, nil
}
