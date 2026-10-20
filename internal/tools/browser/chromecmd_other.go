//go:build browser && !linux

package browser

import "os/exec"

// applyChromeCmdDefaults mirrors chromedp's own per-platform setup of the Chrome
// command, which is empty everywhere but Linux (allocate_other.go). See the file
// next to this one for why it is repeated here at all.
func applyChromeCmdDefaults(*exec.Cmd) {}
