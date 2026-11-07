//go:build gateway || gateway.telegram

package main

import (
	"context"
	"os"

	"github.com/hijera/foxxycode-agent/external/gateway/telegram/relay"
)

func runTelegramRelayWorker() (bool, error) {
	dir := os.Getenv(relay.WorkerEnv)
	if dir == "" {
		return false, nil
	}
	return true, relay.RunWorker(context.Background(), dir)
}
