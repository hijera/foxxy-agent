package skills

// Marketplaces added with `plugin marketplace add`, kept the way Claude Code
// keeps them. An added marketplace is known by the name its marketplace.json
// gives and by the plugins it listed when last read, and adding it installs
// nothing. `plugin install <plugin>@<marketplace>` installs one of its plugins
// and reads the marketplace again first, so a plugin it published after it was
// added is found without an update. Updating it refreshes that list and the
// plugins installed from it, and never installs the others.
//
// A source in skills.sources - the system one, one written by
// `plugin install <owner/repo | url>`, one an operator put in the file - keeps
// the older contract: every plugin it publishes is installed and kept in sync.
//
// The list is state, not configuration: it lives next to the lockfile in the
// managed skills dir.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// marketplacesFile holds the added marketplaces in the managed skills dir.
const marketplacesFile = ".marketplaces.json"

// AddedMarketplace is a marketplace added with `plugin marketplace add`.
type AddedMarketplace struct {
	Name      string         `json:"name"`                 // the name its marketplace.json gives
	Source    string         `json:"source"`               // owner/repo, a git URL or a marketplace.json URL
	Plugins   []ListedPlugin `json:"plugins"`              // what it listed when last read
	UpdatedAt string         `json:"updated_at,omitempty"` // when it was last read, RFC 3339
}

// ListedPlugin is one plugin an added marketplace lists.
type ListedPlugin struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
}

// errMarketplaceNotAdded is a name no added marketplace carries.
var errMarketplaceNotAdded = errors.New("marketplace is not added")

// marketplaceNamePattern is what a marketplace name, and the plugin name in
// <plugin>@<marketplace>, may look like. With no "/", ":" or "@" in it the form
// never reads as owner/repo@ref or git@host:path.
var marketplaceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ParsePluginRef splits <plugin>@<marketplace>. ok is false for anything
// else, a source (owner/repo[@ref], a git URL, a marketplace URL) included.
func ParsePluginRef(ref string) (plugin, market string, ok bool) {
	plugin, market, found := strings.Cut(strings.TrimSpace(ref), "@")
	if !found || !marketplaceNamePattern.MatchString(plugin) || !marketplaceNamePattern.MatchString(market) {
		return "", "", false
	}
	return plugin, market, true
}

// AddedMarketplaces returns the added marketplaces, by name.
func AddedMarketplaces(cfg *config.Config) ([]AddedMarketplace, error) {
	return readMarketplaces(cfg.Skills.ManagedDir(cfg.Paths.Home))
}

// AddMarketplace reads the marketplace source publishes and adds it, or, when
// the marketplace is added already, refreshes its list; refreshed says which.
// It installs nothing. A source without a marketplace.json, or whose
// marketplace.json gives no usable name, cannot be added.
func AddMarketplace(ctx context.Context, cfg *config.Config, source string) (AddedMarketplace, bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return AddedMarketplace{}, false, fmt.Errorf("empty source")
	}
	if _, err := parseSource(source); err != nil {
		return AddedMarketplace{}, false, err
	}
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	ms, err := readMarketplaces(managedDir)
	if err != nil {
		return AddedMarketplace{}, false, err
	}
	om, err := openMarketplace(ctx, source)
	if errors.Is(err, errNoMarketplace) {
		return AddedMarketplace{}, false, fmt.Errorf("%s publishes no marketplace.json, so there is no marketplace to add; install its skills with `plugin install %s`", source, source)
	}
	if err != nil {
		return AddedMarketplace{}, false, err
	}
	defer om.close()
	name := strings.TrimSpace(om.mf.Name)
	if !marketplaceNamePattern.MatchString(name) {
		return AddedMarketplace{}, false, fmt.Errorf("the marketplace.json of %s names it %q, which `plugin install <plugin>@<marketplace>` cannot use; install every plugin it lists with `plugin install %s`", source, name, source)
	}
	added := AddedMarketplace{Name: name, Source: source, Plugins: listingOf(om.mf), UpdatedAt: nowStamp()}
	refreshed := false
	if i := indexAdded(ms, name, false); i >= 0 {
		if !sameSource(ms[i].Source, source) {
			return AddedMarketplace{}, false, fmt.Errorf("a marketplace named %q is added already, from %s; remove it with `plugin marketplace remove %s` to add this one", name, ms[i].Source, name)
		}
		added.Source = ms[i].Source
		ms[i] = added
		refreshed = true
	} else if j := indexAdded(ms, source, true); j >= 0 {
		// The marketplace changed its name since it was added.
		ms[j] = added
		refreshed = true
	} else {
		ms = append(ms, added)
	}
	if err := writeMarketplaces(managedDir, ms); err != nil {
		return AddedMarketplace{}, false, err
	}
	return added, refreshed, nil
}

// InstallFromMarketplace installs the plugin named plugin from the added
// marketplace named market. It reads the marketplace again first, so a plugin
// published after the marketplace was added is found, and refreshes its list.
func InstallFromMarketplace(ctx context.Context, cfg *config.Config, plugin, market string) (*SyncResult, error) {
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create managed dir: %w", err)
	}
	ms, err := readMarketplaces(managedDir)
	if err != nil {
		return nil, err
	}
	i := indexAdded(ms, market, false)
	if i < 0 {
		return nil, notAddedError(market, plugin, ms)
	}
	m := &ms[i]
	om, err := openMarketplace(ctx, m.Source)
	if err != nil {
		return nil, fmt.Errorf("read marketplace %q from %s: %w", m.Name, m.Source, err)
	}
	defer om.close()
	m.Plugins, m.UpdatedAt = listingOf(om.mf), nowStamp()
	if err := writeMarketplaces(managedDir, ms); err != nil {
		return nil, err
	}
	target := om.mf.plugin(plugin)
	if target == nil {
		return nil, fmt.Errorf("plugin %q is not in marketplace %q, which lists %d plugin(s) as read just now; `plugin marketplace list %s` names them", plugin, m.Name, len(m.Plugins), m.Name)
	}
	return installOne(ctx, om, *target, managedDir)
}

// UpdateMarketplace reads the added marketplace key names (by name or source)
// again, refreshes its list and reinstalls the plugins installed from it. The
// plugins it lists that are not installed stay so. A key no added marketplace
// answers to is errMarketplaceNotAdded.
func UpdateMarketplace(ctx context.Context, cfg *config.Config, key string) (AddedMarketplace, *SyncResult, error) {
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	ms, err := readMarketplaces(managedDir)
	if err != nil {
		return AddedMarketplace{}, nil, err
	}
	i := indexAdded(ms, key, false)
	if i < 0 {
		i = indexAdded(ms, key, true)
	}
	if i < 0 {
		return AddedMarketplace{}, nil, fmt.Errorf("%w: %q", errMarketplaceNotAdded, key)
	}
	lock := readRemoteLock(managedDir)
	res := &SyncResult{}
	if err := updateAddedLocked(ctx, &ms[i], managedDir, lock, res); err != nil {
		return ms[i], res, err
	}
	if err := writeRemoteLock(managedDir, lock); err != nil {
		return ms[i], res, fmt.Errorf("write lock: %w", err)
	}
	if err := writeMarketplaces(managedDir, ms); err != nil {
		return ms[i], res, err
	}
	return ms[i], res, nil
}

// RemoveMarketplace drops the added marketplace key names (by name or source)
// and reports which it was, if any. The skills installed from it stay until
// they are removed.
func RemoveMarketplace(cfg *config.Config, key string) (AddedMarketplace, bool, error) {
	syncMu.Lock()
	defer syncMu.Unlock()
	managedDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	ms, err := readMarketplaces(managedDir)
	if err != nil {
		return AddedMarketplace{}, false, err
	}
	i := indexAdded(ms, key, false)
	if i < 0 {
		i = indexAdded(ms, key, true)
	}
	if i < 0 {
		return AddedMarketplace{}, false, nil
	}
	removed := ms[i]
	ms = append(ms[:i], ms[i+1:]...)
	return removed, true, writeMarketplaces(managedDir, ms)
}

// updateAddedLocked reads m again, refreshes its list and reinstalls the
// plugins the lock records as installed from it. Callers hold syncMu, and write
// the lock and the list afterwards.
func updateAddedLocked(ctx context.Context, m *AddedMarketplace, managedDir string, lock map[string]RemoteEntry, res *SyncResult) error {
	om, err := openMarketplace(ctx, m.Source)
	if err != nil {
		return fmt.Errorf("read marketplace %q from %s: %w", m.Name, m.Source, err)
	}
	defer om.close()
	m.Plugins, m.UpdatedAt = listingOf(om.mf), nowStamp()
	for _, name := range installedPlugins(lock, m.Source) {
		p := om.mf.plugin(name)
		if p == nil {
			res.Failed = append(res.Failed, SyncFailure{Source: m.Name, Error: fmt.Sprintf("plugin %q is no longer in the marketplace; its skills stay installed", name)})
			continue
		}
		if err := installPlugin(ctx, *p, om.repoRoot, m.Source, om.base, managedDir, lock, res); err != nil {
			res.Failed = append(res.Failed, SyncFailure{Source: m.Name, Error: err.Error()})
		}
	}
	return nil
}

// updatePluginLocked reinstalls the one plugin ent was installed from, reading
// its marketplace again. Callers hold syncMu and write the lock afterwards.
func updatePluginLocked(ctx context.Context, ent RemoteEntry, managedDir string, lock map[string]RemoteEntry, res *SyncResult) error {
	om, err := openMarketplace(ctx, ent.Source)
	if err != nil {
		return err
	}
	defer om.close()
	p := om.mf.plugin(ent.Plugin)
	if p == nil {
		return fmt.Errorf("plugin %q is no longer in %s; its skills stay installed", ent.Plugin, ent.Source)
	}
	return installPlugin(ctx, *p, om.repoRoot, ent.Source, om.base, managedDir, lock, res)
}

// syncAddedLocked refreshes every added marketplace and the plugins installed
// from it, as Sync does after the sources. One that is also a source was
// synced whole already and is skipped. Callers hold syncMu.
func syncAddedLocked(ctx context.Context, cfg *config.Config, managedDir string, lock map[string]RemoteEntry, res *SyncResult) {
	ms, err := readMarketplaces(managedDir)
	if err != nil {
		res.Failed = append(res.Failed, SyncFailure{Source: marketplacesFile, Error: err.Error()})
		return
	}
	if len(ms) == 0 {
		return
	}
	for i := range ms {
		if isWholeSource(cfg, ms[i].Source) {
			continue
		}
		if err := updateAddedLocked(ctx, &ms[i], managedDir, lock, res); err != nil {
			res.Failed = append(res.Failed, SyncFailure{Source: ms[i].Name, Error: err.Error()})
		}
	}
	if err := writeMarketplaces(managedDir, ms); err != nil {
		res.Failed = append(res.Failed, SyncFailure{Source: marketplacesFile, Error: err.Error()})
	}
}

// isWholeSource reports whether source is one whose every plugin is installed:
// a system source or one skills.sources names.
func isWholeSource(cfg *config.Config, source string) bool {
	for _, s := range ListSources(cfg) {
		if sameSource(s, source) {
			return true
		}
	}
	return false
}

// installedPlugins lists, sorted, the plugins the lock records as installed
// from source.
func installedPlugins(lock map[string]RemoteEntry, source string) []string {
	seen := map[string]bool{}
	var out []string
	for _, ent := range lock {
		name := strings.TrimSpace(ent.Plugin)
		if name == "" || !sameSource(ent.Source, source) || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func notAddedError(market, plugin string, ms []AddedMarketplace) error {
	msg := fmt.Sprintf("marketplace %q is not added: add it with `plugin marketplace add <owner/repo | marketplace.json URL>`, then run `plugin install %s@%s` again", market, plugin, market)
	if len(ms) > 0 {
		names := make([]string, 0, len(ms))
		for _, m := range ms {
			names = append(names, m.Name)
		}
		msg += fmt.Sprintf(" (added: %s)", strings.Join(names, ", "))
	}
	return errors.New(msg)
}

// indexAdded finds an added marketplace by name, or by source when bySource.
func indexAdded(ms []AddedMarketplace, key string, bySource bool) int {
	key = strings.TrimSpace(key)
	for i, m := range ms {
		if bySource && sameSource(m.Source, key) || !bySource && strings.EqualFold(m.Name, key) {
			return i
		}
	}
	return -1
}

// sameSource reports whether a and b name one source: the same string, or the
// same kind, clone or marketplace URL and ref once parsed, so owner/repo matches
// https://github.com/owner/repo and a trailing "/" or ".git" does not matter.
func sameSource(a, b string) bool {
	if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) {
		return true
	}
	sa, errA := parseSource(a)
	sb, errB := parseSource(b)
	if errA != nil || errB != nil {
		return false
	}
	norm := func(u string) string {
		return strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(u), "/"), ".git")
	}
	return sa.kind == sb.kind && strings.EqualFold(norm(sa.url), norm(sb.url)) && sa.ref == sb.ref
}

// UpdateSource refreshes what key names. A whole source (a system one, or one
// skills.sources names) syncs every plugin it publishes; an added marketplace,
// by name or source, refreshes its list and the plugins installed from it; any
// other source is synced whole once, as `plugin marketplace sync <source>`
// always did. added is the marketplace key named, when it is one.
func UpdateSource(ctx context.Context, cfg *config.Config, key string) (res *SyncResult, added *AddedMarketplace, err error) {
	key = strings.TrimSpace(key)
	target := key
	if ms, err := AddedMarketplaces(cfg); err == nil {
		if i := indexAdded(ms, key, false); i >= 0 {
			// A name stands for its source, so a marketplace that is a whole
			// source too syncs whole by its name as by its address.
			target = ms[i].Source
		}
	}
	if !isWholeSource(cfg, target) {
		m, res, err := UpdateMarketplace(ctx, cfg, key)
		switch {
		case err == nil:
			return res, &m, nil
		case !errors.Is(err, errMarketplaceNotAdded):
			return res, nil, err
		}
		if _, perr := parseSource(key); perr != nil {
			return nil, nil, fmt.Errorf("marketplace %q is not added; `plugin marketplace list` shows the added ones", key)
		}
	}
	res, err = SyncSource(ctx, cfg, target)
	return res, nil, err
}

func listingOf(mf *Marketplace) []ListedPlugin {
	out := make([]ListedPlugin, 0, len(mf.Plugins))
	for _, p := range mf.Plugins {
		if name := strings.TrimSpace(p.Name); name != "" {
			out = append(out, ListedPlugin{Name: name, Description: strings.TrimSpace(p.Description), Version: advertisedVersion(p)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339) }

// marketplacesDoc is the file layout of marketplacesFile.
type marketplacesDoc struct {
	Marketplaces []AddedMarketplace `json:"marketplaces"`
}

// readMarketplaces loads the added marketplaces; no file is none. A file that
// does not parse is an error rather than an empty list, so nothing writes over
// what it holds.
func readMarketplaces(managedDir string) ([]AddedMarketplace, error) {
	data, err := os.ReadFile(filepath.Join(managedDir, marketplacesFile)) //nolint:gosec // a fixed name in the managed dir
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc marketplacesDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Join(managedDir, marketplacesFile), err)
	}
	return doc.Marketplaces, nil
}

// writeMarketplaces saves the added marketplaces, sorted by name. The caller's
// slice keeps its order.
func writeMarketplaces(managedDir string, ms []AddedMarketplace) error {
	sorted := append([]AddedMarketplace{}, ms...)
	sort.Slice(sorted, func(i, j int) bool { return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name) })
	data, err := json.MarshalIndent(marketplacesDoc{Marketplaces: sorted}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(managedDir, marketplacesFile, append(data, '\n'))
}

// writeFileAtomic replaces dir/name with data in one rename, so a reader never
// sees a half-written file and a crash leaves the old one.
func writeFileAtomic(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, name+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
