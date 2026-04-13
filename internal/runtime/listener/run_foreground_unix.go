//go:build !windows

package listener

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func runForegroundSignals(_ *appconfig.Resolved, supervisor *Supervisor) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return supervisor.Run(ctx)
}
