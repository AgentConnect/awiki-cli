package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestConfigSetDryRunReturnsPlanWithoutWrites(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspace)

	exitCode, stdout, stderr := executeRootCommand(t, "--dry-run", "config", "set", "--did-domain", "Tenant.Example.")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stdout=%q stderr=%q", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	envelope := decodeSuccessEnvelope(t, stdout)
	if envelope.Command != "awiki-cli config set" {
		t.Fatalf("command = %q, want awiki-cli config set", envelope.Command)
	}
	if envelope.Summary != "Dry run: DID domain update planned" {
		t.Fatalf("summary = %q, want dry-run summary", envelope.Summary)
	}
	plan := mustMap(t, envelope.Data["plan"], "data.plan")
	if plan["action"] != "config_set_did_domain" {
		t.Fatalf("plan.action = %#v, want config_set_did_domain", plan["action"])
	}
	if plan["did_domain"] != "tenant.example" {
		t.Fatalf("plan.did_domain = %#v, want tenant.example", plan["did_domain"])
	}
	if plan["config_file"] != filepath.Join(workspace, "config.yaml") {
		t.Fatalf("plan.config_file = %#v, want %q", plan["config_file"], filepath.Join(workspace, "config.yaml"))
	}
	if _, err := os.Stat(filepath.Join(workspace, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("config.yaml stat error = %v, want not exist", err)
	}
}

func TestConfigSetWritesNormalizedDIDDomainWithoutRuntimeArtifacts(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspace)

	exitCode, stdout, stderr := executeRootCommand(t, "config", "set", "--did-domain", " Tenant.Example. ")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stdout=%q stderr=%q", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	envelope := decodeSuccessEnvelope(t, stdout)
	if envelope.Summary != "DID domain updated" {
		t.Fatalf("summary = %q, want DID domain updated", envelope.Summary)
	}
	if envelope.Data["did_domain"] != "tenant.example" {
		t.Fatalf("data.did_domain = %#v, want tenant.example", envelope.Data["did_domain"])
	}
	fileConfig, exists, err := appconfig.ReadFileConfig(filepath.Join(workspace, "config.yaml"))
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if !exists {
		t.Fatal("expected config.yaml to exist")
	}
	if fileConfig.Services.DIDDomain != "tenant.example" {
		t.Fatalf("did_domain = %q, want tenant.example", fileConfig.Services.DIDDomain)
	}
	if _, err := os.Stat(filepath.Join(workspace, "runtime", "message-daemon.sock")); !os.IsNotExist(err) {
		t.Fatalf("socket stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "runtime", "listener.service.pid")); !os.IsNotExist(err) {
		t.Fatalf("pid stat error = %v, want not exist", err)
	}
}

func TestConfigSetRequiresDidDomainFlag(t *testing.T) {
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", t.TempDir())

	exitCode, stdout, stderr := executeRootCommand(t, "config", "set")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2; stdout=%q stderr=%q", exitCode, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	envelope := decodeErrorEnvelope(t, stderr)
	if envelope.Error.Code != "invalid_argument" {
		t.Fatalf("error.code = %q, want invalid_argument", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, "config set requires --did-domain") {
		t.Fatalf("error.message = %q, want missing flag guidance", envelope.Error.Message)
	}
}

func TestConfigSetRejectsURLLikeDidDomain(t *testing.T) {
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", t.TempDir())

	exitCode, stdout, stderr := executeRootCommand(t, "config", "set", "--did-domain", "https://tenant.example")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2; stdout=%q stderr=%q", exitCode, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	envelope := decodeErrorEnvelope(t, stderr)
	if envelope.Error.Code != "invalid_argument" {
		t.Fatalf("error.code = %q, want invalid_argument", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, "bare domain") {
		t.Fatalf("error.message = %q, want bare-domain validation", envelope.Error.Message)
	}
}
