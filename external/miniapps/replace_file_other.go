//go:build miniapps && !windows

package miniapps

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
