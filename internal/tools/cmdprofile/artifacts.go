package cmdprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
)

const maxRecordedArtifactBytes int64 = 256 << 20
const maxRecordedArtifactTotalBytes int64 = 512 << 20

type commandArtifact struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

// recordCommandArtifacts preserves the bytes observed when a command finishes.
// Reading today's workspace during distillation would mistake a changed file
// for the source result. Scalar commands keep their ordinary stdout contract.
func recordCommandArtifacts(spec cmdprofile.ProfileSpec, params map[string]string, cwd, stdout string) (string, error) {
	if cwd == "" {
		cwd = "."
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return stdout, nil
	}
	root, err := os.OpenRoot(cwd)
	if err != nil {
		return stdout, nil
	}
	defer func() { _ = root.Close() }()
	var artifacts []commandArtifact
	var total int64
	for _, param := range spec.Params {
		if len(artifacts) >= 16 {
			break
		}
		if !cmdprofile.IsOutputFileParam(param) {
			continue
		}
		name := params[param.Name]
		if filepath.IsAbs(name) {
			name, err = filepath.Rel(cwd, name)
			if err != nil {
				continue
			}
		}
		if !filepath.IsLocal(name) {
			continue
		}
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxRecordedArtifactBytes || info.Size() > maxRecordedArtifactTotalBytes-total {
			continue
		}
		file, err := root.Open(name)
		if err != nil {
			continue
		}
		hash := sha256.New()
		size, readErr := io.Copy(hash, io.LimitReader(file, maxRecordedArtifactBytes+1))
		_ = file.Close()
		if readErr != nil || size != info.Size() || size > maxRecordedArtifactBytes {
			continue
		}
		artifacts = append(artifacts, commandArtifact{Path: filepath.ToSlash(filepath.Clean(name)), SHA256: hex.EncodeToString(hash.Sum(nil)), SizeBytes: size})
		total += size
	}
	if len(artifacts) == 0 {
		return stdout, nil
	}
	data, err := json.Marshal(struct {
		Object    string            `json:"object"`
		Stdout    string            `json:"stdout"`
		Artifacts []commandArtifact `json:"artifacts"`
	}{Object: "foxxycode.command_result", Stdout: stdout, Artifacts: artifacts})
	return string(data), err
}
