package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/content"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/message"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
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

func TestRenderIdentityResultIgnoresTypedNilIdentityMeta(t *testing.T) {
	app := &App{}
	result := &identity.CommandResult{
		Data: map[string]any{
			"default_identity": (*identity.IdentitySummary)(nil),
			"identities":       []identity.IdentitySummary{},
		},
		Summary: "Found 0 local identities",
	}

	rendered, err := captureStdout(func() error {
		return app.renderIdentityResult(&cobra.Command{Use: "list"}, output.FormatJSON, result)
	})
	if err != nil {
		t.Fatalf("captureStdout(renderIdentityResult) error = %v", err)
	}
	if strings.Contains(rendered, "\"identity\"") {
		t.Fatalf("rendered output %q unexpectedly included identity meta", rendered)
	}
	if !strings.Contains(rendered, "Found 0 local identities") {
		t.Fatalf("rendered output %q missing summary", rendered)
	}
}

func TestRenderIdentityResultRejectsNilResult(t *testing.T) {
	app := &App{}
	err := app.renderIdentityResult(&cobra.Command{Use: "list"}, output.FormatJSON, nil)
	assertMissingResultError(t, err, "list")
}

func TestRenderMessageResultRejectsNilResult(t *testing.T) {
	app := &App{}
	err := app.renderMessageResult(&cobra.Command{Use: "send"}, output.FormatJSON, nil)
	assertMissingResultError(t, err, "send")
}

func TestRenderContentResultRejectsNilResult(t *testing.T) {
	app := &App{}
	err := app.renderContentResult(&cobra.Command{Use: "get"}, output.FormatJSON, nil)
	assertMissingResultError(t, err, "get")
}

func TestRenderMessageResultRendersValidResult(t *testing.T) {
	app := &App{}
	rendered, err := captureStdout(func() error {
		return app.renderMessageResult(&cobra.Command{Use: "send"}, output.FormatJSON, &message.CommandResult{
			Data:    map[string]any{"ok": true},
			Summary: "Sent",
		})
	})
	if err != nil {
		t.Fatalf("captureStdout(renderMessageResult) error = %v", err)
	}
	if !strings.Contains(rendered, "\"summary\": \"Sent\"") {
		t.Fatalf("rendered output %q missing summary", rendered)
	}
}

func TestRenderMessageResultHidesInformationalTransportWarningWithoutVerbose(t *testing.T) {
	app := &App{}
	rendered, err := captureStdout(func() error {
		return app.renderMessageResult(&cobra.Command{Use: "create"}, output.FormatJSON, &message.CommandResult{
			Data:     map[string]any{"ok": true},
			Summary:  "Created",
			Warnings: []string{"Group lifecycle commands use HTTP transport even when runtime.mode is websocket."},
		})
	})
	if err != nil {
		t.Fatalf("captureStdout(renderMessageResult) error = %v", err)
	}
	if strings.Contains(rendered, "Group lifecycle commands use HTTP transport even when runtime.mode is websocket.") {
		t.Fatalf("rendered output %q unexpectedly included informational warning", rendered)
	}
}

func TestRenderMessageResultShowsInformationalTransportWarningWithVerbose(t *testing.T) {
	app := &App{globals: GlobalOptions{Verbose: true}}
	rendered, err := captureStdout(func() error {
		return app.renderMessageResult(&cobra.Command{Use: "create"}, output.FormatJSON, &message.CommandResult{
			Data:     map[string]any{"ok": true},
			Summary:  "Created",
			Warnings: []string{"Group lifecycle commands use HTTP transport even when runtime.mode is websocket."},
		})
	})
	if err != nil {
		t.Fatalf("captureStdout(renderMessageResult) error = %v", err)
	}
	if !strings.Contains(rendered, "Group lifecycle commands use HTTP transport even when runtime.mode is websocket.") {
		t.Fatalf("rendered output %q missing informational warning in verbose mode", rendered)
	}
}

func TestRenderContentResultRendersValidResult(t *testing.T) {
	app := &App{}
	rendered, err := captureStdout(func() error {
		return app.renderContentResult(&cobra.Command{Use: "get"}, output.FormatJSON, &content.CommandResult{
			Data:    map[string]any{"slug": "hello"},
			Summary: "Loaded page",
		})
	})
	if err != nil {
		t.Fatalf("captureStdout(renderContentResult) error = %v", err)
	}
	if !strings.Contains(rendered, "\"summary\": \"Loaded page\"") {
		t.Fatalf("rendered output %q missing summary", rendered)
	}
}

func assertMissingResultError(t *testing.T, err error, commandName string) {
	t.Helper()

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("errors.As(%T, *output.ExitError) = false", err)
	}
	if exitErr.Detail.Code != "internal_error" {
		t.Fatalf("exitErr.Detail.Code = %q, want internal_error", exitErr.Detail.Code)
	}
	if !strings.Contains(exitErr.Detail.Message, commandName) {
		t.Fatalf("exitErr.Detail.Message = %q, want mention of %q", exitErr.Detail.Message, commandName)
	}
}

func TestIsUpdateExemptCommandAllowsListenerServiceRun(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "awiki-cli"}
	runtime := &cobra.Command{Use: "runtime"}
	listener := &cobra.Command{Use: "listener"}
	command := &cobra.Command{Use: "service-run"}
	root.AddCommand(runtime)
	runtime.AddCommand(listener)
	listener.AddCommand(command)
	if !isUpdateExemptCommand(command) {
		t.Fatal("isUpdateExemptCommand(service-run) = false, want true")
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
