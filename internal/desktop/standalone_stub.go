//go:build !desktop || !windows

package desktop

import "errors"

var ErrStandaloneUnavailable = errors.New("standalone desktop window is unavailable")

func RunStandalone(_, _ string) error { return ErrStandaloneUnavailable }
