package cli

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
)

func TestParentNameNormalizesCommandNames(t *testing.T) {
	if got := parentName(" runtime.listener.status "); got != "runtime.listener" {
		t.Fatalf("parentName() = %q, want runtime.listener", got)
	}
	if got := parentName("status"); got != "" {
		t.Fatalf("parentName(status) = %q, want empty", got)
	}
}

func TestCommandFromSpecConfiguresFlagsAndAliases(t *testing.T) {
	app := &App{}
	command := app.commandFromSpec(cmdmeta.CommandSpec{
		Use:     "sample",
		Short:   "sample command",
		Aliases: []string{"alias"},
		Hidden:  true,
		Handler: "stub",
		Flags: []cmdmeta.FlagSpec{
			{Name: "name", Type: "string", Default: "alice", Required: true},
			{Name: "enabled", Type: "bool", Default: "true"},
			{Name: "limit", Type: "int", Default: "42"},
		},
	})

	if !command.Hidden {
		t.Fatal("command.Hidden = false, want true")
	}
	if len(command.Aliases) != 1 || command.Aliases[0] != "alias" {
		t.Fatalf("command.Aliases = %#v, want [alias]", command.Aliases)
	}
	if got, _ := command.Flags().GetString("name"); got != "alice" {
		t.Fatalf("name default = %q, want alice", got)
	}
	if got, _ := command.Flags().GetBool("enabled"); !got {
		t.Fatal("enabled default = false, want true")
	}
	if got, _ := command.Flags().GetInt("limit"); got != 42 {
		t.Fatalf("limit default = %d, want 42", got)
	}
	if flag := command.Flags().Lookup("name"); flag == nil || flag.Annotations == nil || len(flag.Annotations) == 0 {
		t.Fatalf("required flag annotations missing: %#v", flag)
	}
	if command.RunE == nil {
		t.Fatal("command.RunE = nil, want handler")
	}
}

func TestNewRootCommandAddsHiddenListenerCommands(t *testing.T) {
	app := &App{catalog: cmdmeta.NewCatalog(), docs: nil}
	root := newRootCommand(app)

	serviceRun, _, err := root.Find([]string{"runtime", "listener", "service-run"})
	if err != nil {
		t.Fatalf("Find(service-run) error = %v", err)
	}
	if serviceRun == nil || !serviceRun.Hidden {
		t.Fatalf("serviceRun = %#v, want hidden command", serviceRun)
	}
	foregroundRun, _, err := root.Find([]string{"runtime", "listener", "run"})
	if err != nil {
		t.Fatalf("Find(run) error = %v", err)
	}
	if foregroundRun == nil || !foregroundRun.Hidden {
		t.Fatalf("foregroundRun = %#v, want hidden command", foregroundRun)
	}
}

func TestNewRootCommandExposesConfigSet(t *testing.T) {
	app := &App{catalog: cmdmeta.NewCatalog(), docs: nil}
	root := newRootCommand(app)

	command, _, err := root.Find([]string{"config", "set"})
	if err != nil {
		t.Fatalf("Find(config set) error = %v", err)
	}
	if command == nil {
		t.Fatal("config set command = nil")
	}
	if flag := command.Flags().Lookup("did-domain"); flag == nil {
		t.Fatal("did-domain flag = nil, want configured flag")
	}
}

func TestCommandResultMissingReturnsInternalExitError(t *testing.T) {
	err := commandResultMissing("awiki-cli group add")
	exitErr, ok := err.(*output.ExitError)
	if !ok {
		t.Fatalf("commandResultMissing() error type = %T, want *output.ExitError", err)
	}
	if exitErr.Detail.Code != "internal_error" {
		t.Fatalf("exitErr.Detail.Code = %q, want internal_error", exitErr.Detail.Code)
	}
	if !strings.Contains(exitErr.Detail.Message, "group add") {
		t.Fatalf("exitErr.Detail.Message = %q, want command path", exitErr.Detail.Message)
	}
}

func TestHandleErrorRendersExitErrorAndSanitizesInternalIdentityFields(t *testing.T) {
	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON), Identity: "alice"}}
	exitErr := output.NewExitError("invalid_argument", 2, "bad request", "fix it")
	exitErr.Detail.Details = map[string]any{
		"identity": map[string]any{"handle": "alice", "user_id": "internal-user"},
	}
	rendered, code, err := captureStderrWithCode(func() int {
		return app.handleError(exitErr)
	})
	if err != nil {
		t.Fatalf("captureStderrWithCode(handleError) error = %v", err)
	}
	if code != 2 {
		t.Fatalf("handleError() exit code = %d, want 2", code)
	}
	if strings.Contains(rendered, "user_id") || strings.Contains(rendered, "internal-user") {
		t.Fatalf("rendered stderr %q still contains internal user_id", rendered)
	}
	if !strings.Contains(rendered, "alice") {
		t.Fatalf("rendered stderr %q missing identity handle", rendered)
	}
}

func TestHandleErrorFallsBackToInternalErrorForPlainError(t *testing.T) {
	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON)}}
	rendered, code, err := captureStderrWithCode(func() int {
		return app.handleError(os.ErrPermission)
	})
	if err != nil {
		t.Fatalf("captureStderrWithCode(handleError) error = %v", err)
	}
	if code != 1 {
		t.Fatalf("handleError() exit code = %d, want 1", code)
	}
	if !strings.Contains(rendered, "internal_error") {
		t.Fatalf("rendered stderr %q missing internal_error code", rendered)
	}
}

func TestResolveConfigUsesCurrentIdentityFromStore(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspace)

	resolvedForSave, err := appconfig.Resolve(appconfig.Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	manager := identity.NewManager(resolvedForSave.Paths)
	if _, err := manager.Save(identity.SaveInput{
		IdentityName: "default",
		DID:          "did:wba:example.com:user:alice:e1_alice",
		UniqueID:     "e1_alice",
		DisplayName:  "Alice",
		Handle:       "alice",
		UserID:       "user-123",
	}); err != nil {
		t.Fatalf("manager.Save() error = %v", err)
	}

	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON)}}
	resolved, err := app.resolveConfig()
	if err != nil {
		t.Fatalf("resolveConfig() error = %v", err)
	}
	if resolved.ActiveIdentity != "default" {
		t.Fatalf("resolved.ActiveIdentity = %q, want default", resolved.ActiveIdentity)
	}
	if got := resolved.Sources["active_identity"].Source; got != "identity_index" {
		t.Fatalf("active_identity source = %q, want identity_index", got)
	}
}

func captureStderrWithCode(run func() int) (string, int, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", 0, err
	}
	defer reader.Close()

	originalStderr := os.Stderr
	os.Stderr = writer
	code := run()
	_ = writer.Close()
	os.Stderr = originalStderr

	output, readErr := io.ReadAll(reader)
	if readErr != nil {
		return "", 0, readErr
	}
	return string(output), code, nil
}
