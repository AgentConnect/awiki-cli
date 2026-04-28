package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/output"
)

func TestIsWindowsDirSyncCompatibilityErrorMatchesKnownPattern(t *testing.T) {
	t.Parallel()

	err := errors.New(`write config yaml: sync config dir: sync C:\Users\liuzhuocheng\.awiki-cli: Access is denied.`)
	if !isWindowsDirSyncCompatibilityError(err) {
		t.Fatal("isWindowsDirSyncCompatibilityError() = false, want true")
	}
}

func TestIsWindowsDirSyncCompatibilityErrorIgnoresNormalPermissionErrors(t *testing.T) {
	t.Parallel()

	cases := []error{
		errors.New("create config dir: mkdir C:\\Users\\liuzhuocheng\\.awiki-cli: Access is denied."),
		errors.New("write config yaml: open /tmp/config.yaml: permission denied"),
		errors.New("write route registry: parse route registry: invalid character 'x'"),
	}
	for _, err := range cases {
		if isWindowsDirSyncCompatibilityError(err) {
			t.Fatalf("isWindowsDirSyncCompatibilityError(%q) = true, want false", err)
		}
	}
}

func TestRuntimeExitRefinesWindowsDirSyncCompatibilityHint(t *testing.T) {
	t.Parallel()

	app := &App{}
	err := app.runtimeExit(
		errors.New(`write config yaml: sync config dir: sync C:\Users\liuzhuocheng\.awiki-cli: Access is denied.`),
		"Check write permissions for config.yaml.",
	)

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("errors.As(%T, *output.ExitError) = false", err)
	}
	if exitErr.Detail.Hint != windowsDirSyncCompatibilityHint {
		t.Fatalf("exitErr.Detail.Hint = %q, want %q", exitErr.Detail.Hint, windowsDirSyncCompatibilityHint)
	}
}

func TestConfigCommandExitRefinesWindowsDirSyncCompatibilityHint(t *testing.T) {
	t.Parallel()

	app := &App{}
	err := app.configCommandExit(errors.New(`upgrade workspace: sync dir: sync C:\Users\liuzhuocheng\.awiki-cli: Access is denied.`))

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("errors.As(%T, *output.ExitError) = false", err)
	}
	if !strings.Contains(exitErr.Detail.Hint, "Windows workspace directory-sync compatibility failure") {
		t.Fatalf("exitErr.Detail.Hint = %q, want refined Windows compatibility hint", exitErr.Detail.Hint)
	}
}
