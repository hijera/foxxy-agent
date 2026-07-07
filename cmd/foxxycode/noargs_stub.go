//go:build !desktop

package main

import (
	"fmt"
	"os"
)

func runDesktop([]string) error {
	return fmt.Errorf("desktop support is not built in (rebuild with: go build -tags=\"http,ui,desktop\")")
}

func defaultRun() error {
	printUsage(os.Stderr)
	return fmt.Errorf("usage shown above")
}
