package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
)

func TestRenderSuccessSanitizesInternalIdentityFields(t *testing.T) {
	app := &App{}
	rendered, err := captureStdout(func() error {
		return app.renderSuccess(
			"awiki-cli config show",
			output.FormatJSON,
			"",
			map[string]any{
				"default_identity": map[string]any{
					"handle":  "alice",
					"user_id": "user-123",
				},
			},
			"Resolved configuration",
			nil,
			nil,
		)
	})
	if err != nil {
		t.Fatalf("captureStdout(renderSuccess) error = %v", err)
	}
	if strings.Contains(rendered, "user_id") || strings.Contains(rendered, "user-123") {
		t.Fatalf("rendered output %q still contains internal user_id fields", rendered)
	}
	if !strings.Contains(rendered, "alice") {
		t.Fatalf("rendered output %q lost public fields", rendered)
	}
}

func TestIdentityGatingUsesFrozenErrorCode(t *testing.T) {
	t.Parallel()

	app := &App{}
	err := identity.UserRegistrationError("alice", identity.UserState{
		RegistrationState: "local_identity",
		ReadyForMessaging: false,
		Missing:           []string{"registration", "handle"},
	})

	for _, got := range []error{
		app.messageExit(err, "hint"),
		app.runtimeExit(err, "hint"),
	} {
		var exitErr *output.ExitError
		if !errors.As(got, &exitErr) {
			t.Fatalf("errors.As(%T, *output.ExitError) = false", got)
		}
		if exitErr.Detail.Code != "identity_required" {
			t.Fatalf("exitErr.Detail.Code = %q, want %q", exitErr.Detail.Code, "identity_required")
		}
		if exitErr.Code != 3 {
			t.Fatalf("exitErr.Code = %d, want 3", exitErr.Code)
		}
	}
}

func captureStdout(run func() error) (string, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer reader.Close()

	originalStdout := os.Stdout
	os.Stdout = writer
	runErr := run()
	_ = writer.Close()
	os.Stdout = originalStdout

	output, readErr := io.ReadAll(reader)
	if readErr != nil {
		return "", readErr
	}
	return string(output), runErr
}
