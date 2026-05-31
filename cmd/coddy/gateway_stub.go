//go:build !(gateway || gateway.telegram)

package main

import "fmt"

func runGateway(_ []string) error {
	return fmt.Errorf("gateway support not compiled; rebuild with -tags gateway.telegram (Telegram) or -tags gateway (all adapters)")
}
