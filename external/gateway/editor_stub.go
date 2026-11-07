//go:build !gateway && !gateway.telegram

package gateway

import (
	"context"
	"errors"
)

// ServeEditor reports that the optional adapter is absent in this build.
func ServeEditor(_ context.Context, _ string, _ Options) error {
	return errors.New("editor Telegram relay requires a build with -tags gateway")
}
