package listener

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
)

func TestParseUpgradeEventParsesAvailableNotification(t *testing.T) {
	originalVersion := buildinfo.Version
	defer func() { buildinfo.Version = originalVersion }()
	buildinfo.Version = "1.8.0"

	event, ok := ParseUpgradeEvent(map[string]any{
		"method": upgradeMethodAvailable,
		"params": map[string]any{
			"latest_version":        "1.8.1",
			"min_supported_version": "1.8.0",
			"channel":               "stable",
			"artifact": map[string]any{
				"url":    "https://downloads.example.com/awiki-cli.tar.gz",
				"sha256": "abc123",
			},
			"skill_bundle": map[string]any{
				"bundle_version":    "2026.04.17",
				"bundle_sha256":     "bundle-sha",
				"root_skill_sha256": "root-sha",
			},
		},
	})
	if !ok {
		t.Fatal("ParseUpgradeEvent() ok = false, want true")
	}
	if !event.ValidApply {
		t.Fatal("event.ValidApply = false, want true")
	}
	if !event.Decision.HasNewerVersion {
		t.Fatal("event.Decision.HasNewerVersion = false, want true")
	}
	if event.Decision.Source != "ws_push" {
		t.Fatalf("event.Decision.Source = %q, want ws_push", event.Decision.Source)
	}
}

func TestNormalizeAutoUpgradeMode(t *testing.T) {
	if got := normalizeAutoUpgradeMode(false, "apply"); got != "notify" {
		t.Fatalf("normalizeAutoUpgradeMode(false, apply) = %q, want notify", got)
	}
	if got := normalizeAutoUpgradeMode(true, "predownload"); got != "predownload" {
		t.Fatalf("normalizeAutoUpgradeMode(true, predownload) = %q, want predownload", got)
	}
	if got := normalizeAutoUpgradeMode(true, ""); got != "apply" {
		t.Fatalf("normalizeAutoUpgradeMode(true, empty) = %q, want apply", got)
	}
}
