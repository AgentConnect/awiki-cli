package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/output"
	listenerrt "github.com/agentconnect/awiki-cli/internal/runtime/listener"
	"github.com/agentconnect/awiki-cli/internal/upgrade"
	"github.com/spf13/cobra"
)

func TestRunInitDryRunRendersWorkspacePlan(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspace)

	app := &App{globals: GlobalOptions{DryRun: true, Format: string(output.FormatJSON)}}
	cmd := &cobra.Command{Use: "init"}

	rendered, err := captureStdout(func() error {
		return app.runInit(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}
	if !strings.Contains(rendered, `"action": "init_workspace"`) {
		t.Fatalf("rendered output %q missing init action", rendered)
	}
	if !strings.Contains(rendered, workspace) {
		t.Fatalf("rendered output %q missing workspace path", rendered)
	}
}

func TestRunInitAutoMigratesLegacyOnlyWorkspaceBeforeInitialization(t *testing.T) {
	workspaceHome, configPath := initTestWorkspace(t)

	originalInspect := initUpgradeInspectFunc
	originalUpgrade := initUpgradeIfNeededFunc
	originalApplyRuntimePolicy := initApplyRuntimePolicyFunc
	t.Cleanup(func() {
		initUpgradeInspectFunc = originalInspect
		initUpgradeIfNeededFunc = originalUpgrade
		initApplyRuntimePolicyFunc = originalApplyRuntimePolicy
	})

	upgradeCalled := false
	initUpgradeInspectFunc = func(context.Context, *appconfig.Resolved) (*upgrade.Inspection, error) {
		return &upgrade.Inspection{
			Detection: upgrade.Detection{
				HasWorkspace: false,
				HasLegacy:    true,
			},
		}, nil
	}
	initUpgradeIfNeededFunc = func(_ context.Context, _ *appconfig.Resolved) error {
		upgradeCalled = true
		if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
			return err
		}
		return os.WriteFile(configPath, []byte(`schema_version: 1
runtime:
  mode: http
services:
  service_base_url: https://legacy.awiki.test
  did_domain: legacy.awiki.test
`), 0o600)
	}
	initApplyRuntimePolicyFunc = func(*appconfig.Resolved) (listenerrt.Status, error) {
		return listenerrt.Status{}, nil
	}

	app := &App{}
	if _, err := captureStdout(func() error {
		return app.runInit(&cobra.Command{Use: "init"}, nil)
	}); err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	if !upgradeCalled {
		t.Fatal("initUpgradeIfNeededFunc was not called for a legacy-only workspace")
	}

	fileConfig, exists, err := appconfig.ReadFileConfig(configPath)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if !exists {
		t.Fatalf("expected migrated config file to exist at %s", configPath)
	}
	if fileConfig.Runtime.Mode != "http" {
		t.Fatalf("runtime mode = %q, want http", fileConfig.Runtime.Mode)
	}
	if fileConfig.Services.ServiceBaseURL != "https://legacy.awiki.test" {
		t.Fatalf("service base url = %q, want migrated legacy value", fileConfig.Services.ServiceBaseURL)
	}
	if fileConfig.Services.DIDDomain != "legacy.awiki.test" {
		t.Fatalf("did domain = %q, want migrated legacy value", fileConfig.Services.DIDDomain)
	}

	if _, err := os.Stat(filepath.Join(workspaceHome, "data", "awiki-cli.db")); err != nil {
		t.Fatalf("expected init to preserve normal database initialization after migration, stat error = %v", err)
	}
}

func TestRunInitSkipsAutoMigrationWhenWorkspaceAlreadyExists(t *testing.T) {
	_, configPath := initTestWorkspace(t)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`schema_version: 1
runtime:
  mode: http
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	originalInspect := initUpgradeInspectFunc
	originalUpgrade := initUpgradeIfNeededFunc
	originalApplyRuntimePolicy := initApplyRuntimePolicyFunc
	t.Cleanup(func() {
		initUpgradeInspectFunc = originalInspect
		initUpgradeIfNeededFunc = originalUpgrade
		initApplyRuntimePolicyFunc = originalApplyRuntimePolicy
	})

	upgradeCalled := false
	initUpgradeInspectFunc = func(context.Context, *appconfig.Resolved) (*upgrade.Inspection, error) {
		return &upgrade.Inspection{
			Detection: upgrade.Detection{
				HasWorkspace: true,
				HasLegacy:    true,
			},
		}, nil
	}
	initUpgradeIfNeededFunc = func(context.Context, *appconfig.Resolved) error {
		upgradeCalled = true
		return nil
	}
	initApplyRuntimePolicyFunc = func(*appconfig.Resolved) (listenerrt.Status, error) {
		return listenerrt.Status{}, nil
	}

	app := &App{}
	if _, err := captureStdout(func() error {
		return app.runInit(&cobra.Command{Use: "init"}, nil)
	}); err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	if upgradeCalled {
		t.Fatal("initUpgradeIfNeededFunc was called even though canonical workspace already exists")
	}
}

func TestRunInitDryRunSkipsAutoMigration(t *testing.T) {
	initTestWorkspace(t)

	originalInspect := initUpgradeInspectFunc
	originalUpgrade := initUpgradeIfNeededFunc
	t.Cleanup(func() {
		initUpgradeInspectFunc = originalInspect
		initUpgradeIfNeededFunc = originalUpgrade
	})

	inspectCalled := false
	upgradeCalled := false
	initUpgradeInspectFunc = func(context.Context, *appconfig.Resolved) (*upgrade.Inspection, error) {
		inspectCalled = true
		return nil, nil
	}
	initUpgradeIfNeededFunc = func(context.Context, *appconfig.Resolved) error {
		upgradeCalled = true
		return nil
	}

	app := &App{
		globals: GlobalOptions{
			DryRun: true,
		},
	}
	if _, err := captureStdout(func() error {
		return app.runInit(&cobra.Command{Use: "init"}, nil)
	}); err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	if inspectCalled {
		t.Fatal("initUpgradeInspectFunc was called during dry-run")
	}
	if upgradeCalled {
		t.Fatal("initUpgradeIfNeededFunc was called during dry-run")
	}
}

func initTestWorkspace(t *testing.T) (string, string) {
	t.Helper()

	homeDir := t.TempDir()
	workspaceHome := filepath.Join(homeDir, ".awiki-cli")
	t.Setenv("HOME", homeDir)
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)

	return workspaceHome, filepath.Join(workspaceHome, "config.yaml")
}
