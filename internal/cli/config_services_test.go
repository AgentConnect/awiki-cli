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
	"github.com/spf13/cobra"
)

func TestRunConfigServicesShowRendersServicesAndSources(t *testing.T) {
	workspaceHome := t.TempDir()
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)
	if err := os.WriteFile(filepath.Join(workspaceHome, "config.yaml"), []byte(`schema_version: 1
services:
  did_domain: b.example.com
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON)}}
	cmd := configCommandForTest(t, "config services show")

	rendered, err := captureStdout(func() error {
		return app.runConfigServicesShow(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runConfigServicesShow() error = %v", err)
	}
	for _, want := range []string{
		`"did_domain": "b.example.com"`,
		`"service_base_url": "https://b.example.com"`,
		`"anp_service_endpoint": "https://b.example.com/anp-im/rpc"`,
		`"anp_service_did": "did:wba:b.example.com"`,
		`"sources"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered output %q missing %q", rendered, want)
		}
	}
}

func TestRunConfigServicesSetDryRunDoesNotWriteConfig(t *testing.T) {
	workspaceHome := t.TempDir()
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)
	app := &App{globals: GlobalOptions{DryRun: true, Format: string(output.FormatJSON)}}
	cmd := configCommandForTest(t, "config services set")
	setFlag(t, cmd, "domain", "C.Example.COM.")

	rendered, err := captureStdout(func() error {
		return app.runConfigServicesSet(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runConfigServicesSet() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceHome, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("config file exists after dry-run or unexpected stat error: %v", err)
	}
	if !strings.Contains(rendered, `"action": "config_services_set"`) || !strings.Contains(rendered, `"did_domain": "c.example.com"`) {
		t.Fatalf("rendered output %q missing planned services", rendered)
	}
}

func TestRunConfigServicesSetWritesServicesOnly(t *testing.T) {
	workspaceHome := t.TempDir()
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspaceHome)
	if err := os.WriteFile(filepath.Join(workspaceHome, "config.yaml"), []byte(`schema_version: 1
runtime:
  mode: http
services:
  did_domain: old.example.com
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON)}}
	cmd := configCommandForTest(t, "config services set")
	setFlag(t, cmd, "domain", "d.example.com")

	if _, err := captureStdout(func() error {
		return app.runConfigServicesSet(cmd, nil)
	}); err != nil {
		t.Fatalf("runConfigServicesSet() error = %v", err)
	}
	fileConfig, exists, err := appconfig.ReadFileConfig(filepath.Join(workspaceHome, "config.yaml"))
	if err != nil || !exists {
		t.Fatalf("ReadFileConfig() = exists %v, err %v", exists, err)
	}
	if fileConfig.Runtime.Mode != "http" {
		t.Fatalf("runtime mode = %q, want preserved http", fileConfig.Runtime.Mode)
	}
	if fileConfig.Services.DIDDomain != "d.example.com" {
		t.Fatalf("did_domain = %q", fileConfig.Services.DIDDomain)
	}
	if fileConfig.Services.ServiceBaseURL != "https://d.example.com" {
		t.Fatalf("service_base_url = %q", fileConfig.Services.ServiceBaseURL)
	}
}

func TestRunConfigServicesSetRejectsMissingFlags(t *testing.T) {
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", t.TempDir())
	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON)}}
	cmd := configCommandForTest(t, "config services set")

	err := app.runConfigServicesSet(cmd, nil)
	if err == nil {
		t.Fatal("runConfigServicesSet() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "at least one services flag is required") {
		t.Fatalf("error = %q", err)
	}
}

func TestCatalogPublishesConfigServicesCommands(t *testing.T) {
	catalog := cmdmeta.NewCatalog()
	for _, name := range []string{"config services", "config services show", "config services set"} {
		if _, ok := catalog.Lookup(name); !ok {
			t.Fatalf("Lookup(%q) = false, want true", name)
		}
	}
	spec := catalog.MustLookup("config services set")
	for _, flag := range []string{"domain", "service-base-url", "anp-service-endpoint", "anp-service-did"} {
		if !hasFlagSpec(spec.Flags, flag) {
			t.Fatalf("config services set missing flag %q", flag)
		}
	}
}

func configCommandForTest(t *testing.T, name string) *cobra.Command {
	t.Helper()
	cmd := (&App{}).commandFromSpec(cmdmeta.NewCatalog().MustLookup(name))
	cmd.SetContext(context.Background())
	return cmd
}

func hasFlagSpec(flags []cmdmeta.FlagSpec, name string) bool {
	for _, flag := range flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}
