//go:build !desktop || !windows

package desktop

import "fmt"

// Options configures the desktop launcher (stub).
type Options struct {
	Args []string
}

// Run is unavailable unless built with -tags desktop on GOOS=windows.
func Run(_ Options) error {
	return fmt.Errorf("desktop mode requires rebuild with: go build -tags=desktop (Windows only)")
}
