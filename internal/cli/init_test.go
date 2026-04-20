package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
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

func TestRunInitDryRunWithDomainRendersPlannedServicesWithoutWritingConfig(t *testing.T) {
	workspaceHome, configPath := initTestWorkspace(t)
	app := &App{globals: GlobalOptions{DryRun: true, Format: string(output.FormatJSON)}}
	cmd := initCommandForTest(t)
	setFlag(t, cmd, "domain", "A.Example.COM.")

	rendered, err := captureStdout(func() error {
		return app.runInit(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config file exists after dry-run or unexpected stat error: %v", err)
	}
	for _, want := range []string{
		`"did_domain": "a.example.com"`,
		`"service_base_url": "https://a.example.com"`,
		`"anp_service_endpoint": "https://a.example.com/anp-im/rpc"`,
		`"anp_service_did": "did:wba:a.example.com"`,
		workspaceHome,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered output %q missing %q", rendered, want)
		}
	}
}

func TestRunInitWithDomainWritesServicesConfig(t *testing.T) {
	_, configPath := initTestWorkspace(t)
	originalInspect := initUpgradeInspectFunc
	originalApplyRuntimePolicy := initApplyRuntimePolicyFunc
	t.Cleanup(func() {
		initUpgradeInspectFunc = originalInspect
		initApplyRuntimePolicyFunc = originalApplyRuntimePolicy
	})
	initUpgradeInspectFunc = func(context.Context, *appconfig.Resolved) (*upgrade.Inspection, error) {
		return &upgrade.Inspection{Detection: upgrade.Detection{HasWorkspace: true}}, nil
	}
	initApplyRuntimePolicyFunc = func(*appconfig.Resolved) (listenerrt.Status, error) {
		return listenerrt.Status{}, nil
	}

	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON)}}
	cmd := initCommandForTest(t)
	setFlag(t, cmd, "domain", "b.example.com")

	if _, err := captureStdout(func() error {
		return app.runInit(cmd, nil)
	}); err != nil {
		t.Fatalf("runInit() error = %v", err)
	}
	fileConfig, exists, err := appconfig.ReadFileConfig(configPath)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if !exists {
		t.Fatal("config file does not exist")
	}
	if fileConfig.Services.ServiceBaseURL != "https://b.example.com" {
		t.Fatalf("service_base_url = %q", fileConfig.Services.ServiceBaseURL)
	}
	if fileConfig.Services.DIDDomain != "b.example.com" {
		t.Fatalf("did_domain = %q", fileConfig.Services.DIDDomain)
	}
	if fileConfig.Services.ANPServiceEndpoint != "https://b.example.com/anp-im/rpc" {
		t.Fatalf("anp_service_endpoint = %q", fileConfig.Services.ANPServiceEndpoint)
	}
	if fileConfig.Services.ANPServiceDID != "did:wba:b.example.com" {
		t.Fatalf("anp_service_did = %q", fileConfig.Services.ANPServiceDID)
	}
}

func TestRunInitWithAdvancedServiceOverridesWritesExplicitServices(t *testing.T) {
	_, configPath := initTestWorkspace(t)
	originalInspect := initUpgradeInspectFunc
	originalApplyRuntimePolicy := initApplyRuntimePolicyFunc
	t.Cleanup(func() {
		initUpgradeInspectFunc = originalInspect
		initApplyRuntimePolicyFunc = originalApplyRuntimePolicy
	})
	initUpgradeInspectFunc = func(context.Context, *appconfig.Resolved) (*upgrade.Inspection, error) {
		return &upgrade.Inspection{Detection: upgrade.Detection{HasWorkspace: true}}, nil
	}
	initApplyRuntimePolicyFunc = func(*appconfig.Resolved) (listenerrt.Status, error) {
		return listenerrt.Status{}, nil
	}

	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON)}}
	cmd := initCommandForTest(t)
	setFlag(t, cmd, "domain", "a.example.com")
	setFlag(t, cmd, "service-base-url", "https://api.a.example.com/")
	setFlag(t, cmd, "anp-service-endpoint", "https://gateway.a.example.com/anp-im/rpc")
	setFlag(t, cmd, "anp-service-did", "did:wba:service.a.example.com")

	rendered, err := captureStdout(func() error {
		return app.runInit(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}
	if !strings.Contains(rendered, "anp_service_endpoint host differs from services.did_domain") {
		t.Fatalf("rendered output %q missing advanced endpoint warning", rendered)
	}
	fileConfig, exists, err := appconfig.ReadFileConfig(configPath)
	if err != nil || !exists {
		t.Fatalf("ReadFileConfig() = exists %v, err %v", exists, err)
	}
	if fileConfig.Services.ServiceBaseURL != "https://api.a.example.com" {
		t.Fatalf("service_base_url = %q", fileConfig.Services.ServiceBaseURL)
	}
	if fileConfig.Services.ANPServiceEndpoint != "https://gateway.a.example.com/anp-im/rpc" {
		t.Fatalf("anp_service_endpoint = %q", fileConfig.Services.ANPServiceEndpoint)
	}
	if fileConfig.Services.ANPServiceDID != "did:wba:service.a.example.com" {
		t.Fatalf("anp_service_did = %q", fileConfig.Services.ANPServiceDID)
	}
}

func initCommandForTest(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := (&App{}).commandFromSpec(cmdmeta.NewCatalog().MustLookup("init"))
	cmd.SetArgs(nil)
	return cmd
}

func setFlag(t *testing.T, cmd *cobra.Command, name string, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("Flags().Set(%s) error = %v", name, err)
	}
}
