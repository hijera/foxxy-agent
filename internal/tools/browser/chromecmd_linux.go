//go:build browser && linux

package browser

import (
	"os"
	"os/exec"
	"syscall"
)

// applyChromeCmdDefaults mirrors what chromedp applies to the Chrome command
// itself (allocate_linux.go). It has to be repeated here because chromedp's own
// setup is the else branch of ModifyCmdFunc: passing one to learn Chrome's pid
// replaces that setup rather than running alongside it, and silently dropping
// the safety net below is not a trade worth making for a pid.
func applyChromeCmdDefaults(cmd *exec.Cmd) {
	if _, ok := os.LookupEnv("LAMBDA_TASK_ROOT"); ok {
		// Nothing to do on AWS Lambda, where the signal is not delivered.
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = new(syscall.SysProcAttr)
	}
	// When foxxycode dies, Chrome dies with it rather than outliving the agent
	// that launched it.
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
