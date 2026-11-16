//go:build !windows

package platform

import (
	"fmt"
	"os/exec"
	"runtime"
)

func revealFile(path string) error {
	argv, err := revealFileArgv(runtime.GOOS, path)
	if err != nil {
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...) // #nosec G204 -- fixed platform command and verified artifact source
	AdaptCommand(cmd)
	// This command intentionally opens a desktop window.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start file reveal: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
