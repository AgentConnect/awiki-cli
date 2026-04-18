package cli

import (
	"errors"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

func TestMailSendValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(cmd *cobra.Command)
	}{
		{
			name: "missing_to",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("subject", "Hello")
				_ = cmd.Flags().Set("body", "Hi")
			},
		},
		{
			name: "missing_subject",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("to", "alice@example.com")
				_ = cmd.Flags().Set("body", "Hi")
			},
		},
		{
			name: "missing_body",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("to", "alice@example.com")
				_ = cmd.Flags().Set("subject", "Hello")
			},
		},
	}

	app := &App{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cmd := newMailSendCommand()
			tt.setup(cmd)
			err := app.runMailSend(cmd, nil)
			assertExitErrorCode(t, err, "invalid_argument", 2)
		})
	}
}

func TestMailReadValidation(t *testing.T) {
	t.Parallel()

	app := &App{}
	cmd := newMailReadCommand()
	err := app.runMailRead(cmd, nil)
	assertExitErrorCode(t, err, "invalid_argument", 2)
}

func TestMailAttachmentValidation(t *testing.T) {
	t.Parallel()

	app := &App{}
	cmd := newMailAttachmentDownloadCommand()
	_ = cmd.Flags().Set("attachment-index", "0")
	// missing message-id
	err := app.runMailAttachmentDownload(cmd, nil)
	assertExitErrorCode(t, err, "invalid_argument", 2)
}

func newMailSendCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("to", "", "")
	cmd.Flags().String("cc", "", "")
	cmd.Flags().String("subject", "", "")
	cmd.Flags().String("body", "", "")
	cmd.Flags().String("html", "", "")
	return cmd
}

func newMailReadCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("id", "", "")
	return cmd
}

func newMailAttachmentDownloadCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().Int("attachment-index", 0, "")
	cmd.Flags().String("output", "", "")
	return cmd
}

func assertExitErrorCode(t *testing.T, err error, code string, exitCode int) {
	t.Helper()
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Detail.Code != code {
		t.Fatalf("exitErr.Detail.Code = %q, want %q", exitErr.Detail.Code, code)
	}
	if exitErr.Code != exitCode {
		t.Fatalf("exitErr.Code = %d, want %d", exitErr.Code, exitCode)
	}
}
