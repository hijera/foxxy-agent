//go:build miniapps

package miniapps

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

const (
	maxPortableRuntimeBytes = int64(2 << 30)
	maxPortableFiles        = 10000
)

var dependencyLocks sync.Map

type dependencyLock struct {
	mu sync.Mutex
}

type runtimeLockEntry struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256,omitempty"`
	Executable string `json:"executable"`
}

type dependencyLockFile struct {
	Runtimes []runtimeLockEntry `json:"runtimes"`
}

func (r *Runner) prepareDependencies(ctx context.Context, app MiniApp) (map[string]any, error) {
	resolved := make(map[string]any)
	lock := dependencyLockFile{}
	for _, required := range app.Requirements.Executables {
		path, err := exec.LookPath(required.Name)
		if err != nil {
			return nil, fmt.Errorf("required executable %q is unavailable", required.Name)
		}
		resolved[required.Name] = path
	}
	for _, requirement := range app.Requirements.Runtimes {
		executable, err := r.prepareRuntime(ctx, app, requirement)
		if err != nil {
			return nil, err
		}
		resolved[requirement.ID] = executable
		entry := runtimeLockEntry{ID: requirement.ID, Version: requirement.Version, Executable: executable}
		if requirement.Provision != nil {
			entry.SHA256 = strings.ToLower(requirement.Provision.SHA256)
		}
		lock.Runtimes = append(lock.Runtimes, entry)
	}
	sort.Slice(lock.Runtimes, func(i, j int) bool { return lock.Runtimes[i].ID < lock.Runtimes[j].ID })
	if len(lock.Runtimes) > 0 {
		lockPath := filepath.Join(r.runRoot(app), app.ID, "cache", "manifest.lock.json")
		if err := writeJSONAtomic(lockPath, lock, 0o600); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

func (r *Runner) prepareRuntime(ctx context.Context, app MiniApp, requirement Runtime) (string, error) {
	if requirement.Provision == nil {
		path, err := exec.LookPath(requirement.ID)
		if err != nil {
			return "", fmt.Errorf("runtime %q is unavailable and has no portable provision policy", requirement.ID)
		}
		return path, nil
	}
	policy := requirement.Provision
	if policy.Mode != "portable" || policy.Interaction != "silent_private" {
		return "", fmt.Errorf("runtime %q must use portable silent_private provisioning", requirement.ID)
	}
	if len(policy.SHA256) != sha256.Size*2 {
		return "", fmt.Errorf("runtime %q requires an exact SHA-256 checksum", requirement.ID)
	}
	if policy.SizeBytes <= 0 || policy.SizeBytes > maxPortableRuntimeBytes {
		return "", fmt.Errorf("runtime %q has an invalid portable size", requirement.ID)
	}
	parsed, err := url.Parse(policy.URL)
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf("runtime %q has an invalid provision URL", requirement.ID)
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !isLoopbackHost(parsed.Hostname())) {
		return "", fmt.Errorf("runtime %q provisioning requires HTTPS (HTTP is allowed only on loopback)", requirement.ID)
	}

	cacheRoot := filepath.Join(r.runRoot(app), app.ID, "cache", "runtimes")
	target := filepath.Join(cacheRoot, requirement.ID, safeCacheSegment(requirement.Version), strings.ToLower(policy.SHA256))
	key := filepath.Clean(target)
	entryValue, _ := dependencyLocks.LoadOrStore(key, &dependencyLock{})
	entry := entryValue.(*dependencyLock)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if executable, err := findRuntimeExecutable(target, requirement.ID); err == nil {
		return executable, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(filepath.Dir(target), "."+requirement.ID+"-staging-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	archivePath := filepath.Join(staging, "runtime.zip")
	if err := r.downloadRuntime(ctx, policy, archivePath); err != nil {
		return "", err
	}
	contentRoot := filepath.Join(staging, "content")
	if err := extractPortableZip(archivePath, contentRoot, policy.SizeBytes); err != nil {
		return "", fmt.Errorf("runtime %q: %w", requirement.ID, err)
	}
	if err := writeBundleFiles(filepath.Join(staging, "bundle"), r.bundleFiles); err != nil {
		return "", err
	}
	if policy.InstallScript != "" {
		if err := runPortableInstallScript(ctx, staging, contentRoot, policy.InstallScript); err != nil {
			return "", fmt.Errorf("runtime %q install script: %w", requirement.ID, err)
		}
	}
	executable, err := findRuntimeExecutable(contentRoot, requirement.ID)
	if err != nil {
		return "", fmt.Errorf("runtime %q: %w", requirement.ID, err)
	}
	relative, err := filepath.Rel(contentRoot, executable)
	if err != nil {
		return "", err
	}
	if err := os.Rename(contentRoot, target); err != nil {
		return "", err
	}
	return filepath.Join(target, relative), nil
}

func (r *Runner) downloadRuntime(ctx context.Context, policy *ProvisionPolicy, destination string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, policy.URL, nil)
	if err != nil {
		return err
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("download returned HTTP %d", response.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, policy.SizeBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != policy.SizeBytes {
		return fmt.Errorf("download size mismatch: got %d, expected %d", written, policy.SizeBytes)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, policy.SHA256) {
		return errors.New("download SHA-256 mismatch")
	}
	return nil
}

func extractPortableZip(archivePath, destination string, totalLimit int64) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("portable runtime must be a ZIP archive: %w", err)
	}
	defer func() { _ = archive.Close() }()
	if len(archive.File) > maxPortableFiles {
		return errors.New("portable runtime contains too many files")
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	var total int64
	for _, item := range archive.File {
		total += int64(item.UncompressedSize64)
		if total > totalLimit*4 || total > maxPortableRuntimeBytes {
			return errors.New("expanded portable runtime exceeds its limit")
		}
		clean := filepath.Clean(filepath.FromSlash(item.Name))
		if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe runtime path %q", item.Name)
		}
		target := filepath.Join(destination, clean)
		relative, err := filepath.Rel(destination, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe runtime path %q", item.Name)
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		source, err := item.Open()
		if err != nil {
			return err
		}
		destinationFile, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, item.Mode().Perm()|0o500)
		if err != nil {
			_ = source.Close()
			return err
		}
		_, copyErr := io.Copy(destinationFile, io.LimitReader(source, int64(item.UncompressedSize64)+1))
		closeDestinationErr := destinationFile.Close()
		closeSourceErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeDestinationErr != nil {
			return closeDestinationErr
		}
		if closeSourceErr != nil {
			return closeSourceErr
		}
	}
	return nil
}

func writeBundleFiles(root string, files map[string][]byte) error {
	for name, raw := range files {
		clean := filepath.Clean(filepath.FromSlash(name))
		if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe bundle path %q", name)
		}
		target := filepath.Join(root, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(target, raw, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func runPortableInstallScript(ctx context.Context, staging, contentRoot, scriptName string) error {
	clean := filepath.Clean(filepath.FromSlash(scriptName))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("install_script must reference a bundled relative file")
	}
	scriptPath := filepath.Join(staging, "bundle", clean)
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("bundled install script %q is unavailable", scriptName)
	}
	var command *exec.Cmd
	switch strings.ToLower(filepath.Ext(scriptPath)) {
	case ".ps1":
		command = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-File", scriptPath)
	case ".sh":
		command = exec.CommandContext(ctx, "sh", scriptPath)
	default:
		return errors.New("install_script must be a .ps1 or .sh file")
	}
	privateHome := filepath.Join(staging, "home")
	privateTemp := filepath.Join(staging, "tmp")
	if err := os.MkdirAll(privateHome, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(privateTemp, 0o700); err != nil {
		return err
	}
	command.Dir = contentRoot
	command.Env = append(os.Environ(),
		"FOXXYCODE_RUNTIME_ROOT="+contentRoot,
		"FOXXYCODE_BUNDLE_ROOT="+filepath.Join(staging, "bundle"),
		"FOXXYCODE_PRIVATE_HOME="+privateHome,
		"TMP="+privateTemp,
		"TEMP="+privateTemp,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func findRuntimeExecutable(root, id string) (string, error) {
	candidates := map[string]bool{
		strings.ToLower(id): true,
	}
	if runtime.GOOS == "windows" {
		candidates[strings.ToLower(id)+".exe"] = true
	}
	var found string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if candidates[strings.ToLower(entry.Name())] {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("executable %q was not found in the portable runtime", id)
	}
	return found, nil
}

func safeCacheSegment(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}
