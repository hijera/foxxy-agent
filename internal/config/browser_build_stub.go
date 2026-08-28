//go:build !browser

package config

// BrowserToolCompiled is false in a build without the "browser" tag: the
// interactive browser tools are not in this binary, so browser.enabled cannot
// turn them on. See browser_build.go for the compiled-in counterpart.
const BrowserToolCompiled = false

// BrowserBuildTag is the tag name to quote back to the user.
const BrowserBuildTag = "browser"
