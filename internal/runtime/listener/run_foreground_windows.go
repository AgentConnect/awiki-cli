//go:build windows

package listener

import (
	"context"
	"os"
	"os/signal"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func runForegroundSignals(_ *appconfig.Resolved, supervisor *Supervisor) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return supervisor.Run(ctx)
}
