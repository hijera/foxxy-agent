//go:build memory

package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	memstorage "github.com/hijera/foxxycode-agent/external/memory/storage"
	memtools "github.com/hijera/foxxycode-agent/external/memory/tools"
	"github.com/hijera/foxxycode-agent/internal/config"
)

func extractLinkTargetsForChain(body string) []string {
	raw := regexp.MustCompile(`\b(global|project):([a-zA-Z0-9_./\-]+\.(?:md|txt))\b`)
	mdHref := regexp.MustCompile(`\[[^\]]*]\(((?:global|project):[a-zA-Z0-9_./\-]+\.(?:md|txt))\)`)
	var out []string
	seen := make(map[string]struct{})
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		if strings.Contains(s, "..") {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, sm := range raw.FindAllStringSubmatch(body, -1) {
		add(sm[1] + ":" + sm[2])
	}
	for _, sm := range mdHref.FindAllStringSubmatch(body, -1) {
		add(strings.Trim(sm[1], `"'`))
	}
	return out
}

func memoryTreeFixtureChain(t *testing.T, globalRoot string) {
	t.Helper()
	layout := []struct {
		relPath string
		body    string
	}{
		{
			"index.md",
			"# FoxxyCode memory hub\n\nStart here after recall search for hub navigation.\n\nSee [architecture index](global:docs/arch/overview.md).\nBare link global:guides/quickstart.txt for plain references.\n",
		},
		{
			"guides/quickstart.txt",
			"Quick checklist. Dive into global:topics/services/api-map.md.\n",
		},
		{
			"docs/arch/overview.md",
			"## Services overview\n\nHigh-level sketch. Related [API map](global:topics/services/api-map.md).\n",
		},
		{
			"topics/services/api-map.md",
			"Routes table. Secrets live in vault note: [vault](global:topics/secrets/vault.md).\n",
		},
		{
			"topics/secrets/vault.md",
			"## Vault naming\nANSWER_UNIQUE_TOKEN_XYZZY_42\nStored only here; indexing pages must not duplicate this literal.\n",
		},
	}
	for _, f := range layout {
		dir := filepath.Dir(f.relPath)
		if dir != "." {
			fullDir := filepath.Join(globalRoot, dir)
			if err := os.MkdirAll(fullDir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		p := filepath.Join(globalRoot, f.relPath)
		if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func chainMemoryConfig() *config.MemoryConfig {
	m := &config.MemoryConfig{}
	m.ApplyDefaults()
	return m
}

// TestSequentialSearchReadChain emulates alternating search and read hops an LLM could take.
func TestSequentialSearchReadChain(t *testing.T) {
	const secret = "ANSWER_UNIQUE_TOKEN_XYZZY_42"
	cfg := chainMemoryConfig()
	tmp := t.TempDir()
	g := filepath.Join(tmp, "g")
	p := filepath.Join(tmp, "p")
	if err := os.MkdirAll(g, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	memoryTreeFixtureChain(t, g)
	st := memstorage.NewWithRoots(g, p)

	toolHits, err := memtools.ExecTool(st, cfg, memtools.NameSearch, `{"query":"hub navigation foxxycode","scope":"global"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(toolHits, "global:index.md") {
		t.Fatalf("search tool: %s", toolHits)
	}
	idxBody, err := memtools.ExecTool(st, cfg, memtools.NameRead, `{"path":"global:index.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	pathsIdx := extractLinkTargetsForChain(idxBody)
	if len(pathsIdx) == 0 {
		t.Fatal("index should expose outbound links")
	}
	var overviewPath string
	for _, pth := range pathsIdx {
		if strings.Contains(pth, "overview.md") {
			overviewPath = pth
			break
		}
	}
	if overviewPath == "" {
		t.Fatalf("no overview in %v", pathsIdx)
	}
	ovPayload, err := json.Marshal(map[string]string{"path": overviewPath})
	if err != nil {
		t.Fatal(err)
	}
	ovBody, err := memtools.ExecTool(st, cfg, memtools.NameRead, string(ovPayload))
	if err != nil {
		t.Fatal(err)
	}
	var apiPath string
	for _, pth := range extractLinkTargetsForChain(ovBody) {
		if strings.Contains(pth, "api-map.md") {
			apiPath = pth
			break
		}
	}
	if apiPath == "" {
		t.Fatalf("overview should link to api map, body=%s", ovBody)
	}
	apiPayload, err := json.Marshal(map[string]string{"path": apiPath})
	if err != nil {
		t.Fatal(err)
	}
	apiBody, err := memtools.ExecTool(st, cfg, memtools.NameRead, string(apiPayload))
	if err != nil {
		t.Fatal(err)
	}
	gotVault := false
	for _, pth := range extractLinkTargetsForChain(apiBody) {
		if pth != "global:topics/secrets/vault.md" {
			continue
		}
		gotVault = true
		pl, jerr := json.Marshal(map[string]string{"path": pth})
		if jerr != nil {
			t.Fatal(jerr)
		}
		vaultBody, rerr := memtools.ExecTool(st, cfg, memtools.NameRead, string(pl))
		if rerr != nil {
			t.Fatal(rerr)
		}
		if !strings.Contains(vaultBody, secret) {
			t.Fatalf("vault body missing secret: %q", vaultBody)
		}
	}
	if !gotVault {
		t.Fatalf("api-map should reference vault.md, body=%s", apiBody)
	}
}
