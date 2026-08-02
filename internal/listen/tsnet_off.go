//go:build notsnet

package listen

import (
	"context"
	"errors"
)

func addTailscale(_ context.Context, _ Config, _ *Set) error {
	return errors.New("Tailscale support was disabled at build time with -tags notsnet")
}
