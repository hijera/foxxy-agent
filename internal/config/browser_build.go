//go:build browser

package config

// BrowserToolCompiled reports whether this binary was built with the "browser"
// build tag, i.e. whether the interactive browser tools exist in it at all.
//
// browser.enabled is a runtime switch on top of this: without the tag the tools
// are never registered, so the switch has nothing to turn on. Surfaces that let a
// user edit the setting read this to say so instead of offering a dead toggle.
const BrowserToolCompiled = true

// BrowserBuildTag is the tag name to quote back to the user.
const BrowserBuildTag = "browser"
