package cli

import (
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/output"
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
