//go:build !http

package main

import "fmt"

// httpAvailable reports whether `foxxycode http` is compiled in. Specs gate their
// @http scenarios on it instead of failing where the surface does not exist.
const httpAvailable = false

func runHTTP([]string) error {
	return fmt.Errorf("http support is not built in (rebuild with: go build -tags=http)")
}
