package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/spf13/cobra"
)

func TestIdentityMetaFromDataSkipsTypedNilIdentitySummary(t *testing.T) {
	t.Parallel()

	var summary *identity.IdentitySummary
	meta := identityMetaFromData(map[string]any{
		"active_identity": summary,
	})
	if meta != nil {
		t.Fatalf("identityMetaFromData() = %#v, want nil", meta)
	}
}

func TestRunIDReplaceDIDDryRunWarnsAndTargetsIdentity(t *testing.T) {
	initTestWorkspace(t)

	app := &App{
		globals: GlobalOptions{
			DryRun:   true,
			Identity: "alice",
		},
	}
	cmd := &cobra.Command{Use: "replace-did"}
	cmd.Flags().Bool("is-public", false, "")
	cmd.Flags().Bool("is-agent", false, "")
	cmd.Flags().String("role", "", "")
	cmd.Flags().String("endpoint-url", "", "")

	rendered, err := captureStdout(func() error {
		return app.runIDReplaceDID(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runIDReplaceDID(--dry-run) error = %v", err)
	}
	for _, want := range []string{
		`"action": "replace_did"`,
		`"identity_name": "alice"`,
		`"dangerous": true`,
		"generated_e1_document",
		"did-auth.replace_did",
		".legacy-backup/replace-did",
		"sqlite.owner_did_rebind",
		"Dangerous command: replace-did creates a new e1 DID",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dry-run output %q missing %q", rendered, want)
		}
	}
}

func TestRunIDRecoverDryRunUsesHandleAndWarnsWhenIdentityFlagIsIgnored(t *testing.T) {
	initTestWorkspace(t)

	app := &App{
		globals: GlobalOptions{
			DryRun:          true,
			Identity:        "ignored-name",
			IdentityChanged: true,
		},
	}
	cmd := &cobra.Command{Use: "recover"}
	cmd.Flags().String("handle", "", "")
	cmd.Flags().String("phone", "", "")
	cmd.Flags().String("otp", "", "")
	if err := cmd.Flags().Set("handle", "zhuocheng"); err != nil {
		t.Fatalf("Set(handle) error = %v", err)
	}
	if err := cmd.Flags().Set("phone", "13800138000"); err != nil {
		t.Fatalf("Set(phone) error = %v", err)
	}
	if err := cmd.Flags().Set("otp", "123456"); err != nil {
		t.Fatalf("Set(otp) error = %v", err)
	}

	rendered, err := captureStdout(func() error {
		return app.runIDRecover(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runIDRecover(--dry-run) error = %v", err)
	}
	for _, want := range []string{
		`"target_handle": "zhuocheng"`,
		`"final_identity_name": "zhuocheng"`,
		`"same_handle_candidates": []`,
		`"excluded_identities": []`,
		`"backup_path":`,
		`"did-auth.recover_handle"`,
		recoverIdentityIgnoredWarning,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dry-run output %q missing %q", rendered, want)
		}
	}
}

func TestRunIDRefreshTokenDryRunPlansDidAuthRefresh(t *testing.T) {
	initTestWorkspace(t)

	app := &App{
		globals: GlobalOptions{
			DryRun:   true,
			Identity: "alice",
		},
	}
	cmd := &cobra.Command{Use: "refresh-token"}

	rendered, err := captureStdout(func() error {
		return app.runIDRefreshToken(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runIDRefreshToken(--dry-run) error = %v", err)
	}
	for _, want := range []string{
		`"action": "refresh_token"`,
		`"identity_name": "alice"`,
		`"did-auth.get_me"`,
		`"auth.json"`,
		`"auth_flow": "did_auth_get_me_without_stored_bearer"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dry-run output %q missing %q", rendered, want)
		}
	}
}

func TestRunIDRecoverWithoutOTPReturnsSendOTPSuccess(t *testing.T) {
	initTestWorkspace(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-service/handle/rpc" {
			t.Fatalf("r.URL.Path = %q, want /user-service/handle/rpc", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if got, _ := payload["method"].(string); got != "send_otp" {
			t.Fatalf("rpc method = %q, want send_otp", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"message":"OTP sent successfully"},"id":"req-1"}`))
	}))
	defer server.Close()

	workspaceHome := os.Getenv("AWIKI_CLI_WORKSPACE_HOME_DIR")
	configPath := filepath.Join(workspaceHome, "config.yaml")
	fileConfig := appconfig.FileConfig{}
	fileConfig.Services.ServiceBaseURL = server.URL
	fileConfig.Services.DIDDomain = "awiki.test"
	if err := appconfig.WriteFileConfig(configPath, fileConfig); err != nil {
		t.Fatalf("WriteFileConfig() error = %v", err)
	}

	app := &App{
		globals: GlobalOptions{
			Format: "json",
		},
	}
	cmd := &cobra.Command{Use: "recover"}
	cmd.Flags().String("handle", "", "")
	cmd.Flags().String("phone", "", "")
	cmd.Flags().String("otp", "", "")
	if err := cmd.Flags().Set("handle", "zhuocheng"); err != nil {
		t.Fatalf("Set(handle) error = %v", err)
	}
	if err := cmd.Flags().Set("phone", "13800138000"); err != nil {
		t.Fatalf("Set(phone) error = %v", err)
	}

	rendered, err := captureStdout(func() error {
		return app.runIDRecover(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runIDRecover() error = %v", err)
	}
	for _, want := range []string{
		`"action": "send_recover_otp"`,
		`"verification_state": "otp_sent"`,
		`"identity_name": "zhuocheng"`,
		`"summary": "OTP sent for handle zhuocheng recovery"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("recover send-otp output %q missing %q", rendered, want)
		}
	}
}
