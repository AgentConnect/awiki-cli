package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/update"
)

func TestBuildUpgradeStatusWhenCheckUnavailable(t *testing.T) {
	t.Parallel()

	decision := update.Decision{
		CurrentVersion:  "0.0.1-beta.20",
		StrictDisabled:  false,
		DevBuild:        false,
		HasNewerVersion: false,
		Blocked:         false,
	}
	data, summary, warnings := buildUpgradeStatus(decision, errors.New("npm registry responded with status 503"))

	if summary != "Unable to check for awiki-cli updates" {
		t.Fatalf("summary = %q, want %q", summary, "Unable to check for awiki-cli updates")
	}
	if got, _ := data["current_version"].(string); got != "0.0.1-beta.20" {
		t.Fatalf("data[current_version] = %q, want %q", got, "0.0.1-beta.20")
	}
	if got, _ := data["update_check_status"].(string); got != "unavailable" {
		t.Fatalf("data[update_check_status] = %q, want %q", got, "unavailable")
	}
	if got, _ := data["update_check_error"].(string); !strings.Contains(got, "503") {
		t.Fatalf("data[update_check_error] = %q, want substring %q", got, "503")
	}
	if len(warnings) != 2 {
		t.Fatalf("len(warnings) = %d, want 2", len(warnings))
	}
	if !strings.Contains(warnings[0], "503") {
		t.Fatalf("warnings[0] = %q, want substring %q", warnings[0], "503")
	}
}

func TestBuildUpgradeStatusWhenNewerVersionAvailable(t *testing.T) {
	t.Parallel()

	decision := update.Decision{
		CurrentVersion:      "0.0.1-beta.19",
		LatestVersion:       "0.0.1-beta.20",
		MinSupportedVersion: "0.0.1-beta.19",
		StrictDisabled:      false,
		DevBuild:            false,
		HasNewerVersion:     true,
		Blocked:             false,
	}
	data, summary, warnings := buildUpgradeStatus(decision, nil)

	if summary != "A newer awiki-cli version (0.0.1-beta.20) is available" {
		t.Fatalf("summary = %q, want newer-version summary", summary)
	}
	if got, _ := data["update_check_status"].(string); got != "ok" {
		t.Fatalf("data[update_check_status] = %q, want %q", got, "ok")
	}
	if _, ok := data["update_check_error"]; ok {
		t.Fatal("data[update_check_error] present, want absent")
	}
	if len(warnings) != 1 || warnings[0] != "Upgrading is recommended to stay on a supported version." {
		t.Fatalf("warnings = %#v, want single upgrade recommendation", warnings)
	}
}
