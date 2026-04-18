package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
	docindex "github.com/agentconnect/awiki-cli/internal/docs"
	"github.com/agentconnect/awiki-cli/internal/output"
)

type cliSuccessEnvelope struct {
	OK      bool           `json:"ok"`
	Command string         `json:"command"`
	Data    map[string]any `json:"data"`
	Summary string         `json:"summary"`
	Meta    map[string]any `json:"meta"`
}

type cliErrorEnvelope struct {
	OK    bool               `json:"ok"`
	Error output.ErrorDetail `json:"error"`
	Meta  map[string]any     `json:"meta"`
}

func TestRootCommandSuccessContracts(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		assert func(t *testing.T, workspace string, stdout string, stderr string)
	}{
		{
			name: "init dry run returns execution plan without writes",
			args: []string{"--dry-run", "init"},
			assert: func(t *testing.T, workspace string, stdout string, stderr string) {
				if stderr != "" {
					t.Fatalf("stderr = %q, want empty", stderr)
				}
				envelope := decodeSuccessEnvelope(t, stdout)
				if envelope.Command != "awiki-cli init" {
					t.Fatalf("command = %q, want %q", envelope.Command, "awiki-cli init")
				}
				if envelope.Summary != "Dry run: workspace initialization planned" {
					t.Fatalf("summary = %q, want dry-run summary", envelope.Summary)
				}
				if envelope.Meta["dry_run"] != true {
					t.Fatalf("meta.dry_run = %#v, want true", envelope.Meta["dry_run"])
				}
				plan, ok := envelope.Data["plan"].(map[string]any)
				if !ok {
					t.Fatalf("data.plan type = %T, want map[string]any", envelope.Data["plan"])
				}
				if plan["action"] != "init_workspace" {
					t.Fatalf("plan.action = %#v, want %q", plan["action"], "init_workspace")
				}
				if plan["root_dir"] != workspace {
					t.Fatalf("plan.root_dir = %#v, want %q", plan["root_dir"], workspace)
				}
				if plan["config_exists"] != false {
					t.Fatalf("plan.config_exists = %#v, want false", plan["config_exists"])
				}
				if _, err := os.Stat(filepath.Join(workspace, "config", "config.yaml")); !os.IsNotExist(err) {
					t.Fatalf("config.yaml stat error = %v, want not exist", err)
				}
			},
		},
		{
			name: "docs lookup preserves summary and topic structure",
			args: []string{"docs", "  OvErViEw  "},
			assert: func(t *testing.T, workspace string, stdout string, stderr string) {
				if stderr != "" {
					t.Fatalf("stderr = %q, want empty", stderr)
				}
				envelope := decodeSuccessEnvelope(t, stdout)
				if envelope.Command != "awiki-cli docs" {
					t.Fatalf("command = %q, want %q", envelope.Command, "awiki-cli docs")
				}
				if envelope.Summary != "Documentation topic overview" {
					t.Fatalf("summary = %q, want %q", envelope.Summary, "Documentation topic overview")
				}
				topic, ok := envelope.Data["topic"].(map[string]any)
				if !ok {
					t.Fatalf("data.topic type = %T, want map[string]any", envelope.Data["topic"])
				}
				if topic["name"] != "overview" {
					t.Fatalf("topic.name = %#v, want %q", topic["name"], "overview")
				}
				refs, ok := topic["references"].([]any)
				if !ok || len(refs) == 0 {
					t.Fatalf("topic.references = %#v, want non-empty []any", topic["references"])
				}
			},
		},
		{
			name: "doctor reports envelope summary and report data shape",
			args: []string{"doctor"},
			assert: func(t *testing.T, workspace string, stdout string, stderr string) {
				if stderr != "" {
					t.Fatalf("stderr = %q, want empty", stderr)
				}
				envelope := decodeSuccessEnvelope(t, stdout)
				if envelope.Command != "awiki-cli doctor" {
					t.Fatalf("command = %q, want %q", envelope.Command, "awiki-cli doctor")
				}
				checks, ok := envelope.Data["checks"].([]any)
				if !ok || len(checks) == 0 {
					t.Fatalf("data.checks = %#v, want non-empty []any", envelope.Data["checks"])
				}
				counts, ok := envelope.Data["counts"].(map[string]any)
				if !ok {
					t.Fatalf("data.counts type = %T, want map[string]any", envelope.Data["counts"])
				}
				for _, key := range []string{"ok", "warn", "error", "info"} {
					if _, exists := counts[key]; !exists {
						t.Fatalf("data.counts missing key %q in %#v", key, counts)
					}
				}
				reportSummary, ok := envelope.Data["summary"].(string)
				if !ok || reportSummary == "" {
					t.Fatalf("data.summary = %#v, want non-empty string", envelope.Data["summary"])
				}
				if envelope.Summary != reportSummary {
					t.Fatalf("envelope summary = %q, want %q", envelope.Summary, reportSummary)
				}
			},
		},
		{
			name: "version exposes current build info contract",
			args: []string{"version"},
			assert: func(t *testing.T, workspace string, stdout string, stderr string) {
				if stderr != "" {
					t.Fatalf("stderr = %q, want empty", stderr)
				}
				envelope := decodeSuccessEnvelope(t, stdout)
				if envelope.Command != "awiki-cli version" {
					t.Fatalf("command = %q, want %q", envelope.Command, "awiki-cli version")
				}
				if envelope.Summary != "Build information" {
					t.Fatalf("summary = %q, want %q", envelope.Summary, "Build information")
				}
				for _, key := range []string{"version", "commit", "build_date", "go_version", "goos", "goarch", "compiler", "cgo_enabled"} {
					if _, exists := envelope.Data[key]; !exists {
						t.Fatalf("data missing key %q in %#v", key, envelope.Data)
					}
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspace)

			exitCode, stdout, stderr := executeRootCommand(t, tc.args...)
			if exitCode != 0 {
				t.Fatalf("executeRootCommand(%v) exitCode = %d, want 0; stderr = %q", tc.args, exitCode, stderr)
			}
			tc.assert(t, workspace, stdout, stderr)
		})
	}
}

func TestRootDocsErrorsUseFrozenExitCodes(t *testing.T) {
	cases := []struct {
		name              string
		args              []string
		wantExitCode      int
		wantCode          string
		wantMessageSubstr string
		wantHintSubstr    string
	}{
		{
			name:              "too many args",
			args:              []string{"docs", "overview", "extra"},
			wantExitCode:      2,
			wantCode:          "invalid_argument",
			wantMessageSubstr: "docs accepts at most one topic",
			wantHintSubstr:    "awiki-cli docs",
		},
		{
			name:              "unknown topic",
			args:              []string{"docs", "missing-topic"},
			wantExitCode:      5,
			wantCode:          "not_found",
			wantMessageSubstr: "Unknown docs topic",
			wantHintSubstr:    "awiki-cli docs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", t.TempDir())

			exitCode, stdout, stderr := executeRootCommand(t, tc.args...)
			if exitCode != tc.wantExitCode {
				t.Fatalf("exitCode = %d, want %d", exitCode, tc.wantExitCode)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}

			envelope := decodeErrorEnvelope(t, stderr)
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error.code = %q, want %q", envelope.Error.Code, tc.wantCode)
			}
			if !contains(envelope.Error.Message, tc.wantMessageSubstr) {
				t.Fatalf("error.message = %q, want substring %q", envelope.Error.Message, tc.wantMessageSubstr)
			}
			if !contains(envelope.Error.Hint, tc.wantHintSubstr) {
				t.Fatalf("error.hint = %q, want substring %q", envelope.Error.Hint, tc.wantHintSubstr)
			}
			if envelope.Meta["format"] != string(output.FormatJSON) {
				t.Fatalf("meta.format = %#v, want %q", envelope.Meta["format"], output.FormatJSON)
			}
		})
	}
}

func executeRootCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	app := &App{
		globals: GlobalOptions{Format: string(output.FormatJSON)},
		catalog: cmdmeta.NewCatalog(),
		docs:    docindex.NewIndex(),
	}
	rootCmd := newRootCommand(app)
	rootCmd.SetArgs(args)

	stdout, stderr, runErr := captureOutputStreams(func() error {
		return rootCmd.Execute()
	})
	if runErr == nil {
		return 0, stdout, stderr
	}
	exitCode := 0
	finalStdout, finalStderr, renderErr := captureOutputStreams(func() error {
		exitCode = app.handleError(runErr)
		return nil
	})
	if renderErr != nil {
		t.Fatalf("capture handleError() error = %v", renderErr)
	}
	return exitCode, stdout + finalStdout, stderr + finalStderr
}

func captureOutputStreams(run func() error) (string, string, error) {
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	defer stdoutReader.Close()

	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutWriter.Close()
		return "", "", err
	}
	defer stderrReader.Close()

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter

	runErr := run()

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout = originalStdout
	os.Stderr = originalStderr

	stdoutRaw, stdoutErr := io.ReadAll(stdoutReader)
	if stdoutErr != nil {
		return "", "", stdoutErr
	}
	stderrRaw, stderrErr := io.ReadAll(stderrReader)
	if stderrErr != nil {
		return "", "", stderrErr
	}
	return string(stdoutRaw), string(stderrRaw), runErr
}

func decodeSuccessEnvelope(t *testing.T, raw string) cliSuccessEnvelope {
	t.Helper()
	var envelope cliSuccessEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(success) error = %v; raw = %q", err, raw)
	}
	if !envelope.OK {
		t.Fatalf("success envelope ok = %t, want true; raw = %q", envelope.OK, raw)
	}
	return envelope
}

func decodeErrorEnvelope(t *testing.T, raw string) cliErrorEnvelope {
	t.Helper()
	var envelope cliErrorEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(error) error = %v; raw = %q", err, raw)
	}
	if envelope.OK {
		t.Fatalf("error envelope ok = %t, want false; raw = %q", envelope.OK, raw)
	}
	return envelope
}

func contains(haystack string, needle string) bool {
	return strings.Contains(haystack, needle)
}
