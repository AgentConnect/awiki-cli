package cli

import (
	"strings"
	"testing"

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
