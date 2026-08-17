//go:build !windows && !darwin

package cmd

import (
	"context"
	"errors"

	"github.com/loom/loom/api"
)

func startApp(ctx context.Context, client *api.Client) error {
	return errors.New("could not connect to loom server, run 'loom serve' to start it")
}
