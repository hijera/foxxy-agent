package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/gitws"
	"github.com/hijera/foxxycode-agent/internal/tools/web"
)

// sourceMu serializes mutations of skills.sources (add/remove) so concurrent
// /plugin commands cannot corrupt the slice or race on the config.yaml write.
var sourceMu sync.Mutex

// syncMu serializes materialization into the managed skills dir so concurrent
// Sync/UpdateSkill calls cannot race on the shared staging directories.
var syncMu sync.Mutex

// remoteGuard is the SSRF guard every http(s) address of a remote source goes
// through: a clone URL before git runs, and a marketplace.json or a plugin
// archive together with each redirect on the way to it. It is a variable so
// tests can reach an httptest server on loopback.
var remoteGuard = func(ctx context.Context, rawURL string) error {
	_, err := web.ValidateFetchURL(ctx, rawURL)
	return err
}

// remoteTransport carries the http(s) downloads of remote sources; nil means
// http.DefaultTransport. It is a variable so tests can trust the certificate of
// an httptest TLS server.
var remoteTransport http.RoundTripper

// maxRemoteRedirects bounds the redirects one remote download follows.
const maxRemoteRedirects = 5

// remoteClient is the client a remote download runs on: at most
// maxRemoteRedirects redirects, each target vetted by check before it is
// contacted.
func remoteClient(timeout time.Duration, check func(ctx context.Context, rawURL string) error) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: remoteTransport,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			// via holds the requests made so far, the first one included.
			if len(via) > maxRemoteRedirects {
				return fmt.Errorf("too many redirects")
			}
			if err := check(r.Context(), r.URL.String()); err != nil {
				return fmt.Errorf("redirect not allowed: %w", err)
			}
			return nil
		},
	}
}

// safeClone applies the SSRF guard to http(s) clone URLs (blocking loopback /
// private hosts reachable over http(s), including those coming from a
// marketplace manifest) before cloning. Operator-chosen local/SSH transports
// (file://, git@host:path) are cloned as-is — they are not network SSRF vectors
// and file:// backs offline marketplaces and the tests.
func safeClone(url, ref, dest string) error {
	low := strings.ToLower(strings.TrimSpace(url))
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		if err := remoteGuard(context.Background(), url); err != nil {
			return fmt.Errorf("clone url not allowed: %w", err)
		}
	}
	return gitws.Clone(url, ref, dest)
}

// remoteLockFile is the provenance sidecar written into the managed skills dir.
const remoteLockFile = ".remote.json"

// maxManifestBytes caps an API marketplace.json download.
const maxManifestBytes = 4 << 20

// maxWalkDepth bounds the recursive SKILL.md search inside a clone.
const maxWalkDepth = 6

// RemoteEntry records where an installed skill came from (one per skill dir).
type RemoteEntry struct {
	Source  string `json:"source"`            // the configured source string
	Repo    string `json:"repo,omitempty"`    // git URL the skill was cloned from
	Ref     string `json:"ref,omitempty"`     // branch or tag
	URL     string `json:"url,omitempty"`     // API marketplace URL, when applicable
	Archive string `json:"archive,omitempty"` // zip archive URL the skill was unpacked from
	Plugin  string `json:"plugin,omitempty"`  // marketplace plugin entry name (for update lookup)
	Version string `json:"version,omitempty"` // installed version, as declared at sync time
}

// SyncResult summarizes a Sync run.
type SyncResult struct {
	Added   []string      `json:"added"`
	Updated []string      `json:"updated"`
	Failed  []SyncFailure `json:"failed"`
}

// SyncFailure is one source that could not be processed.
type SyncFailure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

// sourceSpec is a classified top-level config source.
type sourceSpec struct {
	kind string // "git" | "api"
	url  string // git clone URL or API URL
	ref  string // branch/tag for git
}

// parseSource classifies a configured skills.sources entry.
//
//	owner/repo             → git https://github.com/owner/repo
//	owner/repo@ref         → git, ref
//	https://github.com/owner/repo(.git) → git
//	git@host:path / *.git  → git
//	https://host/marketplace.json (or any other http[s]) → api
func parseSource(raw string) (sourceSpec, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return sourceSpec{}, fmt.Errorf("empty source")
	}

	// scp-style git URL: git@host:owner/repo(.git)
	if strings.HasPrefix(s, "git@") {
		return sourceSpec{kind: "git", url: s}, nil
	}

	if strings.Contains(s, "://") {
		low := strings.ToLower(s)
		if strings.HasPrefix(low, "file://") || isGitCloneURL(low) {
			return sourceSpec{kind: "git", url: s}, nil
		}
		return sourceSpec{kind: "api", url: s}, nil
	}

	// No scheme: treat as owner/repo[@ref] GitHub shorthand.
	ref := ""
	repo := s
	if at := strings.LastIndex(s, "@"); at >= 0 {
		repo = s[:at]
		ref = s[at+1:]
	}
	repo = strings.TrimSuffix(repo, "/")
	if strings.Count(repo, "/") != 1 || strings.HasPrefix(repo, "/") || strings.HasSuffix(repo, "/") {
		return sourceSpec{}, fmt.Errorf("unrecognized source %q (expected owner/repo, a git URL, or an http(s) marketplace URL)", raw)
	}
	return sourceSpec{kind: "git", url: "https://github.com/" + repo, ref: ref}, nil
}

// isGitCloneURL reports whether a scheme'd URL should be cloned rather than
// fetched as an API marketplace. github.com/owner/repo and *.git are git.
func isGitCloneURL(lowerURL string) bool {
	if strings.HasSuffix(lowerURL, ".git") {
		return true
	}
	if strings.HasSuffix(lowerURL, ".json") {
		return false
	}
	for _, host := range []string{"github.com/", "gitlab.com/", "bitbucket.org/"} {
		if i := strings.Index(lowerURL, host); i >= 0 {
			rest := strings.Trim(lowerURL[i+len(host):], "/")
			if rest != "" && strings.Count(rest, "/") == 1 {
				return true
			}
		}
	}
	return false
}

// Sync fetches every configured source and materializes skills into the
// managed dir. It never runs automatically; callers invoke it explicitly.
func Sync(ctx context.Context, cfg *config.Config) (*SyncResult, error) {
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create managed dir: %w", err)
	}
	lock := readRemoteLock(managedDir)
	res := &SyncResult{}

	for _, src := range ListSources(cfg) {
		if err := syncOne(ctx, src, managedDir, lock, res); err != nil {
			res.Failed = append(res.Failed, SyncFailure{Source: src, Error: err.Error()})
		}
	}
	// Added marketplaces: their lists and the plugins installed from them, not
	// every plugin they list.
	syncAddedLocked(ctx, cfg, managedDir, lock, res)

	if err := writeRemoteLock(managedDir, lock); err != nil {
		return res, fmt.Errorf("write lock: %w", err)
	}
	return res, nil
}

// SyncSource fetches a single source (a GitHub owner/repo, git URL, or
// marketplace.json URL) and materializes its skills, independent of whether the
// source is listed in skills.sources. Backs `plugin marketplace sync <src>`.
func SyncSource(ctx context.Context, cfg *config.Config, source string) (*SyncResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("empty source")
	}
	if _, err := parseSource(source); err != nil {
		return nil, err
	}
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create managed dir: %w", err)
	}
	lock := readRemoteLock(managedDir)
	res := &SyncResult{}
	if err := syncOne(ctx, source, managedDir, lock, res); err != nil {
		res.Failed = append(res.Failed, SyncFailure{Source: source, Error: err.Error()})
	}
	if err := writeRemoteLock(managedDir, lock); err != nil {
		return res, fmt.Errorf("write lock: %w", err)
	}
	return res, nil
}

// SkillReadonly reports whether a loaded skill cannot be deleted from disk
// (bundled skills carry a relative virtual FilePath; everything the loader found
// on disk is absolute and therefore removable).
func SkillReadonly(sk *Skill) bool {
	return sk == nil || !filepath.IsAbs(sk.FilePath)
}

// DeleteSkill removes any on-disk skill by canonical name (not just remote ones),
// with its remote lock entry when present. Bundled skills are read-only and
// cannot be deleted. cwd expands ${CWD} in skills.dirs for lookup.
func DeleteSkill(cfg *config.Config, cwd, skillName string) error {
	name, err := sanitizeSkillName(skillName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)

	loader := NewLoader(cfg.Skills.Dirs)
	loaded, err := loader.LoadAll(cwd, cfg.Paths.Home, managedDir)
	if err != nil {
		return err
	}
	var target *Skill
	for _, sk := range loaded {
		if CanonicalCommandName(sk) == name {
			target = sk
			break
		}
	}
	if target == nil {
		return fmt.Errorf("skill %q not found", name)
	}
	if SkillReadonly(target) {
		return fmt.Errorf("skill %q is read-only (bundled) and cannot be deleted", name)
	}
	victim, err := skillDeletePath(cfg, cwd, target.FilePath)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(victim); err != nil {
		return fmt.Errorf("remove skill: %w", err)
	}
	if lock := readRemoteLock(managedDir); lock != nil {
		if _, ok := lock[name]; ok {
			delete(lock, name)
			_ = writeRemoteLock(managedDir, lock)
		}
	}
	return nil
}

// skillDeletePath resolves what to remove for a skill file: its containing
// directory for a `<dir>/SKILL.md`, or the file itself for a root `.md`/`.mdc`.
// It refuses paths that are not strictly inside a configured skills directory.
func skillDeletePath(cfg *config.Config, cwd, filePath string) (string, error) {
	base := filepath.Base(filePath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	victim := filePath
	if strings.EqualFold(stem, "SKILL") {
		victim = filepath.Dir(filePath)
	}
	victim = filepath.Clean(victim)
	for _, d := range cfg.Skills.Dirs {
		root := filepath.Clean(ExpandConfiguredPath(d, cwd, cfg.Paths.Home))
		if victim == root {
			return "", fmt.Errorf("refusing to delete the skills directory itself")
		}
		if strings.HasPrefix(victim, root+string(filepath.Separator)) {
			return victim, nil
		}
	}
	return "", fmt.Errorf("refusing to delete skill outside configured skill directories")
}

func syncOne(ctx context.Context, src, managedDir string, lock map[string]RemoteEntry, res *SyncResult) error {
	spec, err := parseSource(src)
	if err != nil {
		return err
	}

	switch spec.kind {
	case "api":
		mf, err := fetchManifestHTTP(ctx, spec.url)
		if err != nil {
			return err
		}
		return installMarketplace(ctx, mf, "", src, RemoteEntry{Source: src, URL: spec.url}, managedDir, lock, res)

	case "git":
		tmp, err := os.MkdirTemp("", "foxxycode-skillsrc-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		clone := filepath.Join(tmp, "repo")
		if err := safeClone(spec.url, spec.ref, clone); err != nil {
			return fmt.Errorf("clone %s: %w", spec.url, err)
		}
		base := RemoteEntry{Source: src, Repo: spec.url, Ref: spec.ref}
		if mfPath := findMarketplaceFile(clone); mfPath != "" {
			mf, err := parseMarketplace(mfPath)
			if err != nil {
				return fmt.Errorf("parse manifest: %w", err)
			}
			return installMarketplace(ctx, mf, clone, src, base, managedDir, lock, res)
		}
		// No manifest: treat the whole clone as a skill container.
		return installFromDir(clone, base, managedDir, lock, res)

	default:
		return fmt.Errorf("unsupported source kind %q", spec.kind)
	}
}

// installMarketplace resolves every plugin in a manifest and installs its skills.
// repoRoot is the marketplace clone (for relative path sources); "" for API manifests.
func installMarketplace(ctx context.Context, mf *Marketplace, repoRoot, src string, base RemoteEntry, managedDir string, lock map[string]RemoteEntry, res *SyncResult) error {
	var firstErr error
	for _, p := range mf.Plugins {
		if err := installPlugin(ctx, p, repoRoot, src, base, managedDir, lock, res); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func installPlugin(ctx context.Context, p MarketplacePlugin, repoRoot, src string, base RemoteEntry, managedDir string, lock map[string]RemoteEntry, res *SyncResult) error {
	entry := base
	entry.Plugin = strings.TrimSpace(p.Name)
	entry.Version = strings.TrimSpace(p.Version)

	switch p.Source.Kind {
	case "github", "url":
		cloneURL := p.Source.URL
		if p.Source.Kind == "github" {
			cloneURL = "https://github.com/" + strings.Trim(p.Source.Repo, "/")
		}
		if cloneURL == "" {
			return fmt.Errorf("plugin %q: empty source url", p.Name)
		}
		tmp, err := os.MkdirTemp("", "foxxycode-plugin-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		dst := filepath.Join(tmp, "repo")
		if err := safeClone(cloneURL, p.Source.Ref, dst); err != nil {
			return fmt.Errorf("clone plugin %q: %w", p.Name, err)
		}
		entry.Repo = cloneURL
		entry.Ref = p.Source.Ref
		return installPluginDir(dst, p, entry, managedDir, lock, res)

	case "path":
		if repoRoot == "" {
			return fmt.Errorf("plugin %q: relative path source requires a repository (not supported for API sources)", p.Name)
		}
		dir := filepath.Join(repoRoot, filepath.Clean("/"+p.Source.Path))
		return installPluginDir(dir, p, entry, managedDir, lock, res)

	case "archive":
		return installArchivePlugin(ctx, p, entry, managedDir, lock, res)

	default:
		return fmt.Errorf("plugin %q: unsupported source kind %q", p.Name, p.Source.Kind)
	}
}

// An inline skills list scopes a plugin whose source is a shared repository.
func installPluginDir(root string, p MarketplacePlugin, entry RemoteEntry, managedDir string, lock map[string]RemoteEntry, res *SyncResult) error {
	paths, declared, err := decodeSkillPaths(p.Skills, "marketplace plugin "+p.Name)
	if err != nil {
		return err
	}
	if !declared {
		return installFromDir(root, entry, managedDir, lock, res)
	}
	hits, err := declaredSkillDirs(root, "marketplace.json", paths, true)
	if err != nil {
		return err
	}
	return installSkillDirs(hits, entry, managedDir, lock, res)
}

// installFromDir finds every skill dir under root and copies each into managedDir.
func installFromDir(root string, entry RemoteEntry, managedDir string, lock map[string]RemoteEntry, res *SyncResult) error {
	hits := locateSkillDirs(root)
	if len(hits) == 0 {
		return fmt.Errorf("no SKILL.md found under %s", filepath.Base(root))
	}
	return installSkillDirs(hits, entry, managedDir, lock, res)
}

// installSkillDirs copies each skill dir in hits into managedDir and records it
// in the lock.
func installSkillDirs(hits []skillHit, entry RemoteEntry, managedDir string, lock map[string]RemoteEntry, res *SyncResult) error {
	var firstErr error
	for _, h := range hits {
		name, err := sanitizeSkillName(h.name)
		if err == nil && strings.HasPrefix(name, ".") {
			// The managed dir keeps its own files under dot names (.remote.json,
			// .marketplaces.json, staging and backup copies), and the loader skips
			// dot names, so such a skill could only take the place of one of them.
			err = fmt.Errorf("skill name %q starts with a dot, which the skills directory keeps for its own files", name)
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		dst := filepath.Join(managedDir, name)
		_, existed := os.Stat(dst)
		// Copy into a sibling temp dir, then let replaceSkillDir swap it in.
		tmpDst := stagingDir(managedDir, name)
		_ = os.RemoveAll(tmpDst)
		if err := copySkillDir(h.dir, tmpDst); err != nil {
			_ = os.RemoveAll(tmpDst)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := replaceSkillDir(managedDir, name, tmpDst); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ent := entry
		if ent.Version == "" {
			ent.Version = skillDirVersion(h.dir)
		}
		lock[name] = ent
		if existed == nil {
			res.Updated = append(res.Updated, name)
		} else {
			res.Added = append(res.Added, name)
		}
	}
	return firstErr
}

// The sidecar directories a skill install uses: the staged copy it is built in,
// and the backup the previous install moves aside to. Both carry the pid.
// syncMu keeps one process from using a name twice; the pid keeps two processes
// apart, and a console and a `foxxycode serve` starting at once on a fresh home do
// both hand over the standard delivery. Without it, one process copying into
// the other's staging directory yields a tree neither wrote, and one process
// clearing the other's backup destroys the only copy of what was there. A
// leading dot keeps the loader from reading either as a skill.
const (
	stagingPrefix = ".tmp-"
	backupPrefix  = ".bak-"
)

func stagingDir(managedDir, name string) string {
	return filepath.Join(managedDir, fmt.Sprintf("%s%s-%d", stagingPrefix, name, os.Getpid()))
}

func backupDir(managedDir, name string) string {
	return filepath.Join(managedDir, fmt.Sprintf("%s%s-%d", backupPrefix, name, os.Getpid()))
}

// RecoverInterruptedInstalls repairs what a process killed mid-swap left
// behind. Replacing a skill is a rename of the old copy aside followed by a
// rename of the new one into place, and between the two the skill is not there
// at all: a process that dies in that window leaves a backup and no skill, and
// nothing would ever put it back - the delivery would read the gap as a skill
// the operator deleted. So every backup is resolved before anything else runs:
// the skill is missing, and the backup is what it was, or the swap did finish
// and the backup is the leftover of its last step.
func RecoverInterruptedInstalls(managedDir string) {
	entries, err := os.ReadDir(managedDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), backupPrefix) {
			continue
		}
		suffix := strings.TrimPrefix(e.Name(), backupPrefix)
		cut := strings.LastIndex(suffix, "-")
		if cut <= 0 {
			continue
		}
		name, err := sanitizeSkillName(suffix[:cut])
		if err != nil {
			continue
		}
		bak := filepath.Join(managedDir, e.Name())
		dst := filepath.Join(managedDir, name)
		if _, err := os.Stat(dst); err == nil {
			_ = os.RemoveAll(bak)
			continue
		}
		_ = renameSkillDir(bak, dst)
	}
}

// replaceSkillDir swaps a staged copy in as the skill named name: any existing
// install moves aside to a backup, the staged copy takes its place, and the
// backup is dropped - so neither a copy nor a rename failure can leave the
// skill deleted or half-written. Shared by the marketplace installer and the
// standard delivery.
func replaceSkillDir(managedDir, name, staged string) error {
	dst := filepath.Join(managedDir, name)
	bak := backupDir(managedDir, name)
	_ = os.RemoveAll(bak)

	movedAside := false
	if _, err := os.Stat(dst); err == nil {
		if err := renameSkillDir(dst, bak); err != nil {
			_ = os.RemoveAll(staged)
			return err
		}
		movedAside = true
	}
	if err := renameSkillDir(staged, dst); err != nil {
		_ = os.RemoveAll(staged)
		if movedAside {
			if rbErr := renameSkillDir(bak, dst); rbErr != nil {
				// Both the swap and the rollback failed: keep the backup and
				// surface where the previous copy is so it can be recovered.
				return fmt.Errorf("install %q failed (%w) and rollback failed (%v); previous copy left at %s", name, err, rbErr, bak)
			}
		}
		return err
	}
	_ = os.RemoveAll(bak)
	return nil
}

// Windows readers may briefly hold a directory without sharing delete. Keep
// the atomic swap and its rollback, waiting at most 450 ms for those handles.
func renameSkillDir(from, to string) error {
	const (
		accessDenied     = syscall.Errno(5)
		sharingViolation = syscall.Errno(32)
	)
	for attempt := 0; ; attempt++ {
		err := os.Rename(from, to)
		if err == nil || runtime.GOOS != "windows" || attempt == 9 ||
			(!errors.Is(err, accessDenied) && !errors.Is(err, sharingViolation)) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
}

// skillDirVersion reads the optional version from a skill directory's SKILL.md
// frontmatter. Used when a marketplace plugin entry does not declare a version.
func skillDirVersion(dir string) string {
	if s, err := loadFile(filepath.Join(dir, "SKILL.md")); err == nil {
		return strings.TrimSpace(s.Version)
	}
	return ""
}

// skillHit is a discovered skill directory (the one holding SKILL.md).
type skillHit struct {
	dir  string
	name string
}

// locateSkillDirs recursively finds directories containing a SKILL.md under
// root (any depth up to maxWalkDepth), skipping .git and node_modules. It does
// not hardcode skills/ or plugins/, so it handles root, skills/<name>/,
// .claude/skills/<name>/, and plugins/<p>/skills/<s>/ layouts alike.
//
// Duplicate skill names (e.g. a root SKILL.md plus a nested skills/<name>/SKILL.md)
// collapse to one hit, preferring the deeper (resource-colocated) directory.
func locateSkillDirs(root string) []skillHit {
	byName := map[string]skillHit{}
	depthOf := map[string]int{}

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxWalkDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
			name := skillNameForDir(dir, root)
			if name != "" {
				if prev, ok := byName[name]; !ok || depth > depthOf[name] {
					byName[name] = skillHit{dir: dir, name: name}
					depthOf[name] = depth
					_ = prev
				}
			}
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			n := e.Name()
			if n == ".git" || n == "node_modules" {
				continue
			}
			walk(filepath.Join(dir, n), depth+1)
		}
	}
	walk(root, 0)

	out := make([]skillHit, 0, len(byName))
	for _, h := range byName {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// skillNameForDir derives a skill's canonical name. Precedence: SKILL.md
// frontmatter name, then .claude-plugin/plugin.json name, then the directory
// basename. At the clone root the basename is a throwaway temp name, so
// frontmatter/plugin.json are strongly preferred there.
func skillNameForDir(dir, root string) string {
	if s, err := loadFile(filepath.Join(dir, "SKILL.md")); err == nil {
		if n := strings.TrimSpace(s.Name); n != "" && !strings.EqualFold(n, "SKILL") {
			return n
		}
	}
	if pj := readPluginJSON(dir); pj != nil && strings.TrimSpace(pj.Name) != "" {
		return strings.TrimSpace(pj.Name)
	}
	if dir == root {
		return "" // no reliable name for a root SKILL.md; skip rather than use temp dir name
	}
	return filepath.Base(dir)
}

// copySkillDir recursively copies src to dst, excluding .git and skipping
// symlinks (following a link inside an untrusted clone could copy arbitrary
// host files into the managed skills dir).
func copySkillDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil // never follow links out of the skill directory
		}
		if info.IsDir() {
			if info.Name() == ".git" && rel != "." {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyFile(path, filepath.Join(dst, rel), info)
	})
}

func copyFile(src, dst string, info os.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src) //nolint:gosec // src is inside a controlled clone dir
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()) //nolint:gosec // dst under managed dir
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// fetchManifestHTTP GETs an agents-standard marketplace manifest from an API URL,
// guarding against SSRF and capping the response size.
func fetchManifestHTTP(ctx context.Context, rawURL string) (*Marketplace, error) {
	if err := remoteGuard(ctx, rawURL); err != nil {
		return nil, fmt.Errorf("url not allowed: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "foxxycode-agent-skills")
	// Re-run the SSRF guard on every redirect target so a public URL cannot
	// bounce the request to localhost / private infrastructure.
	client := remoteClient(30*time.Second, remoteGuard)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("marketplace fetch %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		return nil, err
	}
	var mf Marketplace
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("parse marketplace json: %w", err)
	}
	return &mf, nil
}

// ---- lockfile ----

func remoteLockPath(managedDir string) string { return filepath.Join(managedDir, remoteLockFile) }

// readRemoteLock loads the provenance sidecar; a missing/invalid file yields an empty map.
func readRemoteLock(managedDir string) map[string]RemoteEntry {
	out := map[string]RemoteEntry{}
	data, err := os.ReadFile(remoteLockPath(managedDir))
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func writeRemoteLock(managedDir string, lock map[string]RemoteEntry) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	// In one rename: a write cut short must not leave a lock that reads as
	// "no remote skills".
	return writeFileAtomic(managedDir, remoteLockFile, data)
}

// RemoteSources returns the set of skill names installed from a remote source,
// keyed by canonical skill name. Used by callers to badge remote skills.
func RemoteSources(cfg *config.Config) map[string]RemoteEntry {
	return readRemoteLock(cfg.Skills.ManagedDir(cfg.Paths.Home))
}

// ---- config mutation ----

// AddSource appends a source to skills.sources and persists config.yaml.
// It reports whether the source was newly added.
func AddSource(cfg *config.Config, source string) (bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return false, fmt.Errorf("empty source")
	}
	if _, err := parseSource(source); err != nil {
		return false, err
	}
	if IsSystemSource(source) {
		// Already in effect, and writing it into the file would only create a
		// duplicate the operator could then delete from half of it.
		return false, nil
	}
	sourceMu.Lock()
	defer sourceMu.Unlock()
	return applySourceChange(cfg, func(current []string) ([]string, bool, error) {
		for _, s := range current {
			if strings.EqualFold(strings.TrimSpace(s), source) {
				return current, false, nil
			}
		}
		return append(append([]string(nil), current...), source), true, nil
	})
}

// applySourceChange reloads the on-disk config so a stale in-memory *Config
// cannot clobber unrelated settings, lets mutate compute the new source list
// from the current on-disk sources, persists only when it changed, and mirrors
// the result back into cfg. Callers hold sourceMu.
func applySourceChange(cfg *config.Config, mutate func(current []string) (next []string, changed bool, err error)) (bool, error) {
	changed := false
	// The read-mutate-persist cycle runs under the process-wide config file
	// lock shared with the staged config_commit / config_rollback transactions
	// and the HTTP PUT handler, so it can neither interleave with them nor
	// persist a base that another writer replaced mid-cycle.
	err := config.WithConfigFileLock(func() error {
		fresh := cfg
		if strings.TrimSpace(cfg.Paths.ConfigPath) != "" {
			reloaded, err := config.LoadWithPaths(cfg.Paths)
			switch {
			case err == nil && reloaded != nil:
				fresh = reloaded
			case errors.Is(err, os.ErrNotExist):
				// No config file on disk yet — it will be created from cfg below.
			default:
				// A real read/parse error: fail loudly rather than persist the
				// (possibly stale) caller config over whatever is on disk.
				return fmt.Errorf("reload config before source change: %w", err)
			}
		}
		next, mutated, err := mutate(fresh.Skills.Sources)
		if err != nil {
			return err
		}
		if !mutated {
			cfg.Skills.Sources = append([]string(nil), fresh.Skills.Sources...)
			return nil
		}
		fresh.Skills.Sources = next
		if err := persistConfig(fresh); err != nil {
			return err
		}
		cfg.Skills.Sources = append([]string(nil), next...)
		changed = true
		return nil
	})
	return changed, err
}

// RemoveRemote deletes an installed remote skill directory and its lock entry.
func RemoveRemote(cfg *config.Config, skillName string) error {
	name, err := sanitizeSkillName(skillName)
	if err != nil {
		return err
	}
	// Serialize with Sync/UpdateSkill so removal cannot race a materialization
	// (deleting a dir mid-swap) or lose a concurrent .remote.json update.
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	lock := readRemoteLock(managedDir)
	if _, ok := lock[name]; !ok {
		return fmt.Errorf("skill %q is not a remote (synced) skill", name)
	}
	if err := os.RemoveAll(filepath.Join(managedDir, name)); err != nil {
		return fmt.Errorf("remove skill dir: %w", err)
	}
	delete(lock, name)
	return writeRemoteLock(managedDir, lock)
}

// ---- version display ----

// InstalledVersion returns the version to display for a loaded skill: the
// version recorded in the remote lockfile when the skill was synced, else the
// skill's own SKILL.md frontmatter version. Empty when neither is known.
func InstalledVersion(remote map[string]RemoteEntry, name string, sk *Skill) string {
	if ent, ok := remote[name]; ok && strings.TrimSpace(ent.Version) != "" {
		return ent.Version
	}
	if sk != nil {
		return strings.TrimSpace(sk.Version)
	}
	return ""
}

// ---- update detection ----

// UpdateStatus reports whether a newer version of an installed remote skill is
// available in its marketplace source.
type UpdateStatus struct {
	Name            string `json:"name"`
	Source          string `json:"source"`
	Version         string `json:"version"` // installed
	Latest          string `json:"latest"`  // latest declared upstream
	UpdateAvailable bool   `json:"update_available"`
}

// CheckUpdates fetches the manifest for every remote source and reports, per
// installed remote skill, whether a newer version is available. It performs
// network / git access but never modifies installed skills. Sources that cannot
// be reached are treated as "no update" rather than failing the whole check.
func CheckUpdates(ctx context.Context, cfg *config.Config) ([]UpdateStatus, error) {
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	lock := readRemoteLock(managedDir)
	if len(lock) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(lock))
	for n := range lock {
		names = append(names, n)
	}
	sort.Strings(names)

	cache := map[string]map[string]string{} // source -> (plugin/skill name -> version)
	out := make([]UpdateStatus, 0, len(names))
	for _, name := range names {
		ent := lock[name]
		st := UpdateStatus{Name: name, Source: ent.Source, Version: ent.Version, Latest: ent.Version}
		versions, ok := cache[ent.Source]
		if !ok {
			versions, _ = sourceManifestVersions(ctx, ent.Source) // best-effort
			cache[ent.Source] = versions
		}
		key := ent.Plugin
		if key == "" {
			key = name
		}
		if latest := strings.TrimSpace(versions[key]); latest != "" {
			st.Latest = latest
			st.UpdateAvailable = isUpdate(latest, ent.Version)
		}
		out = append(out, st)
	}
	return out, nil
}

// UpdateSkill re-syncs the source that provides skillName, installing whatever
// version that source currently declares. Fails if the skill was not installed
// from a remote source.
func UpdateSkill(ctx context.Context, cfg *config.Config, skillName string) (*SyncResult, error) {
	name, err := sanitizeSkillName(skillName)
	if err != nil {
		return nil, err
	}
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	lock := readRemoteLock(managedDir)
	ent, ok := lock[name]
	if !ok {
		return nil, fmt.Errorf("skill %q is not a remote (synced) skill", name)
	}
	res := &SyncResult{}
	if strings.TrimSpace(ent.Plugin) != "" && !isWholeSource(cfg, ent.Source) {
		// From an added marketplace (or a source no longer configured): update
		// that plugin, never every plugin its marketplace lists.
		if err := updatePluginLocked(ctx, ent, managedDir, lock, res); err != nil {
			res.Failed = append(res.Failed, SyncFailure{Source: ent.Source, Error: err.Error()})
		}
	} else if err := syncOne(ctx, ent.Source, managedDir, lock, res); err != nil {
		res.Failed = append(res.Failed, SyncFailure{Source: ent.Source, Error: err.Error()})
	}
	if err := writeRemoteLock(managedDir, lock); err != nil {
		return res, fmt.Errorf("write lock: %w", err)
	}
	return res, nil
}

// AvailablePlugin is one installable plugin advertised by a configured
// marketplace, for the "install skills" browse/filter UI.
type AvailablePlugin struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version,omitempty"`
	Source      string `json:"source"`    // the configured source it comes from
	Installed   bool   `json:"installed"` // already present on disk
}

// AvailablePlugins fetches the manifest of every configured source and every
// added marketplace (network / git) and returns the plugins they advertise,
// flagged with whether each is already installed. Sources that cannot be
// reached are skipped best-effort.
func AvailablePlugins(ctx context.Context, cfg *config.Config, cwd string) ([]AvailablePlugin, error) {
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	installed := map[string]bool{}
	loader := NewLoader(cfg.Skills.Dirs)
	if loaded, err := loader.LoadAll(cwd, cfg.Paths.Home, cfg.Skills.ManagedDir(cfg.Paths.Home)); err == nil {
		for _, sk := range loaded {
			installed[CanonicalCommandName(sk)] = true
		}
	}
	seen := map[string]bool{}
	out := []AvailablePlugin{}
	srcs := ListSources(cfg)
	if added, err := AddedMarketplaces(cfg); err == nil {
		for _, m := range added {
			if !isWholeSource(cfg, m.Source) {
				srcs = append(srcs, m.Source)
			}
		}
	}
	for _, src := range srcs {
		mf, err := fetchSourceManifest(ctx, src)
		if err != nil || mf == nil {
			continue
		}
		for _, p := range mf.Plugins {
			name := strings.TrimSpace(p.Name)
			if name == "" {
				continue
			}
			key := src + "\x00" + name
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, AvailablePlugin{
				Name:        name,
				Description: strings.TrimSpace(p.Description),
				Version:     strings.TrimSpace(p.Version),
				Source:      src,
				Installed:   installed[name],
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// InstallPlugin installs a single plugin from a marketplace source (rather than
// syncing every plugin the source advertises). Backs the browse/filter install UI.
func InstallPlugin(ctx context.Context, cfg *config.Config, source, pluginName string) (*SyncResult, error) {
	source = strings.TrimSpace(source)
	pluginName = strings.TrimSpace(pluginName)
	if source == "" || pluginName == "" {
		return nil, fmt.Errorf("install requires a source and a plugin name")
	}
	if _, err := parseSource(source); err != nil {
		return nil, err
	}
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create managed dir: %w", err)
	}
	om, err := openMarketplace(ctx, source)
	if errors.Is(err, errNoMarketplace) {
		return nil, fmt.Errorf("source %q has no marketplace.json to install a named plugin from", source)
	}
	if err != nil {
		return nil, err
	}
	defer om.close()
	target := om.mf.plugin(pluginName)
	if target == nil {
		return nil, fmt.Errorf("plugin %q not found in %s", pluginName, source)
	}
	return installOne(ctx, om, *target, managedDir)
}

// installOne installs one plugin of an opened marketplace and records it in
// the lock. Callers hold syncMu.
func installOne(ctx context.Context, om *openedMarketplace, p MarketplacePlugin, managedDir string) (*SyncResult, error) {
	lock := readRemoteLock(managedDir)
	res := &SyncResult{}
	if err := installPlugin(ctx, p, om.repoRoot, om.base.Source, om.base, managedDir, lock, res); err != nil {
		res.Failed = append(res.Failed, SyncFailure{Source: om.base.Source, Error: err.Error()})
	}
	if err := writeRemoteLock(managedDir, lock); err != nil {
		return res, fmt.Errorf("write lock: %w", err)
	}
	return res, nil
}

// errNoMarketplace is a source that publishes no marketplace.json.
var errNoMarketplace = errors.New("no marketplace.json")

// openedMarketplace is a marketplace read from its source: the manifest, the
// lock entry its plugins start from, and for a git source the clone that
// relative plugin paths resolve against, which close removes.
type openedMarketplace struct {
	mf       *Marketplace
	repoRoot string
	base     RemoteEntry
	close    func()
}

// openMarketplace reads the marketplace a source publishes: over http for a
// marketplace.json URL, from a shallow clone for a git source. A source with no
// marketplace.json is errNoMarketplace.
func openMarketplace(ctx context.Context, source string) (*openedMarketplace, error) {
	spec, err := parseSource(source)
	if err != nil {
		return nil, err
	}
	om := &openedMarketplace{base: RemoteEntry{Source: source}, close: func() {}}
	switch spec.kind {
	case "api":
		mf, err := fetchManifestHTTP(ctx, spec.url)
		if err != nil {
			return nil, err
		}
		om.mf = mf
		om.base.URL = spec.url
		return om, nil
	case "git":
		tmp, err := os.MkdirTemp("", "foxxycode-marketplace-")
		if err != nil {
			return nil, err
		}
		cleanup := func() { _ = os.RemoveAll(tmp) }
		clone := filepath.Join(tmp, "repo")
		if err := safeClone(spec.url, spec.ref, clone); err != nil {
			cleanup()
			return nil, fmt.Errorf("clone %s: %w", spec.url, err)
		}
		mfPath := findMarketplaceFile(clone)
		if mfPath == "" {
			cleanup()
			return nil, fmt.Errorf("%w in %s", errNoMarketplace, source)
		}
		mf, err := parseMarketplace(mfPath)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("parse manifest: %w", err)
		}
		om.mf, om.repoRoot, om.close = mf, clone, cleanup
		om.base.Repo, om.base.Ref = spec.url, spec.ref
		return om, nil
	default:
		return nil, fmt.Errorf("unsupported source kind %q", spec.kind)
	}
}

// plugin returns the entry named name, compared without case, or nil.
func (m *Marketplace) plugin(name string) *MarketplacePlugin {
	name = strings.TrimSpace(name)
	for i := range m.Plugins {
		if strings.EqualFold(strings.TrimSpace(m.Plugins[i].Name), name) {
			return &m.Plugins[i]
		}
	}
	return nil
}

// fetchSourceManifest fetches a source's agents-standard marketplace manifest
// (HTTP for API sources, a shallow clone for git sources). Returns an error when
// the source has no manifest.
func fetchSourceManifest(ctx context.Context, source string) (*Marketplace, error) {
	om, err := openMarketplace(ctx, source)
	if err != nil {
		return nil, err
	}
	om.close()
	return om.mf, nil
}

// sourceManifestVersions fetches a source's marketplace manifest and returns a
// pluginName -> version map. For a plain repo with no manifest it maps each
// discovered skill's frontmatter version by skill name instead.
func sourceManifestVersions(ctx context.Context, source string) (map[string]string, error) {
	spec, err := parseSource(source)
	if err != nil {
		return nil, err
	}
	switch spec.kind {
	case "api":
		mf, err := fetchManifestHTTP(ctx, spec.url)
		if err != nil {
			return nil, err
		}
		return marketplaceVersions(mf), nil

	case "git":
		tmp, err := os.MkdirTemp("", "foxxycode-skillcheck-")
		if err != nil {
			return nil, err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		clone := filepath.Join(tmp, "repo")
		if err := safeClone(spec.url, spec.ref, clone); err != nil {
			return nil, err
		}
		if mfPath := findMarketplaceFile(clone); mfPath != "" {
			mf, err := parseMarketplace(mfPath)
			if err != nil {
				return nil, err
			}
			return marketplaceVersions(mf), nil
		}
		out := map[string]string{}
		for _, h := range locateSkillDirs(clone) {
			if v := skillDirVersion(h.dir); v != "" {
				out[h.name] = v
			}
		}
		return out, nil

	default:
		return map[string]string{}, nil
	}
}

// marketplaceVersions maps each plugin name to the version it advertises (see
// advertisedVersion); entries that advertise none are omitted, so update
// detection has no false positives.
func marketplaceVersions(mf *Marketplace) map[string]string {
	out := map[string]string{}
	for _, p := range mf.Plugins {
		if v := advertisedVersion(p); v != "" {
			out[strings.TrimSpace(p.Name)] = v
		}
	}
	return out
}

// advertisedVersion is the version a marketplace entry stands for: its
// declared version, else, for an archive plugin, the digest version of its
// declared sha256 - what installing it records. "" when it has neither.
func advertisedVersion(p MarketplacePlugin) string {
	if v := strings.TrimSpace(p.Version); v != "" {
		return v
	}
	if p.Source.Kind == "archive" {
		if sum, err := normalizeSHA256(p.Source.SHA256); err == nil && sum != "" {
			return archiveVersion(sum)
		}
	}
	return ""
}

// isUpdate reports whether the version a marketplace advertises is an update
// over the installed one. A digest version has no order, so any change of it
// is an update, and so is a switch between a digest and a declared version;
// declared versions compare by semantic versioning.
func isUpdate(latest, installed string) bool {
	if isArchiveVersion(latest) || isArchiveVersion(installed) {
		return latest != installed
	}
	return compareVersions(latest, installed) > 0
}

// compareVersions returns -1, 0, or 1 comparing two versions using semantic
// versioning precedence: numeric core fields compare left to right, and when
// cores are equal a normal release outranks a prerelease (§11). An optional
// leading "v" and any +build metadata are ignored. When either side has a
// non-numeric core it falls back to a lexical comparison.
func compareVersions(a, b string) int {
	ca, pa, oka := parseSemver(a)
	cb, pb, okb := parseSemver(b)
	if oka && okb {
		if c := compareCore(ca, cb); c != 0 {
			return c
		}
		return comparePrerelease(pa, pb)
	}
	sa := strings.TrimPrefix(strings.TrimSpace(a), "v")
	sb := strings.TrimPrefix(strings.TrimSpace(b), "v")
	return strings.Compare(sa, sb)
}

// parseSemver splits "v1.2.3-rc.1+build" into the numeric core [1 2 3] and the
// prerelease string ("rc.1"); ok is false when the core is not all-integer.
func parseSemver(v string) (core []int, prerelease string, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil, "", false
	}
	if i := strings.IndexByte(v, '+'); i >= 0 { // drop build metadata
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 { // split off prerelease
		prerelease = v[i+1:]
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	core = make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, "", false
		}
		core = append(core, n)
	}
	return core, prerelease, true
}

// compareCore compares dotted numeric fields, treating a missing field as 0.
func compareCore(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// comparePrerelease implements semver §11 precedence for the prerelease part:
// an empty prerelease (a release) outranks any prerelease; otherwise dot-
// separated identifiers compare field by field (numeric < alphanumeric, numeric
// numerically, alphanumeric lexically), and a shorter set of fields is lower.
func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		if i >= len(as) {
			return -1
		}
		if i >= len(bs) {
			return 1
		}
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil:
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
		case aerr == nil: // numeric identifiers are lower than alphanumeric
			return -1
		case berr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return 0
}

// ---- source management ----

// SystemSources are the marketplaces FoxxyCode is born with. They are listed and
// synced exactly like configured ones, but they live here rather than in
// skills.sources, so no surface can remove one and no config file has to be
// written to have it. Tests replace this to keep their assertions off the
// network; nothing else writes to it.
var SystemSources = []string{config.SystemSkillsSource}

// IsSystemSource reports whether source is one FoxxyCode brings itself, which is
// what makes it undeletable.
func IsSystemSource(source string) bool {
	source = strings.TrimSpace(source)
	for _, sys := range SystemSources {
		if strings.EqualFold(strings.TrimSpace(sys), source) {
			return true
		}
	}
	return false
}

// configured reports whether sources already names source.
func configured(sources []string, source string) bool {
	for _, s := range sources {
		if strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(source)) {
			return true
		}
	}
	return false
}

// ListSources returns every remote skill source in effect: the system ones
// first, then what skills.sources names (trimmed, non-empty, deduplicated - a
// config that repeats a system source does not make it appear twice).
func ListSources(cfg *config.Config) []string {
	out := make([]string, 0, len(SystemSources)+len(cfg.Skills.Sources))
	seen := make(map[string]struct{}, cap(out))
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			key := strings.ToLower(s)
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				out = append(out, s)
			}
		}
	}
	for _, s := range SystemSources {
		add(s)
	}
	for _, s := range cfg.Skills.Sources {
		add(s)
	}
	return out
}

// RemoveSource drops a source from skills.sources and persists config.yaml.
// It reports whether a matching source was found and removed. A system source
// is refused: it is not in the file, so there is nothing to take out of it.
func RemoveSource(cfg *config.Config, source string) (bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return false, fmt.Errorf("empty source")
	}
	if IsSystemSource(source) {
		// A config that also names it is carrying a redundant entry - written
		// before the source became a system one, or by hand. That entry can go;
		// the system source itself stays, and saying so is the whole answer.
		if !configured(cfg.Skills.Sources, source) {
			return false, fmt.Errorf("%s is built into FoxxyCode and cannot be removed; disable the skills you do not want with `foxxycode skills disable <name>`", source)
		}
		sourceMu.Lock()
		defer sourceMu.Unlock()
		if _, err := removeConfiguredSource(cfg, source); err != nil {
			return false, err
		}
		return false, fmt.Errorf("removed the redundant %s from skills.sources; the marketplace itself is built into FoxxyCode and stays in effect", source)
	}
	sourceMu.Lock()
	defer sourceMu.Unlock()
	return removeConfiguredSource(cfg, source)
}

// removeConfiguredSource drops source from skills.sources. Callers hold sourceMu.
func removeConfiguredSource(cfg *config.Config, source string) (bool, error) {
	return applySourceChange(cfg, func(current []string) ([]string, bool, error) {
		kept := make([]string, 0, len(current))
		removed := false
		for _, s := range current {
			if sameSource(s, source) {
				removed = true
				continue
			}
			kept = append(kept, s)
		}
		return kept, removed, nil
	})
}

func persistConfig(cfg *config.Config) error {
	path := cfg.Paths.ConfigPath
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("config path is empty")
	}
	data, err := config.MarshalConfigYAMLForFile(cfg, path)
	if err != nil {
		return err
	}
	if err := config.BackupCurrent(path); err != nil {
		return err
	}
	return config.AtomicWriteConfigYAML(path, data)
}
