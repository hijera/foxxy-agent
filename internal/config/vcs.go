package config

import "strings"

// SVNDefaultTimeoutSeconds bounds a single svn invocation when
// vcs.svn.timeout_seconds is unset.
const SVNDefaultTimeoutSeconds = 120

// VCSConfig is the YAML vcs section (key vcs). Git needs no configuration - it
// is always available when the binary is installed - so the section currently
// carries the Subversion settings only.
type VCSConfig struct {
	SVN SVNConfig `yaml:"svn"`
}

// SVNConfig configures Subversion support: working copy detection for the
// composer chips and the svn_* tools.
type SVNConfig struct {
	// Enabled turns Subversion support on. Defaults to true when unset; turning
	// it off hides the SVN chip and removes every svn_* tool from the model.
	Enabled *bool `yaml:"enabled"`
	// Binary is an explicit path to the svn client. Empty resolves "svn" on PATH,
	// which covers most installs; set it when the client ships outside PATH.
	Binary string `yaml:"binary"`
	// TimeoutSeconds bounds each svn invocation. Defaults to SVNDefaultTimeoutSeconds.
	TimeoutSeconds int `yaml:"timeout_seconds"`
	// BranchLookup allows listing repository branches for the chip menu. This
	// contacts the server, so it can be turned off on slow links. Default true.
	BranchLookup *bool `yaml:"branch_lookup"`
}

// SVNEnabled reports whether Subversion support is on (default true).
func (c *SVNConfig) SVNEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// BranchLookupEnabled reports whether repository branch listing is allowed (default true).
func (c *SVNConfig) BranchLookupEnabled() bool {
	return c.BranchLookup == nil || *c.BranchLookup
}

// ResolvedTimeoutSeconds returns TimeoutSeconds with a safe default.
func (c *SVNConfig) ResolvedTimeoutSeconds() int {
	if c.TimeoutSeconds <= 0 {
		return SVNDefaultTimeoutSeconds
	}
	return c.TimeoutSeconds
}

// ApplyDefaults fills unset fields with defaults.
func (c *VCSConfig) ApplyDefaults() {
	if c.SVN.TimeoutSeconds <= 0 {
		c.SVN.TimeoutSeconds = SVNDefaultTimeoutSeconds
	}
}

// Validate trims the configured client path.
func (c *VCSConfig) Validate() error {
	c.SVN.Binary = strings.TrimSpace(c.SVN.Binary)
	return nil
}
