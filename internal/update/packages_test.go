package update

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestPackageAssetFileName(t *testing.T) {
	for _, tc := range []struct {
		format string
		goarch string
		want   string
	}{
		{"deb", "amd64", "foxxycode_1.2.3_linux_amd64.deb"},
		{"deb", "arm64", "foxxycode_1.2.3_linux_arm64.deb"},
		{"rpm", "amd64", "foxxycode_1.2.3_linux_amd64.rpm"},
		{"rpm", "arm64", "foxxycode_1.2.3_linux_arm64.rpm"},
	} {
		got, err := PackageAssetFileName("1.2.3", tc.format, tc.goarch)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.format, tc.goarch, err)
		}
		if got != tc.want {
			t.Fatalf("%s/%s = %q, want %q", tc.format, tc.goarch, got, tc.want)
		}
	}

	if _, err := PackageAssetFileName("1.2.3", "apk", "amd64"); err == nil {
		t.Fatal("unsupported format accepted")
	}
	if _, err := PackageAssetFileName("1.2.3", "deb", "riscv64"); err == nil {
		t.Fatal("unsupported architecture accepted")
	}
}

// lookPathFor answers as a host where only the named tools are installed.
func lookPathFor(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		if slices.Contains(names, name) {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}

func TestPackageFrontEndPrefersTheResolvingTool(t *testing.T) {
	for _, tc := range []struct {
		name    string
		format  packageFormat
		present []string
		want    string
	}{
		{"apt over dpkg", formatDeb, []string{"dpkg", "apt-get"}, "apt-get"},
		{"apt when apt-get is absent", formatDeb, []string{"dpkg", "apt"}, "apt"},
		{"dpkg alone", formatDeb, []string{"dpkg"}, "dpkg"},
		{"deb fallback with nothing installed", formatDeb, nil, "dpkg"},
		{"dnf over rpm", formatRPM, []string{"rpm", "dnf"}, "dnf"},
		{"zypper when dnf is absent", formatRPM, []string{"rpm", "zypper", "yum"}, "zypper"},
		{"rpm fallback with nothing installed", formatRPM, nil, "rpm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := packageFrontEnd(packageEnv{LookPath: lookPathFor(tc.present...)}, tc.format)
			if got != tc.want {
				t.Fatalf("front end = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPackageInstallCommandIsNonInteractive(t *testing.T) {
	for _, tc := range []struct {
		manager string
		want    string
	}{
		{"apt-get", "apt-get install -y --allow-downgrades /tmp/foxxycode.deb"},
		{"apt", "apt install -y --allow-downgrades /tmp/foxxycode.deb"},
		{"dpkg", "dpkg --install /tmp/foxxycode.deb"},
		{"dnf", "dnf install -y /tmp/foxxycode.deb"},
		{"yum", "yum install -y /tmp/foxxycode.deb"},
		{"zypper", "zypper --non-interactive install --allow-unsigned-rpm /tmp/foxxycode.deb"},
		{"rpm", "rpm --upgrade --oldpackage /tmp/foxxycode.deb"},
	} {
		got := strings.Join(packageInstallCommand(tc.manager, "/tmp/foxxycode.deb"), " ")
		if got != tc.want {
			t.Fatalf("%s: command = %q, want %q", tc.manager, got, tc.want)
		}
	}
}

// A low-level tool cannot upgrade from a repository, so the hint has to send
// the user back through FoxxyCode, which downloads the file the tool needs.
func TestPackageUpgradeHintFallsBackToFoxxyCodeForFileOnlyTools(t *testing.T) {
	for _, tc := range []struct {
		manager string
		want    string
	}{
		{"apt-get", "sudo apt-get install --only-upgrade foxxycode"},
		{"dnf", "sudo dnf upgrade foxxycode"},
		{"zypper", "sudo zypper update foxxycode"},
		{"dpkg", "sudo foxxycode update"},
		{"rpm", "sudo foxxycode update"},
	} {
		got := packageUpgradeHint(systemPackage{Name: "foxxycode", Manager: tc.manager})
		if got != tc.want {
			t.Fatalf("%s: hint = %q, want %q", tc.manager, got, tc.want)
		}
	}
}

func TestDetectSystemPackage(t *testing.T) {
	const owned = "/usr/bin/foxxycode"

	debEnv := func(out string, err error) packageEnv {
		return packageEnv{
			GOOS:     "linux",
			LookPath: lookPathFor("dpkg-query", "apt-get"),
			Query:    func(context.Context, string, ...string) (string, error) { return out, err },
		}
	}

	t.Run("dpkg ownership", func(t *testing.T) {
		pkg, ok := detectSystemPackage(context.Background(), debEnv("foxxycode: /usr/bin/foxxycode\n", nil), owned)
		if !ok {
			t.Fatal("owned file reported as unmanaged")
		}
		if pkg.Format != formatDeb || pkg.Name != "foxxycode" || pkg.Manager != "apt-get" {
			t.Fatalf("package = %+v", pkg)
		}
	})

	t.Run("several packages declare the file", func(t *testing.T) {
		pkg, ok := detectSystemPackage(context.Background(), debEnv("foxxycode, foxxycode-extras: /usr/bin/foxxycode\n", nil), owned)
		if !ok || pkg.Name != "foxxycode" {
			t.Fatalf("package = %+v, ok = %v; want the first listed package", pkg, ok)
		}
	})

	t.Run("dpkg reports no owner", func(t *testing.T) {
		if _, ok := detectSystemPackage(context.Background(), debEnv("", errors.New("exit 1")), owned); ok {
			t.Fatal("unowned file reported as package-managed")
		}
	})

	t.Run("rpm ownership", func(t *testing.T) {
		env := packageEnv{
			GOOS:     "linux",
			LookPath: lookPathFor("rpm", "dnf"),
			Query:    func(context.Context, string, ...string) (string, error) { return "foxxycode", nil },
		}
		pkg, ok := detectSystemPackage(context.Background(), env, owned)
		if !ok || pkg.Format != formatRPM || pkg.Manager != "dnf" {
			t.Fatalf("package = %+v, ok = %v", pkg, ok)
		}
	})

	// Some rpm builds answer a miss on stdout with a zero exit ("file ... is
	// not owned by any package"), which must not be read as a package name.
	t.Run("rpm reports a miss on stdout", func(t *testing.T) {
		env := packageEnv{
			GOOS:     "linux",
			LookPath: lookPathFor("rpm"),
			Query: func(context.Context, string, ...string) (string, error) {
				return "file /usr/bin/foxxycode is not owned by any package", nil
			},
		}
		if _, ok := detectSystemPackage(context.Background(), env, owned); ok {
			t.Fatal("rpm miss text read as a package name")
		}
	})

	t.Run("no package tools installed", func(t *testing.T) {
		env := packageEnv{GOOS: "linux", LookPath: lookPathFor()}
		if _, ok := detectSystemPackage(context.Background(), env, owned); ok {
			t.Fatal("host without dpkg or rpm reported a package")
		}
	})

	// Homebrew is read off the resolved path, so it needs no package tool at
	// all and works the same on macOS and on Linux. The Caskroom and the Cellar
	// are upgraded by different commands, so the two are told apart.
	t.Run("homebrew", func(t *testing.T) {
		for _, tc := range []struct {
			path string
			cask bool
			hint string
		}{
			{"/opt/homebrew/Caskroom/foxxycode/1.0.11/foxxycode", true, "brew upgrade --cask foxxycode"},
			{"/home/dev/.linuxbrew/Caskroom/foxxycode/1.0.11/foxxycode", true, "brew upgrade --cask foxxycode"},
			{"/usr/local/Cellar/foxxycode/1.0.11/bin/foxxycode", false, "brew upgrade foxxycode"},
			{"/opt/homebrew/Cellar/foxxycode/1.0.13/bin/foxxycode", false, "brew upgrade foxxycode"},
		} {
			env := packageEnv{GOOS: "darwin", LookPath: lookPathFor()}
			pkg, ok := detectSystemPackage(context.Background(), env, tc.path)
			if !ok || pkg.Format != formatBrew || pkg.Name != "foxxycode" || pkg.Manager != "brew" {
				t.Fatalf("%s: package = %+v, ok = %v", tc.path, pkg, ok)
			}
			if pkg.Cask != tc.cask {
				t.Fatalf("%s: cask = %v, want %v", tc.path, pkg.Cask, tc.cask)
			}
			if got := packageUpgradeHint(pkg); got != tc.hint {
				t.Fatalf("%s: hint = %q, want %q", tc.path, got, tc.hint)
			}
		}
	})

	// A Cellar path whose package directory is missing is not an install; the
	// same holds for the Caskroom, so neither marker is trusted on its own.
	t.Run("a path that ends at the marker", func(t *testing.T) {
		env := packageEnv{GOOS: "darwin", LookPath: lookPathFor()}
		for _, path := range []string{"/opt/homebrew/Cellar/", "/opt/homebrew/Caskroom/"} {
			if _, ok := detectSystemPackage(context.Background(), env, path); ok {
				t.Fatalf("%s was read as a Homebrew install", path)
			}
		}
	})

	t.Run("a path that only mentions a cellar", func(t *testing.T) {
		env := packageEnv{GOOS: "darwin", LookPath: lookPathFor()}
		if _, ok := detectSystemPackage(context.Background(), env, "/home/dev/Cellar-notes/foxxycode"); ok {
			t.Fatal("an unrelated path was read as a Homebrew install")
		}
	})

	t.Run("not linux", func(t *testing.T) {
		env := packageEnv{
			GOOS:     "darwin",
			LookPath: lookPathFor("dpkg-query"),
			Query:    func(context.Context, string, ...string) (string, error) { return "foxxycode: x", nil },
		}
		if _, ok := detectSystemPackage(context.Background(), env, owned); ok {
			t.Fatal("package database queried outside linux")
		}
	})

	t.Run("no install path", func(t *testing.T) {
		env := packageEnv{
			GOOS:     "linux",
			LookPath: lookPathFor("dpkg-query"),
			Query: func(context.Context, string, ...string) (string, error) {
				t.Fatal("package database queried without a path")
				return "", nil
			},
		}
		if _, ok := detectSystemPackage(context.Background(), env, "  "); ok {
			t.Fatal("empty path reported as package-managed")
		}
	})
}

// A release older than the packaging pipeline publishes archives only. Root has
// to learn that from the update rather than from a bare "no asset" line.
func TestInstallSystemPackageWithoutAPackageAsset(t *testing.T) {
	rel := &ghRelease{TagName: "0.9.70", Assets: []releaseAsset{
		{Name: "foxxycode_0.9.70_linux_amd64.tar.gz", BrowserDownloadURL: "http://example.invalid/a"},
	}}
	env := packageEnv{
		GOOS:    "linux",
		Geteuid: func() int { return 0 },
		Install: func(context.Context, io.Writer, string, ...string) error {
			t.Fatal("package manager invoked without a package to install")
			return nil
		},
	}
	pkg := systemPackage{Format: formatDeb, Name: "foxxycode", Manager: "apt-get"}
	opts := Options{GOARCH: "amd64", Yes: true}

	err := installSystemPackage(context.Background(), opts, env, pkg, rel, "0.9.70", io.Discard, nil)
	if err == nil {
		t.Fatal("missing package asset accepted")
	}
	if !strings.Contains(err.Error(), "foxxycode_0.9.70_linux_amd64.deb") || !strings.Contains(err.Error(), "package manager") {
		t.Fatalf("error does not explain the missing package: %v", err)
	}
}
