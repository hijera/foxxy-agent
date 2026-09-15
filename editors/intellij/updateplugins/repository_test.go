// Tests for the plugin repository document. The generator is the only thing standing between a
// bad build and every IDE that trusts the repository URL, so the fixtures mimic the real
// distribution layout (descriptor inside a jar, next to unrelated jars and bundled binaries)
// instead of the ~70 MB artifact the release actually ships.
package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// descriptorXML renders a plugin.xml shaped like the one patchPluginXml produces: CDATA
// description, a vendor element carrying an attribute, change notes, injected version and
// idea-version.
func descriptorXML(id, name, version, sinceBuild, untilBuild string) string {
	until := ""
	if untilBuild != "" {
		until = ` until-build="` + untilBuild + `"`
	}
	return `<idea-plugin>
  <id>` + id + `</id>
  <name>` + name + `</name>
  <vendor email="hebopine981@gmail.com">FoxxyCode</vendor>
  <description><![CDATA[<p>Run the full <b>FoxxyCode</b> agent inside any JetBrains IDE.</p>]]></description>
  <change-notes><![CDATA[<h3>` + version + `</h3><ul><li>Things happened.</li></ul>]]></change-notes>
  <depends>com.intellij.modules.platform</depends>
  <version>` + version + `</version>
  <idea-version since-build="` + sinceBuild + `"` + until + `/>
</idea-plugin>`
}

// pluginZip writes a plugin distribution zip whose descriptor lives inside a nested jar.
func pluginZip(t *testing.T, name, pluginXML string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer func() { _ = file.Close() }()

	archive := zip.NewWriter(file)
	add := func(entry string, content []byte) {
		t.Helper()
		writer, err := archive.Create(entry)
		if err != nil {
			t.Fatalf("add %s: %v", entry, err)
		}
		if _, err := writer.Write(content); err != nil {
			t.Fatalf("write %s: %v", entry, err)
		}
	}

	// An unrelated dependency jar: a real archive, but with no plugin descriptor.
	add("FoxxyCode/lib/kotlin-stdlib.jar", jar(t, map[string]string{"kotlin/KotlinVersion.class": "not really a class"}))
	if pluginXML != "" {
		add("FoxxyCode/lib/foxxycode-intellij.jar", jar(t, map[string]string{
			"META-INF/plugin.xml":  pluginXML,
			"META-INF/MANIFEST.MF": "Manifest-Version: 1.0\n",
		}))
	}
	// The bundled binaries, which dwarf everything else in the real artifact.
	add("FoxxyCode/foxxycode-bin/linux-amd64/foxxycode", []byte("\x7fELF not really"))

	if err := archive.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return path
}

func jar(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range entries {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatalf("add %s to jar: %v", name, err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("write %s to jar: %v", name, err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close jar: %v", err)
	}
	return buffer.Bytes()
}

func TestInspectReadsDescriptorFromNestedJAR(t *testing.T) {
	path := pluginZip(t, "foxxycode-intellij-1.2.3.zip",
		descriptorXML("dev.foxxycode.intellij", "FoxxyCode", "1.2.3", "222", ""))

	found, err := inspect(path, "dev.foxxycode.intellij")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	want := descriptor{
		ID:         "dev.foxxycode.intellij",
		Name:       "FoxxyCode",
		Version:    "1.2.3",
		Vendor:     "FoxxyCode",
		SinceBuild: "222",
	}
	if found != want {
		t.Fatalf("descriptor = %+v, want %+v", found, want)
	}
}

func TestInspectFailsWithoutDescriptor(t *testing.T) {
	path := pluginZip(t, "foxxycode-intellij-1.2.3.zip", "")

	if _, err := inspect(path, "dev.foxxycode.intellij"); err == nil {
		t.Fatal("expected an error for a zip without a plugin descriptor")
	}
}

func TestInspectFailsOnForeignPluginID(t *testing.T) {
	path := pluginZip(t, "foxxycode-intellij-1.2.3.zip",
		descriptorXML("com.example.other", "Other", "1.2.3", "222", ""))

	if _, err := inspect(path, "dev.foxxycode.intellij"); err == nil {
		t.Fatal("expected an error when no jar declares the expected plugin id")
	}
}

func TestValidate(t *testing.T) {
	good := descriptor{
		ID:         "dev.foxxycode.intellij",
		Name:       "FoxxyCode",
		Version:    "1.2.3",
		Vendor:     "FoxxyCode",
		SinceBuild: "222",
	}

	for _, testCase := range []struct {
		name    string
		mutate  func(*descriptor)
		tag     string
		wantErr string
	}{
		{name: "accepts a matching descriptor", mutate: func(*descriptor) {}, tag: "1.2.3"},
		{
			name:    "rejects a foreign plugin id",
			mutate:  func(d *descriptor) { d.ID = "com.example.other" },
			tag:     "1.2.3",
			wantErr: "plugin ID mismatch",
		},
		{
			name:    "rejects a renamed plugin",
			mutate:  func(d *descriptor) { d.Name = "Foxxy Code" },
			tag:     "1.2.3",
			wantErr: "plugin name mismatch",
		},
		{
			name:    "rejects a version that disagrees with the tag",
			mutate:  func(*descriptor) {},
			tag:     "1.2.4",
			wantErr: "plugin version mismatch",
		},
		{
			name:    "rejects a missing since-build",
			mutate:  func(d *descriptor) { d.SinceBuild = "" },
			tag:     "1.2.3",
			wantErr: "declares no since-build",
		},
		{
			name:    "rejects a since-build that is not a build number",
			mutate:  func(d *descriptor) { d.SinceBuild = "latest" },
			tag:     "1.2.3",
			wantErr: "is not an IntelliJ build number",
		},
		{
			// The IDE version instead of the platform branch: parses, then hides the
			// plugin from every IDE, because no real build is newer than 2022.
			name:    "rejects an IDE version in place of a since-build",
			mutate:  func(d *descriptor) { d.SinceBuild = "2022.2" },
			tag:     "1.2.3",
			wantErr: "three-digit platform branch",
		},
		{
			name:    "rejects an overlong build number",
			mutate:  func(d *descriptor) { d.SinceBuild = "222.1.2.3.4.5.6.7.8" },
			tag:     "1.2.3",
			wantErr: "more than 8 components",
		},
		{
			name:    "accepts a SNAPSHOT until-build",
			mutate:  func(d *descriptor) { d.UntilBuild = "231.SNAPSHOT" },
			tag:     "1.2.3",
			wantErr: "",
		},
		{
			name:    "rejects an unusable until-build",
			mutate:  func(d *descriptor) { d.UntilBuild = "latest" },
			tag:     "1.2.3",
			wantErr: "unusable until-build",
		},
		{
			name:    "accepts a wildcard until-build",
			mutate:  func(d *descriptor) { d.UntilBuild = "231.*" },
			tag:     "1.2.3",
			wantErr: "",
		},
		{
			name:    "accepts a product-prefixed since-build",
			mutate:  func(d *descriptor) { d.SinceBuild = "IC-222.4459.24" },
			tag:     "1.2.3",
			wantErr: "",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := good
			testCase.mutate(&candidate)

			err := validate(candidate, "dev.foxxycode.intellij", "FoxxyCode", testCase.tag)
			switch {
			case testCase.wantErr == "" && err != nil:
				t.Fatalf("validate: %v", err)
			case testCase.wantErr != "" && err == nil:
				t.Fatalf("expected an error containing %q", testCase.wantErr)
			case testCase.wantErr != "" && !strings.Contains(err.Error(), testCase.wantErr):
				t.Fatalf("error = %q, want it to contain %q", err, testCase.wantErr)
			}
		})
	}
}

// The exact bytes matter: this is the contract with every IDE polling the repository URL.
func TestRenderMatchesJetBrainsFormat(t *testing.T) {
	found := descriptor{
		ID:         "dev.foxxycode.intellij",
		Name:       "FoxxyCode",
		Version:    "1.2.3",
		Vendor:     "FoxxyCode",
		SinceBuild: "222",
	}

	document, err := render(found, downloadURL("hijera/foxxy-agent", "1.2.3", "foxxycode-intellij-1.2.3.zip"))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	want := `<?xml version="1.0" encoding="UTF-8"?>
<!-- Generated by editors/intellij/updateplugins on every release. Do not edit by hand. -->
<plugins>
    <plugin id="dev.foxxycode.intellij" url="https://github.com/hijera/foxxy-agent/releases/download/1.2.3/foxxycode-intellij-1.2.3.zip" version="1.2.3">
        <idea-version since-build="222"></idea-version>
        <name>FoxxyCode</name>
        <vendor>FoxxyCode</vendor>
    </plugin>
</plugins>
`
	if string(document) != want {
		t.Fatalf("document =\n%s\nwant\n%s", document, want)
	}
}

func TestRenderKeepsUntilBuild(t *testing.T) {
	found := descriptor{
		ID:         "dev.foxxycode.intellij",
		Name:       "FoxxyCode",
		Version:    "1.2.3",
		Vendor:     "FoxxyCode",
		SinceBuild: "222",
		UntilBuild: "231.*",
	}

	document, err := render(found, "https://example.test/plugin.zip")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(document), `<idea-version since-build="222" until-build="231.*">`) {
		t.Fatalf("until-build is missing from\n%s", document)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, testCase := range []struct {
		left, right string
		want        int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.4", "1.2.3", 1},
		{"1.2.3", "1.2.4", -1},
		{"1.3.0", "1.2.99", 1},
		{"2.0.0", "10.0.0", -1},
		{"0.2.68", "0.2.9", 1},
		// A hand-edited or dev value must lose to a real release.
		{"1.2.3", "0.0.0-dev-abc1234", 1},
		{"0.0.0-dev-abc1234", "1.2.3", -1},
	} {
		if got := compareVersions(testCase.left, testCase.right); got != testCase.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", testCase.left, testCase.right, got, testCase.want)
		}
	}
}

func TestPublishedVersionOfMissingFile(t *testing.T) {
	version, err := publishedVersion(filepath.Join(t.TempDir(), "updatePlugins.xml"))
	if err != nil {
		t.Fatalf("publishedVersion: %v", err)
	}
	if version != "" {
		t.Fatalf("version = %q, want empty", version)
	}
}

func TestRunWritesDocument(t *testing.T) {
	path := pluginZip(t, "foxxycode-intellij-1.2.3.zip",
		descriptorXML("dev.foxxycode.intellij", "FoxxyCode", "1.2.3", "222", ""))
	out := filepath.Join(t.TempDir(), "docs", "updatePlugins.xml")

	var log bytes.Buffer
	if err := run([]string{"-zip", path, "-tag", "1.2.3", "-out", out}, &log); err != nil {
		t.Fatalf("run: %v", err)
	}

	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read document: %v", err)
	}
	if !strings.Contains(string(written), `version="1.2.3"`) {
		t.Fatalf("document does not advertise 1.2.3:\n%s", written)
	}
	if !strings.Contains(string(written), "/releases/download/1.2.3/foxxycode-intellij-1.2.3.zip") {
		t.Fatalf("document does not point at the release asset:\n%s", written)
	}
}

func TestRunRefusesAVersionThatDisagreesWithTheTag(t *testing.T) {
	path := pluginZip(t, "foxxycode-intellij-1.2.4.zip",
		descriptorXML("dev.foxxycode.intellij", "FoxxyCode", "1.2.3", "222", ""))
	out := filepath.Join(t.TempDir(), "updatePlugins.xml")

	err := run([]string{"-zip", path, "-tag", "1.2.4", "-out", out}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "plugin version mismatch") {
		t.Fatalf("err = %v, want a version mismatch", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatal("a rejected build must not leave a document behind")
	}
}

// A workflow_dispatch rebuild of an old tag must not walk every IDE backwards.
func TestRunKeepsTheNewerPublishedVersion(t *testing.T) {
	out := filepath.Join(t.TempDir(), "updatePlugins.xml")
	newer := pluginZip(t, "foxxycode-intellij-1.3.0.zip",
		descriptorXML("dev.foxxycode.intellij", "FoxxyCode", "1.3.0", "222", ""))
	if err := run([]string{"-zip", newer, "-tag", "1.3.0", "-out", out}, &bytes.Buffer{}); err != nil {
		t.Fatalf("publish 1.3.0: %v", err)
	}

	older := pluginZip(t, "foxxycode-intellij-1.2.3.zip",
		descriptorXML("dev.foxxycode.intellij", "FoxxyCode", "1.2.3", "222", ""))
	var log bytes.Buffer
	if err := run([]string{"-zip", older, "-tag", "1.2.3", "-out", out}, &log); err != nil {
		t.Fatalf("republish 1.2.3: %v", err)
	}

	version, err := publishedVersion(out)
	if err != nil {
		t.Fatalf("publishedVersion: %v", err)
	}
	if version != "1.3.0" {
		t.Fatalf("version = %q, want it left at 1.3.0", version)
	}
	if !strings.Contains(log.String(), "already advertises 1.3.0") {
		t.Fatalf("log = %q, want it to explain the skip", log.String())
	}

	// -force is how a deliberate rollback says it means it.
	if err := run([]string{"-zip", older, "-tag", "1.2.3", "-out", out, "-force"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("forced rollback: %v", err)
	}
	version, err = publishedVersion(out)
	if err != nil {
		t.Fatalf("publishedVersion: %v", err)
	}
	if version != "1.2.3" {
		t.Fatalf("version = %q, want the forced 1.2.3", version)
	}
}

func TestRunRequiresZipAndTag(t *testing.T) {
	if err := run([]string{"-tag", "1.2.3"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected an error without -zip")
	}
	if err := run([]string{"-zip", "plugin.zip"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected an error without -tag")
	}
}
