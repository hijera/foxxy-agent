package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// SourceStatus describes whether a configured marketplace source resolves to a
// valid agents-standard marketplace, reported by `plugin marketplace list`.
type SourceStatus struct {
	Source   string `json:"source"`
	Kind     string `json:"kind"`              // "git" | "api"
	Valid    bool   `json:"valid"`             // usable as a skill source
	Standard string `json:"standard"`          // "marketplace" | "no-manifest" | "unreachable" | "invalid"
	Name     string `json:"name,omitempty"`    // marketplace name (agents standard)
	Version  string `json:"version,omitempty"` // marketplace metadata version
	Plugins  int    `json:"plugins"`           // plugin/skill count when known
	Error    string `json:"error,omitempty"`
}

// MarketplaceStatus probes every configured source (network / git access) and
// reports whether each is a valid agents-standard marketplace. Failures are
// captured per source rather than aborting the whole listing.
func MarketplaceStatus(ctx context.Context, cfg *config.Config) []SourceStatus {
	srcs := ListSources(cfg)
	out := make([]SourceStatus, 0, len(srcs))
	for _, src := range srcs {
		out = append(out, probeSource(ctx, src))
	}
	return out
}

func probeSource(ctx context.Context, src string) SourceStatus {
	st := SourceStatus{Source: src}
	spec, err := parseSource(src)
	if err != nil {
		st.Standard = "invalid"
		st.Error = err.Error()
		return st
	}
	st.Kind = spec.kind

	switch spec.kind {
	case "api":
		mf, err := fetchManifestHTTP(ctx, spec.url)
		if err != nil {
			st.Standard = "unreachable"
			st.Error = err.Error()
			return st
		}
		st.Valid = true
		st.Standard = "marketplace"
		st.Name = mf.Name
		st.Version = mf.Metadata.Version
		st.Plugins = len(mf.Plugins)
		return st

	case "git":
		tmp, err := os.MkdirTemp("", "foxxycode-mpstat-")
		if err != nil {
			st.Standard = "unreachable"
			st.Error = err.Error()
			return st
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		clone := filepath.Join(tmp, "repo")
		if err := safeClone(spec.url, spec.ref, clone); err != nil {
			st.Standard = "unreachable"
			st.Error = err.Error()
			return st
		}
		if mfPath := findMarketplaceFile(clone); mfPath != "" {
			mf, err := parseMarketplace(mfPath)
			if err != nil {
				st.Standard = "invalid"
				st.Error = err.Error()
				return st
			}
			st.Valid = true
			st.Standard = "marketplace"
			st.Name = mf.Name
			st.Version = mf.Metadata.Version
			st.Plugins = len(mf.Plugins)
			return st
		}
		hits := locateSkillDirs(clone)
		st.Standard = "no-manifest"
		st.Plugins = len(hits)
		st.Valid = len(hits) > 0
		return st

	default:
		st.Standard = "unknown"
		return st
	}
}

// RunPluginCommand dispatches a `plugin ...` invocation shared by the CLI
// (`foxxycode plugin ...`) and the chat `/plugin` command. args are the tokens
// after `plugin` / `/plugin`. It returns human-readable output; a non-nil error
// signals a usage or execution failure the caller surfaces to the user.
func RunPluginCommand(ctx context.Context, cfg *config.Config, cwd string, args []string) (string, error) {
	args = trimTokens(args)
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return pluginUsage(), nil
	}
	switch args[0] {
	case "marketplace", "mp":
		return runPluginMarketplace(ctx, cfg, args[1:])
	case "install", "add":
		if len(args) < 2 {
			return "", fmt.Errorf("usage: plugin install <plugin>@<marketplace> | <owner/repo | git-url | marketplace-url>")
		}
		if plugin, market, ok := ParsePluginRef(args[1]); ok {
			return pluginInstallFromMarketplace(ctx, cfg, plugin, market)
		}
		return pluginInstall(ctx, cfg, args[1])
	case "remove", "uninstall":
		if len(args) < 2 {
			return "", fmt.Errorf("usage: plugin remove <name>")
		}
		if err := DeleteSkill(cfg, cwd, args[1]); err != nil {
			return "", err
		}
		return fmt.Sprintf("Removed skill %q.", args[1]), nil
	case "enable":
		if len(args) < 2 {
			return "", fmt.Errorf("usage: plugin enable <name>")
		}
		if err := Enable(cfg, args[1]); err != nil {
			return "", err
		}
		return fmt.Sprintf("Enabled skill %q.", args[1]), nil
	case "disable":
		if len(args) < 2 {
			return "", fmt.Errorf("usage: plugin disable <name>")
		}
		if err := Disable(cfg, args[1]); err != nil {
			return "", err
		}
		return fmt.Sprintf("Disabled skill %q.", args[1]), nil
	case "list":
		// Convenience: `plugin list` = installed skills with versions.
		return pluginInstalledList(cfg, cwd), nil
	default:
		return "", fmt.Errorf("unknown plugin subcommand %q (try `plugin help`)", args[0])
	}
}

func runPluginMarketplace(ctx context.Context, cfg *config.Config, args []string) (string, error) {
	args = trimTokens(args)
	if len(args) == 0 {
		return "", fmt.Errorf("usage: plugin marketplace list|add|remove|update")
	}
	switch args[0] {
	case "list", "ls":
		if len(args) >= 2 {
			return pluginMarketplaceShow(cfg, args[1])
		}
		return pluginMarketplaceList(ctx, cfg), nil
	case "add":
		if len(args) < 2 {
			return "", fmt.Errorf("usage: plugin marketplace add <owner/repo | git-url | marketplace-url>")
		}
		return pluginMarketplaceAdd(ctx, cfg, args[1])
	case "remove", "rm":
		if len(args) < 2 {
			return "", fmt.Errorf("usage: plugin marketplace remove <marketplace | owner/repo | git-url | marketplace-url>")
		}
		return pluginMarketplaceRemove(cfg, args[1])
	case "sync", "update":
		// No argument: every source and added marketplace. With one: only it.
		if len(args) >= 2 {
			return pluginMarketplaceUpdate(ctx, cfg, args[1])
		}
		res, err := Sync(ctx, cfg)
		if err != nil {
			return "", err
		}
		return "Synced all marketplaces. " + formatSyncLine(res), nil
	default:
		return "", fmt.Errorf("unknown marketplace subcommand %q", args[0])
	}
}

// pluginMarketplaceAdd adds a marketplace: it reads the list of plugins the
// marketplace publishes and installs none of them. Adding it again refreshes
// that list.
func pluginMarketplaceAdd(ctx context.Context, cfg *config.Config, source string) (string, error) {
	m, refreshed, err := AddMarketplace(ctx, cfg, source)
	if err != nil {
		return "", err
	}
	how := fmt.Sprintf("Install one with `plugin install <plugin>@%s`; `plugin marketplace list %s` names them.", m.Name, m.Name)
	if isWholeSource(cfg, m.Source) {
		how = "It is one of FoxxyCode's sources as well, so every plugin it lists is installed and kept in sync."
	}
	if refreshed {
		return fmt.Sprintf("Marketplace %q is already added; its list is refreshed: %d plugin(s). %s", m.Name, len(m.Plugins), how), nil
	}
	return fmt.Sprintf("Added marketplace %q from %s: %d plugin(s). %s", m.Name, m.Source, len(m.Plugins), how), nil
}

// pluginInstallFromMarketplace installs one plugin of an added marketplace.
// Nothing installed is an error, so a script sees it in the exit status.
func pluginInstallFromMarketplace(ctx context.Context, cfg *config.Config, plugin, market string) (string, error) {
	res, err := InstallFromMarketplace(ctx, cfg, plugin, market)
	if err != nil {
		return "", err
	}
	if len(res.Added)+len(res.Updated) == 0 && len(res.Failed) > 0 {
		return "", fmt.Errorf("install %s@%s: %s", plugin, market, res.Failed[0].Error)
	}
	return fmt.Sprintf("Installed %s@%s. %s", plugin, market, formatSyncLine(res)), nil
}

// pluginMarketplaceUpdate refreshes one marketplace or source (UpdateSource).
func pluginMarketplaceUpdate(ctx context.Context, cfg *config.Config, key string) (string, error) {
	res, m, err := UpdateSource(ctx, cfg, key)
	if err != nil {
		return "", err
	}
	if m != nil {
		return fmt.Sprintf("Updated marketplace %q: %d plugin(s) listed. %s", m.Name, len(m.Plugins), formatSyncLine(res)), nil
	}
	return fmt.Sprintf("Synced marketplace %q. %s", key, formatSyncLine(res)), nil
}

// pluginMarketplaceRemove drops an added marketplace (by name or source) and
// a source in skills.sources, whichever key names.
func pluginMarketplaceRemove(cfg *config.Config, key string) (string, error) {
	m, wasAdded, err := RemoveMarketplace(cfg, key)
	if err != nil {
		return "", err
	}
	// Removed by name, the marketplace takes its source out of skills.sources
	// too, or the next sync would install every plugin of it again.
	sourceKey := key
	if wasAdded {
		sourceKey = m.Source
	}
	removedSource, srcErr := RemoveSource(cfg, sourceKey)
	switch {
	case wasAdded:
		msg := fmt.Sprintf("Removed marketplace %q. Installed skills remain until removed.", m.Name)
		if removedSource {
			msg = fmt.Sprintf("Removed marketplace %q and its source %s from skills.sources. Installed skills remain until removed.", m.Name, m.Source)
		}
		if srcErr != nil {
			// A system source: its registration is gone, the source stays.
			msg += " " + srcErr.Error() + "."
		}
		return msg, nil
	case srcErr != nil:
		return "", srcErr
	case removedSource:
		return fmt.Sprintf("Removed marketplace %q. Installed skills remain until removed.", key), nil
	default:
		return fmt.Sprintf("Marketplace %q was not configured.", key), nil
	}
}

// pluginMarketplaceShow lists the plugins of an added marketplace as it read
// them last.
func pluginMarketplaceShow(cfg *config.Config, key string) (string, error) {
	ms, err := AddedMarketplaces(cfg)
	if err != nil {
		return "", err
	}
	i := indexAdded(ms, key, false)
	if i < 0 {
		i = indexAdded(ms, key, true)
	}
	if i < 0 {
		return "", fmt.Errorf("marketplace %q is not added; `plugin marketplace list` shows the added ones", key)
	}
	m := ms[i]
	installed := map[string]bool{}
	for _, p := range installedPlugins(RemoteSources(cfg), m.Source) {
		installed[strings.ToLower(p)] = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Marketplace %q (%s), %d plugin(s) as read %s:\n", m.Name, m.Source, len(m.Plugins), m.UpdatedAt)
	for _, p := range m.Plugins {
		line := "  - " + p.Name
		if p.Version != "" {
			line += "@" + p.Version
		}
		if installed[strings.ToLower(p.Name)] {
			line += "  [installed]"
		}
		if p.Description != "" {
			line += "  " + firstLine(p.Description)
		}
		b.WriteString(line + "\n")
	}
	fmt.Fprintf(&b, "Install one with `plugin install <plugin>@%s`.", m.Name)
	return b.String(), nil
}

// pluginInstall adds the source when new and (re-)syncs it, so install also
// updates an already-installed source in one step. Only this source is synced:
// installing one marketplace neither refreshes the others nor fails on them.
func pluginInstall(ctx context.Context, cfg *config.Config, source string) (string, error) {
	added, err := AddSource(cfg, source)
	if err != nil {
		return "", err
	}
	res, err := SyncSource(ctx, cfg, source)
	if err != nil {
		return "", err
	}
	verb := "Updated"
	if added {
		verb = "Installed"
	}
	return fmt.Sprintf("%s %q. %s", verb, source, formatSyncLine(res)), nil
}

func pluginMarketplaceList(ctx context.Context, cfg *config.Config) string {
	statuses := MarketplaceStatus(ctx, cfg)
	added, addedErr := AddedMarketplaces(cfg)
	if len(statuses) == 0 && len(added) == 0 && addedErr == nil {
		return "No marketplaces configured. Add one with `plugin marketplace add <owner/repo | url>`."
	}
	var b strings.Builder
	if addedErr != nil {
		fmt.Fprintf(&b, "Added marketplaces could not be read: %s\n", firstLine(addedErr.Error()))
	}
	if len(added) > 0 {
		fmt.Fprintf(&b, "%d added marketplace(s), install a plugin with `plugin install <plugin>@<marketplace>`:\n", len(added))
		for _, m := range added {
			fmt.Fprintf(&b, "  - %s  [%s; %s]\n", m.Name, statusDetail(probeSource(ctx, m.Source)), m.Source)
		}
	}
	if len(statuses) > 0 {
		fmt.Fprintf(&b, "%d source(s), every plugin installed and kept in sync:\n", len(statuses))
		for _, st := range statuses {
			fmt.Fprintf(&b, "  - %s  [%s]\n", st.Source, statusDetail(st))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// statusDetail is the bracketed state of a marketplace in `marketplace list`.
func statusDetail(st SourceStatus) string {
	detail := st.Standard
	switch st.Standard {
	case "marketplace":
		name := st.Name
		if name == "" {
			name = "(unnamed)"
		}
		ver := ""
		if st.Version != "" {
			ver = " v" + st.Version
		}
		detail = fmt.Sprintf("valid marketplace — %s%s, %d plugin(s)", name, ver, st.Plugins)
	case "no-manifest":
		detail = fmt.Sprintf("no marketplace.json — %d skill(s) discovered directly", st.Plugins)
	case "unreachable":
		detail = "unreachable"
		if st.Error != "" {
			detail += " (" + firstLine(st.Error) + ")"
		}
	case "invalid":
		detail = "invalid source"
		if st.Error != "" {
			detail += " (" + firstLine(st.Error) + ")"
		}
	}
	return detail
}

func pluginInstalledList(cfg *config.Config, cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	loader := NewLoader(cfg.Skills.Dirs)
	loaded, err := loader.LoadAll(cwd, cfg.Paths.Home, cfg.Skills.ManagedDir(cfg.Paths.Home))
	if err != nil {
		return fmt.Sprintf("failed to load skills: %v", err)
	}
	remote := RemoteSources(cfg)
	byName := make(map[string]*Skill, len(loaded))
	for _, sk := range loaded {
		n := CanonicalCommandName(sk)
		if _, ok := byName[n]; !ok {
			byName[n] = sk
		}
	}
	sums := ListSkills(loaded)
	if len(sums) == 0 {
		return "No skills installed."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d skill(s):\n", len(sums))
	for _, sum := range sums {
		version := InstalledVersion(remote, sum.Name, byName[sum.Name])
		if version == "" {
			version = "-"
		}
		origin := ""
		if ent, ok := remote[sum.Name]; ok {
			origin = " (from " + ent.Source + ")"
		}
		fmt.Fprintf(&b, "  - %s@%s%s\n", sum.Name, version, origin)
	}
	return strings.TrimRight(b.String(), "\n")
}

func pluginUsage() string {
	return strings.Join([]string{
		"plugin commands:",
		"  plugin marketplace add <owner/repo|url>     add a marketplace and read its plugin list (installs nothing)",
		"  plugin marketplace list [<marketplace>]     list marketplaces and sources, or the plugins of one marketplace",
		"  plugin marketplace update [<marketplace>]   refresh marketplaces and what is installed from them (alias: sync)",
		"  plugin marketplace remove <marketplace>     remove a marketplace or a source",
		"  plugin install <plugin>@<marketplace>       install one plugin of an added marketplace",
		"  plugin install <owner/repo|url>             install every skill a source publishes and keep them in sync",
		"  plugin remove <name>                        remove an installed skill",
		"  plugin enable <name>                        enable a skill",
		"  plugin disable <name>                       disable a skill",
		"  plugin list                                 list installed skills with versions",
	}, "\n")
}

func formatSyncLine(res *SyncResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d added, %d updated, %d failed.", len(res.Added), len(res.Updated), len(res.Failed))
	for _, f := range res.Failed {
		fmt.Fprintf(&b, "\n  ! %s: %s", f.Source, firstLine(f.Error))
	}
	return b.String()
}

func trimTokens(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
