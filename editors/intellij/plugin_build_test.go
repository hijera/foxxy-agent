// Build tests for the IntelliJ plugin.
//
// The Gradle build needs JDK 17 plus Go and Node, so it only runs in the
// intellij-plugin.yaml workflow. These tests run with plain `go test ./...`
// (i.e. in `make test` on every PR) and validate everything that commonly
// breaks the plugin zip without a compiler noticing: the plugin.xml
// descriptor referencing classes or resources that do not exist, locale
// bundles drifting apart, the Gradle build config drifting from the release
// targets shared with the VS Code extension, and a missing Gradle wrapper.
package intellij

import (
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	kotlinRoot    = "src/main/kotlin"
	resourcesRoot = "src/main/resources"
	pluginXMLPath = resourcesRoot + "/META-INF/plugin.xml"
)

// descriptorRefs walks plugin.xml (validating well-formedness along the way)
// and collects attribute values that must resolve against plugin sources.
type descriptorRefs struct {
	classes []string // fully-qualified Kotlin class names
	actions []string // AnAction implementation classes
	icons   []string // resource paths like /icons/foo.svg
	id      string
	name    string
	depends []string
}

func parsePluginXML(t *testing.T) descriptorRefs {
	t.Helper()
	data, err := os.ReadFile(pluginXMLPath)
	if err != nil {
		t.Fatalf("read plugin.xml: %v", err)
	}
	var refs descriptorRefs
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var inID, inName, inDepends bool
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("plugin.xml is not well-formed XML: %v", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			inID = el.Name.Local == "id"
			inName = el.Name.Local == "name"
			inDepends = el.Name.Local == "depends"
			for _, a := range el.Attr {
				switch a.Name.Local {
				case "serviceImplementation", "instance", "factoryClass", "implementationClass":
					refs.classes = append(refs.classes, a.Value)
				case "class":
					if el.Name.Local == "action" {
						refs.actions = append(refs.actions, a.Value)
					}
				case "icon":
					refs.icons = append(refs.icons, a.Value)
				}
			}
		case xml.CharData:
			text := strings.TrimSpace(string(el))
			switch {
			case inID && text != "":
				refs.id = text
			case inName && text != "":
				refs.name = text
			case inDepends && text != "":
				refs.depends = append(refs.depends, text)
			}
		case xml.EndElement:
			inID, inName, inDepends = false, false, false
		}
	}
	return refs
}

func TestPluginDescriptor(t *testing.T) {
	refs := parsePluginXML(t)

	if refs.id != "dev.foxxycode.intellij" {
		t.Errorf("plugin id = %q, want dev.foxxycode.intellij", refs.id)
	}
	if refs.name == "" {
		t.Error("plugin.xml must declare a <name>")
	}
	// Platform-only dependency keeps the plugin installable in every
	// IntelliJ-based IDE (see the comment in plugin.xml).
	found := false
	for _, d := range refs.depends {
		if d == "com.intellij.modules.platform" {
			found = true
		}
	}
	if !found {
		t.Errorf("plugin.xml <depends> = %v, must include com.intellij.modules.platform", refs.depends)
	}
	if len(refs.classes) == 0 {
		t.Fatal("plugin.xml declares no extension classes; descriptor parsing is likely broken")
	}
}

// Every class the descriptor references must exist as a Kotlin source file
// with a matching package and class declaration, or the plugin fails at
// runtime with PluginException: class not found.
func TestPluginDescriptorClassesExist(t *testing.T) {
	refs := parsePluginXML(t)
	for _, fqcn := range refs.classes {
		assertKotlinClassExists(t, fqcn)
	}
}

// Every action the descriptor references must exist as a Kotlin source file
// with a matching class declaration, or the plugin fails at runtime.
func TestPluginDescriptorActionsExist(t *testing.T) {
	refs := parsePluginXML(t)
	if len(refs.actions) == 0 {
		t.Fatal("plugin.xml declares no actions; FoxxyCodeWelcomeAction is expected")
	}
	for _, fqcn := range refs.actions {
		assertKotlinClassExists(t, fqcn)
	}
}

func assertKotlinClassExists(t *testing.T, fqcn string) {
	t.Helper()
	dot := strings.LastIndex(fqcn, ".")
	if dot < 0 {
		t.Errorf("class %q in plugin.xml is not fully qualified", fqcn)
		return
	}
	pkg, cls := fqcn[:dot], fqcn[dot+1:]
	file := filepath.Join(kotlinRoot, filepath.FromSlash(strings.ReplaceAll(fqcn, ".", "/"))+".kt")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Errorf("plugin.xml references %s but %s does not exist", fqcn, file)
		return
	}
	src := string(data)
	if !strings.Contains(src, "package "+pkg) {
		t.Errorf("%s does not declare package %s", file, pkg)
	}
	if !regexp.MustCompile(`(?m)\bclass ` + regexp.QuoteMeta(cls) + `\b`).MatchString(src) {
		t.Errorf("%s does not declare class %s", file, cls)
	}
}

func TestWelcomeHtmlResource(t *testing.T) {
	const welcomePath = resourcesRoot + "/welcome/welcome.html"
	data, err := os.ReadFile(welcomePath)
	if err != nil {
		t.Fatalf("read %s: %v", welcomePath, err)
	}
	html := string(data)
	for i := 1; i <= 5; i++ {
		placeholder := fmt.Sprintf("%%welcome.step%d.title%%", i)
		if !strings.Contains(html, placeholder) {
			t.Errorf("%s must reference placeholder %s", welcomePath, placeholder)
		}
	}
}

func TestPluginDescriptorIconsExist(t *testing.T) {
	refs := parsePluginXML(t)
	if len(refs.icons) == 0 {
		t.Fatal("plugin.xml declares no icons; the tool window icon is expected")
	}
	for _, icon := range refs.icons {
		file := filepath.Join(resourcesRoot, filepath.FromSlash(strings.TrimPrefix(icon, "/")))
		if _, err := os.Stat(file); err != nil {
			t.Errorf("plugin.xml references icon %s but %s is missing", icon, file)
		}
	}
}

// propertiesKeys parses a .properties file into its key set. Values do not
// matter here; continuation lines (trailing backslash) are folded away.
func propertiesKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	keys := make(map[string]bool)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	continuation := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if continuation {
			continuation = strings.HasSuffix(line, "\\")
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		continuation = strings.HasSuffix(line, "\\")
		if eq := strings.Index(line, "="); eq > 0 {
			keys[strings.TrimSpace(line[:eq])] = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return keys
}

// The ru bundle must mirror the default bundle exactly, otherwise switching
// the language yields raw message keys in the UI.
func TestMessageBundleLocaleParity(t *testing.T) {
	en := propertiesKeys(t, resourcesRoot+"/messages/FoxxyCodeBundle.properties")
	ru := propertiesKeys(t, resourcesRoot+"/messages/FoxxyCodeBundle_ru.properties")
	if len(en) == 0 {
		t.Fatal("default bundle has no keys; parsing is likely broken")
	}
	for k := range en {
		if !ru[k] {
			t.Errorf("key %q missing from FoxxyCodeBundle_ru.properties", k)
		}
	}
	for k := range ru {
		if !en[k] {
			t.Errorf("key %q only exists in FoxxyCodeBundle_ru.properties", k)
		}
	}
}

func TestGradleBuildConfig(t *testing.T) {
	data, err := os.ReadFile("build.gradle.kts")
	if err != nil {
		t.Fatalf("read build.gradle.kts: %v", err)
	}
	gradle := string(data)

	// The plugin advertises compatibility from build 223 (2022.3) with no
	// upper bound; the toolchain is pinned to what 2022.3's JBR supports.
	for _, want := range []string{
		`sinceBuild.set("223")`,
		`untilBuild.set("")`,
		`JavaVersion.VERSION_17`,
		`jvmTarget = "17"`,
		`version.set("2022.3")`,
	} {
		if !strings.Contains(gradle, want) {
			t.Errorf("build.gradle.kts no longer contains %q", want)
		}
	}

	// Production builds must verify all bundled binaries before packaging.
	if !strings.Contains(gradle, "foxxycodeVerifyBinaries") {
		t.Error("build.gradle.kts lost the foxxycodeVerifyBinaries fail-fast task")
	}
}

func TestGradleWrapperPresent(t *testing.T) {
	for _, f := range []string{
		"gradlew",
		"gradlew.bat",
		"gradle/wrapper/gradle-wrapper.jar",
		"gradle/wrapper/gradle-wrapper.properties",
		"settings.gradle.kts",
	} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("missing %s — `make intellij-build` cannot run without it", f)
		}
	}
}

// Both plugins bundle the same set of foxxycode binaries. The Gradle
// binTargets list and the VS Code prepare-binary.mjs TARGETS list must stay
// identical, or one plugin silently ships fewer platforms than the other.
func TestBinaryTargetsMatchVSCodePlugin(t *testing.T) {
	gradleData, err := os.ReadFile("build.gradle.kts")
	if err != nil {
		t.Fatalf("read build.gradle.kts: %v", err)
	}
	nodeData, err := os.ReadFile("../vscode/scripts/prepare-binary.mjs")
	if err != nil {
		t.Fatalf("read prepare-binary.mjs: %v", err)
	}

	targets := func(re *regexp.Regexp, src string) map[string]bool {
		set := make(map[string]bool)
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			set[m[1]+"-"+m[2]] = true
		}
		return set
	}
	gradleTargets := targets(
		regexp.MustCompile(`BinTarget\("([a-z0-9]+)", "([a-z0-9]+)"\)`), string(gradleData))
	nodeTargets := targets(
		regexp.MustCompile(`\{ goos: "([a-z0-9]+)", goarch: "([a-z0-9]+)" \}`), string(nodeData))

	if len(gradleTargets) == 0 || len(nodeTargets) == 0 {
		t.Fatalf("failed to extract targets (gradle=%d, vscode=%d); the parsing regexes need updating",
			len(gradleTargets), len(nodeTargets))
	}
	for tgt := range gradleTargets {
		if !nodeTargets[tgt] {
			t.Errorf("target %s is built for IntelliJ but not for VS Code", tgt)
		}
	}
	for tgt := range nodeTargets {
		if !gradleTargets[tgt] {
			t.Errorf("target %s is built for VS Code but not for IntelliJ", tgt)
		}
	}
}
