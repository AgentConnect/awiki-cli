package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/update"
	"github.com/spf13/cobra"
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

func TestBuildUpgradeStatusIncludesNpmmirrorFallbackHint(t *testing.T) {
	t.Parallel()

	data, _, _ := buildUpgradeStatus(update.Decision{}, nil)

	got, _ := data["upgrade_hint"].(string)
	if !strings.Contains(got, directNpmInstallCommand()) {
		t.Fatalf("upgrade_hint = %q, want direct install command %q", got, directNpmInstallCommand())
	}
	if !strings.Contains(got, mirrorNpmInstallCommand()) {
		t.Fatalf("upgrade_hint = %q, want mirror install command %q", got, mirrorNpmInstallCommand())
	}
}

func TestBuildUpgradeStatusWhenUsingStaleCache(t *testing.T) {
	t.Parallel()

	decision := update.Decision{
		CurrentVersion:      "1.0.9",
		LatestVersion:       "1.0.10",
		MinSupportedVersion: "1.0.9",
		MetadataSource:      "cache_stale",
		HasNewerVersion:     true,
	}
	data, _, warnings := buildUpgradeStatus(decision, nil)

	if got, _ := data["update_check_status"].(string); got != "stale_cache" {
		t.Fatalf("data[update_check_status] = %q, want %q", got, "stale_cache")
	}
	if got, _ := data["update_metadata_source"].(string); got != "cache_stale" {
		t.Fatalf("data[update_metadata_source] = %q, want %q", got, "cache_stale")
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v, want 2 warnings", warnings)
	}
	if !strings.Contains(warnings[1], "cached update metadata") {
		t.Fatalf("warnings[1] = %q, want cached-metadata warning", warnings[1])
	}
}

func TestRunNpmGlobalInstallFallsBackToNpmmirror(t *testing.T) {
	original := runNpmInstallAttempt
	t.Cleanup(func() {
		runNpmInstallAttempt = original
	})

	var attempts []string
	runNpmInstallAttempt = func(ctx context.Context, stdout, stderr io.Writer, args []string) error {
		attempts = append(attempts, formatNpmInstallCommand(args))
		if len(attempts) == 1 {
			return errors.New("dial tcp timeout")
		}
		return nil
	}

	cmd := &cobra.Command{Use: "upgrade"}
	cmd.SetContext(context.Background())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := runNpmGlobalInstall(cmd); err != nil {
		t.Fatalf("runNpmGlobalInstall() error = %v", err)
	}

	wantAttempts := []string{
		directNpmInstallCommand(),
		mirrorNpmInstallCommand(),
	}
	if !reflect.DeepEqual(attempts, wantAttempts) {
		t.Fatalf("attempts = %#v, want %#v", attempts, wantAttempts)
	}
	if !strings.Contains(stderr.String(), npmMirrorRegistryURL) {
		t.Fatalf("stderr = %q, want retry message mentioning %s", stderr.String(), npmMirrorRegistryURL)
	}
}

func TestRunNpmGlobalInstallReturnsCombinedError(t *testing.T) {
	original := runNpmInstallAttempt
	t.Cleanup(func() {
		runNpmInstallAttempt = original
	})

	runNpmInstallAttempt = func(ctx context.Context, stdout, stderr io.Writer, args []string) error {
		if strings.Contains(formatNpmInstallCommand(args), "--registry=") {
			return errors.New("mirror registry timeout")
		}
		return errors.New("primary registry timeout")
	}

	cmd := &cobra.Command{Use: "upgrade"}
	cmd.SetContext(context.Background())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := runNpmGlobalInstall(cmd)
	if err == nil {
		t.Fatal("runNpmGlobalInstall() error = nil, want combined failure")
	}
	if !strings.Contains(err.Error(), directNpmInstallCommand()) {
		t.Fatalf("error = %q, want direct install command %q", err.Error(), directNpmInstallCommand())
	}
	if !strings.Contains(err.Error(), mirrorNpmInstallCommand()) {
		t.Fatalf("error = %q, want mirror install command %q", err.Error(), mirrorNpmInstallCommand())
	}
}
