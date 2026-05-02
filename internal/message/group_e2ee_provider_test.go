package message

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordingMLSRunner struct {
	args  []string
	stdin []byte
}

func TestMLSExecProviderBinaryDiscoveryOrder(t *testing.T) {
	t.Setenv(ANPMLSBinaryEnv, "")
	pathDir := t.TempDir()
	pathBinary := filepath.Join(pathDir, "anp-mls")
	if err := os.WriteFile(pathBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathDir)

	injected := filepath.Join(t.TempDir(), "runtime-anp-mls")
	if err := os.WriteFile(injected, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	provider := MLSExecProvider{BinaryPath: injected}
	got, err := provider.ResolveBinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != injected {
		t.Fatalf("ResolveBinaryPath injected = %q, want %q", got, injected)
	}

	envBinary := filepath.Join(t.TempDir(), "env-anp-mls")
	if err := os.WriteFile(envBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ANPMLSBinaryEnv, envBinary)
	got, err = provider.ResolveBinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != envBinary {
		t.Fatalf("ResolveBinaryPath env = %q, want %q", got, envBinary)
	}

	t.Setenv(ANPMLSBinaryEnv, "")
	provider.BinaryPath = ""
	got, err = provider.ResolveBinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != pathBinary {
		t.Fatalf("ResolveBinaryPath PATH = %q, want %q", got, pathBinary)
	}
}

func (r *recordingMLSRunner) Run(_ context.Context, _ string, args []string, stdin []byte) ([]byte, []byte, error) {
	r.args = append([]string(nil), args...)
	r.stdin = append([]byte(nil), stdin...)
	return []byte(`{"ok":true,"api_version":"anp-mls/v1","request_id":"req-1","result":{"non_cryptographic":true}}`), nil, nil
}

func TestMLSExecProviderPassesPlaintextOnStdinNotArgv(t *testing.T) {
	runner := &recordingMLSRunner{}
	provider := MLSExecProvider{BinaryPath: "anp-mls", DataDir: t.TempDir(), Runner: runner}
	_, err := provider.Call(context.Background(), "message", "encrypt", MLSRequest{
		APIVersion:          "anp-mls/v1",
		RequestID:           "req-1",
		AgentDID:            "did:wba:example.com:users:alice:e1",
		DeviceID:            "device-1",
		ContractTestEnabled: true,
		Params: map[string]any{
			"agent_did":             "did:wba:example.com:users:alice:e1",
			"device_id":             "device-1",
			"application_plaintext": map[string]any{"text": "super secret"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(runner.args, " "), "super secret") {
		t.Fatalf("plaintext leaked into argv: %#v", runner.args)
	}
	if !strings.Contains(string(runner.stdin), "super secret") {
		t.Fatalf("plaintext request was not sent via stdin: %s", string(runner.stdin))
	}
	var req MLSRequest
	if err := json.Unmarshal(runner.stdin, &req); err != nil {
		t.Fatal(err)
	}
	if !req.ContractTestEnabled {
		t.Fatal("contract test flag not preserved")
	}
	if req.AgentDID == "" || req.Params["agent_did"] != req.AgentDID {
		t.Fatalf("agent_did not preserved in envelope and params: %#v", req)
	}
	if req.DeviceID == "" || req.Params["device_id"] != req.DeviceID {
		t.Fatalf("device_id not preserved in envelope and params: %#v", req)
	}
	if got := runner.args[len(runner.args)-2]; got != "--data-dir" {
		t.Fatalf("args missing --data-dir before final value: %#v", runner.args)
	}
}
