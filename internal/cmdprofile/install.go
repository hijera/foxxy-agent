package cmdprofile

import (
	"os/exec"
	"runtime"
)

// Manager is one way to install the profile's binary on this machine: a
// package manager that is present on PATH AND for which the profile declares
// a coordinate. Argv is complete and literal — built entirely from the
// validated coordinate, never from request input.
type Manager struct {
	// ID identifies the manager: winget, scoop, brew, apt, dnf.
	ID string
	// Package is the declared coordinate.
	Package string
	// Argv is the full install command, argv[0] included. Note: on Windows,
	// scoop resolves to a .cmd shim; that is acceptable here — unlike profile
	// binaries — because every token is a validated literal, so cmd.exe has
	// nothing to re-parse.
	Argv []string
}

// managerLookPath is a test seam over exec.LookPath.
var managerLookPath = exec.LookPath

// managerCandidate describes one platform's package manager.
type managerCandidate struct {
	id     string
	binary string
	argv   func(pkg string) []string
}

// managerCandidates lists the managers considered per GOOS, in preference
// order (first is the default the UI offers).
func managerCandidates() []managerCandidate {
	switch runtime.GOOS {
	case "windows":
		return []managerCandidate{
			{"winget", "winget", func(pkg string) []string {
				return []string{"winget", "install", "--id", pkg, "-e",
					"--accept-source-agreements", "--accept-package-agreements"}
			}},
			{"scoop", "scoop", func(pkg string) []string {
				return []string{"scoop", "install", pkg}
			}},
		}
	case "darwin":
		return []managerCandidate{
			{"brew", "brew", func(pkg string) []string {
				return []string{"brew", "install", pkg}
			}},
		}
	default:
		return []managerCandidate{
			// apt-get typically needs root; the install job runs unelevated
			// and surfaces the failure output verbatim.
			{"apt", "apt-get", func(pkg string) []string {
				return []string{"apt-get", "install", "-y", pkg}
			}},
			{"dnf", "dnf", func(pkg string) []string {
				return []string{"dnf", "install", "-y", pkg}
			}},
		}
	}
}

// coordinateFor returns the declared package for a manager id.
func coordinateFor(install InstallSpec, id string) string {
	switch id {
	case "winget":
		return install.Winget
	case "scoop":
		return install.Scoop
	case "brew":
		return install.Brew
	case "apt":
		return install.Apt
	case "dnf":
		return install.Dnf
	default:
		return ""
	}
}

// DetectManagers returns the installation options available for this profile
// on this machine, in preference order.
func DetectManagers(spec ProfileSpec) []Manager {
	var managers []Manager
	for _, candidate := range managerCandidates() {
		pkg := coordinateFor(spec.Install, candidate.id)
		if pkg == "" {
			continue
		}
		if _, err := managerLookPath(candidate.binary); err != nil {
			continue
		}
		managers = append(managers, Manager{ID: candidate.id, Package: pkg, Argv: candidate.argv(pkg)})
	}
	return managers
}
