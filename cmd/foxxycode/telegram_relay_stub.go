//go:build !gateway && !gateway.telegram

package main

func runTelegramRelayWorker() (bool, error) { return false, nil }
