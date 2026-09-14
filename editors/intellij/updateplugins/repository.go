// Renders the JetBrains custom-plugin-repository document (updatePlugins.xml) that lets any
// IntelliJ-platform IDE install and auto-update FoxxyCode straight from GitHub Releases.
//
// A JetBrains plugin repository is nothing but a URL serving this one XML file; the plugin zip
// itself may live anywhere reachable over HTTPS. So instead of mirroring releases onto a server,
// the document points at the release asset GitHub already hosts, and CI republishes it on every
// tag (GitHub Pages for the primary URL, plus a copy attached to the release).
//
// The descriptor is read out of the built zip rather than taken from build.gradle.kts: what the
// IDE is told must match what it will actually download, or the install fails on the user side.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Guards against a hostile or simply broken archive: the plugin zip is ~70 MB of bundled
// foxxycode binaries, and only the small jars inside it are ever read into memory.
const (
	maxZipEntries    = 50_000
	maxJARSize       = 64 << 20
	maxPluginXMLSize = 2 << 20
)

// descriptor is the subset of META-INF/plugin.xml the repository document needs.
type descriptor struct {
	ID         string
	Name       string
	Version    string
	Vendor     string
	SinceBuild string
	UntilBuild string
}

// pluginXML mirrors the descriptor the IntelliJ platform reads out of a plugin jar.
type pluginXML struct {
	ID          string `xml:"id"`
	Name        string `xml:"name"`
	Version     string `xml:"version"`
	Vendor      vendor `xml:"vendor"`
	IdeaVersion struct {
		SinceBuild string `xml:"since-build,attr"`
		UntilBuild string `xml:"until-build,attr"`
	} `xml:"idea-version"`
}

type vendor struct {
	Name string `xml:",chardata"`
}

// inspect finds the plugin descriptor carrying expectedID inside a built plugin zip. The
// descriptor lives in META-INF/plugin.xml of one of the jars under <plugin>/lib/.
func inspect(archivePath, expectedID string) (descriptor, error) {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return descriptor{}, fmt.Errorf("open plugin zip: %w", err)
	}
	defer func() { _ = archive.Close() }()

	if len(archive.File) == 0 {
		return descriptor{}, errors.New("plugin zip is empty")
	}
	if len(archive.File) > maxZipEntries {
		return descriptor{}, fmt.Errorf("plugin zip has too many entries: %d", len(archive.File))
	}

	var found []descriptor
	for _, file := range archive.File {
		name := strings.ToLower(strings.ReplaceAll(file.Name, `\`, "/"))
		switch {
		case name == "meta-inf/plugin.xml" || strings.HasSuffix(name, "/meta-inf/plugin.xml"):
			parsed, err := descriptorFromEntry(file)
			if err != nil {
				return descriptor{}, fmt.Errorf("read %s: %w", file.Name, err)
			}
			if parsed.ID == expectedID {
				found = append(found, parsed)
			}
		case strings.HasSuffix(name, ".jar"):
			parsed, ok, err := descriptorFromJAR(file, expectedID)
			if err != nil {
				return descriptor{}, fmt.Errorf("inspect %s: %w", file.Name, err)
			}
			if ok {
				found = append(found, parsed)
			}
		}
	}

	switch len(found) {
	case 0:
		return descriptor{}, fmt.Errorf("no META-INF/plugin.xml with id %q inside %s", expectedID, filepath.Base(archivePath))
	case 1:
		return found[0], nil
	default:
		for _, other := range found[1:] {
			if other != found[0] {
				return descriptor{}, fmt.Errorf("plugin zip carries conflicting descriptors for id %q", expectedID)
			}
		}
		return found[0], nil
	}
}

// descriptorFromJAR reads a nested jar into memory (jars in the distribution are small; the
// bundled binaries are not jars) and looks for the plugin descriptor inside it.
func descriptorFromJAR(file *zip.File, expectedID string) (descriptor, bool, error) {
	if file.UncompressedSize64 > maxJARSize {
		return descriptor{}, false, nil
	}
	raw, err := readEntry(file, maxJARSize)
	if err != nil {
		return descriptor{}, false, err
	}
	jar, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		// Not every ".jar" in a plugin zip has to be a readable archive; skip rather than fail.
		return descriptor{}, false, nil
	}
	for _, entry := range jar.File {
		if strings.ToLower(strings.ReplaceAll(entry.Name, `\`, "/")) != "meta-inf/plugin.xml" {
			continue
		}
		parsed, err := descriptorFromEntry(entry)
		if err != nil {
			return descriptor{}, false, err
		}
		if parsed.ID == expectedID {
			return parsed, true, nil
		}
	}
	return descriptor{}, false, nil
}

func descriptorFromEntry(file *zip.File) (descriptor, error) {
	raw, err := readEntry(file, maxPluginXMLSize)
	if err != nil {
		return descriptor{}, err
	}
	var parsed pluginXML
	if err := xml.Unmarshal(raw, &parsed); err != nil {
		return descriptor{}, fmt.Errorf("parse plugin.xml: %w", err)
	}
	return descriptor{
		ID:         strings.TrimSpace(parsed.ID),
		Name:       strings.TrimSpace(parsed.Name),
		Version:    strings.TrimSpace(parsed.Version),
		Vendor:     strings.TrimSpace(parsed.Vendor.Name),
		SinceBuild: strings.TrimSpace(parsed.IdeaVersion.SinceBuild),
		UntilBuild: strings.TrimSpace(parsed.IdeaVersion.UntilBuild),
	}, nil
}

func readEntry(file *zip.File, limit int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", file.Name, limit)
	}
	return raw, nil
}

// validate refuses to advertise an archive that does not describe the plugin it claims to.
// A wrong id makes the IDE treat the release as a different plugin; a version that disagrees
// with the tag means the IDE downloads something other than what the document promised.
func validate(d descriptor, expectedID, expectedName, tag string) error {
	if d.ID != expectedID {
		return fmt.Errorf("plugin ID mismatch: expected %q, got %q", expectedID, d.ID)
	}
	if d.Name != expectedName {
		return fmt.Errorf("plugin name mismatch: expected %q, got %q", expectedName, d.Name)
	}
	if d.Version != tag {
		return fmt.Errorf("plugin version mismatch: git tag is %q, plugin.xml contains %q", tag, d.Version)
	}
	if d.SinceBuild == "" {
		return errors.New("plugin.xml declares no since-build")
	}
	if err := validateBuildNumber(d.SinceBuild); err != nil {
		return fmt.Errorf("plugin.xml declares an unusable since-build: %w", err)
	}
	if d.UntilBuild != "" {
		if err := validateBuildNumber(d.UntilBuild); err != nil {
			return fmt.Errorf("plugin.xml declares an unusable until-build: %w", err)
		}
	}
	return nil
}

// IntelliJ build numbers look like "222", "222.4459.24", "IC-222.4459.24", "223.*" or
// "231.SNAPSHOT" — a product code, then dot-separated components, the first of which is the
// three-digit platform branch.
var buildNumberPattern = regexp.MustCompile(`^([A-Za-z]{2,}-)?([0-9]+|\*|(?i:SNAPSHOT))(\.([0-9]+|\*|(?i:SNAPSHOT)))*$`)

// productCodePattern strips the "IC-" in "IC-222.4459.24".
var productCodePattern = regexp.MustCompile(`^[A-Za-z]{2,}-`)

const maxBuildComponents = 8

func validateBuildNumber(build string) error {
	if !buildNumberPattern.MatchString(build) {
		return fmt.Errorf("%q is not an IntelliJ build number", build)
	}
	components := strings.Split(productCodePattern.ReplaceAllString(build, ""), ".")
	if len(components) > maxBuildComponents {
		return fmt.Errorf("%q has more than %d components", build, maxBuildComponents)
	}
	// The classic footgun is writing the IDE version ("2022.2") where the platform branch
	// ("222") belongs. That parses fine and then silently hides the plugin from every IDE,
	// because no real build is greater than 2022.
	branch, err := strconv.Atoi(components[0])
	if err == nil && (branch < 100 || branch > 999) {
		return fmt.Errorf("%q does not start with a three-digit platform branch (222 is IDE version 2022.2)", build)
	}
	return nil
}

// pluginsXML is the repository document itself. JetBrains allows a plugin id to appear only
// once per document, so exactly one version is advertised: the newest release.
type pluginsXML struct {
	XMLName xml.Name           `xml:"plugins"`
	Plugins []repositoryPlugin `xml:"plugin"`
}

type repositoryPlugin struct {
	ID          string      `xml:"id,attr"`
	URL         string      `xml:"url,attr"`
	Version     string      `xml:"version,attr"`
	IdeaVersion ideaVersion `xml:"idea-version"`
	Name        string      `xml:"name"`
	Vendor      string      `xml:"vendor"`
}

type ideaVersion struct {
	SinceBuild string `xml:"since-build,attr"`
	UntilBuild string `xml:"until-build,attr,omitempty"`
}

// downloadURL is the permanent link to a release asset on GitHub.
func downloadURL(repository, tag, assetName string) string {
	return "https://github.com/" + repository + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(assetName)
}

func render(d descriptor, assetURL string) ([]byte, error) {
	document := pluginsXML{Plugins: []repositoryPlugin{{
		ID:      d.ID,
		URL:     assetURL,
		Version: d.Version,
		IdeaVersion: ideaVersion{
			SinceBuild: d.SinceBuild,
			UntilBuild: d.UntilBuild,
		},
		Name:   d.Name,
		Vendor: d.Vendor,
	}}}
	content, err := xml.MarshalIndent(document, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("build updatePlugins.xml: %w", err)
	}
	// docs/ is otherwise hand-written, so the published copy says who owns it: a manual
	// edit here is silently replaced by the next release.
	return []byte(xml.Header + generatedBanner + string(content) + "\n"), nil
}

// generatedBanner marks the document as machine-managed.
const generatedBanner = "<!-- Generated by editors/intellij/updateplugins on every release. Do not edit by hand. -->\n"

// publishedVersion reads the version an existing repository document advertises. A missing
// file is not an error: the first publication has nothing to compare against.
func publishedVersion(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var document pluginsXML
	if err := xml.Unmarshal(raw, &document); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	if len(document.Plugins) == 0 {
		return "", nil
	}
	return strings.TrimSpace(document.Plugins[0].Version), nil
}

// compareVersions orders two X.Y.Z versions. Anything that is not SemVer (a hand-edited file,
// a dev build) compares as older, so a real release always wins.
func compareVersions(a, b string) int {
	left, leftOK := parseVersion(a)
	right, rightOK := parseVersion(b)
	switch {
	case !leftOK && !rightOK:
		return 0
	case !leftOK:
		return -1
	case !rightOK:
		return 1
	}
	for i := range left {
		if left[i] != right[i] {
			if left[i] < right[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parseVersion(version string) ([3]int, bool) {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var parsed [3]int
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return [3]int{}, false
		}
		parsed[i] = number
	}
	return parsed, true
}

// writeDocument replaces path atomically, so a reader (the GitHub Pages build, a concurrent
// curl) never sees a half-written document.
func writeDocument(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".updatePlugins-*.xml")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return nil
}
