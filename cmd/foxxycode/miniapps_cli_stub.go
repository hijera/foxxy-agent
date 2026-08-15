//go:build !miniapps

package main

func runMiniAppsCommand(_ []string) (bool, error) { return false, nil }
func runEmbeddedMiniApp(_ []string) (bool, error) { return false, nil }
func miniAppsUsage() string                       { return "" }
func miniAppsHomeDirs() []string                  { return nil }
