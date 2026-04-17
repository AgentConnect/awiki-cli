package listener

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/upgrader"
)

const upgradeProcessingTimeout = 10 * time.Minute

func (s *Supervisor) handleUpgradeNotification(session *session, notification map[string]any) bool {
	event, ok := ParseUpgradeEvent(notification)
	if !ok {
		return false
	}
	go func() {
		ctx, cancel := context.WithTimeout(session.ctx, upgradeProcessingTimeout)
		defer cancel()
		_ = s.processUpgradeEvent(ctx, event)
	}()
	return true
}

func (s *Supervisor) processUpgradeEvent(ctx context.Context, event UpgradeEvent) error {
	manager, err := upgrader.NewManager(s.resolved)
	if err != nil {
		return err
	}
	if !s.resolved.UpdateAllowWSPushTrigger {
		_, err := manager.RecordWSPushObservation(event.Decision)
		return err
	}
	mode := normalizeAutoUpgradeMode(s.resolved.UpdateAutoUpgradeEnabled, s.resolved.UpdateAutoUpgradeMode)
	state, err := manager.RecordWSPush(event.Decision)
	if err != nil {
		return err
	}
	if mode == "notify" {
		return nil
	}
	if !event.ValidApply {
		_, _ = upgrader.RecordFailure(manager.Paths().ReleaseStatePath, strings.TrimSpace(event.Decision.LatestVersion), "ws_event", "upgrade event is missing installable artifact metadata")
		return fmt.Errorf("upgrade event is missing installable artifact metadata")
	}
	if alreadyApplied(state, event.Decision.LatestVersion) {
		return nil
	}
	switch mode {
	case "predownload":
		_, err = manager.Predownload(ctx, event.Decision)
		return err
	case "apply":
		_, err = manager.Apply(ctx, event.Decision)
		return err
	default:
		return nil
	}
}

func alreadyApplied(state *upgrader.ReleaseState, version string) bool {
	if state == nil {
		return false
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}
	if strings.TrimSpace(state.CurrentVersion) == version {
		return true
	}
	if strings.TrimSpace(state.LastAppliedVersion) == version {
		return true
	}
	return false
}
