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
	if strings.Join(args, " ") == "system version --json-in -" {
		return []byte(`{"ok":true,"api_version":"anp-mls/v1","request_id":"doctor-system-version","result":{"api_version":"anp-mls/v1","binary_name":"anp-mls","binary_version":"test","supported_commands":["system version","message encrypt"]}}`), nil, nil
	}
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
	dataDir := runner.args[len(runner.args)-1]
	if !strings.Contains(dataDir, filepath.Join("agents")) || !strings.HasSuffix(dataDir, filepath.Join("agents", filepath.Base(filepath.Dir(dataDir)), "device-1")) {
		t.Fatalf("data dir = %q, want agent-scoped device directory", dataDir)
	}
	if strings.Contains(dataDir, "did:wba") {
		t.Fatalf("data dir leaked raw DID: %q", dataDir)
	}
}

func TestMLSExecProviderMembershipLifecycleUsesStableCommands(t *testing.T) {
	cases := []struct {
		name string
		call func(context.Context, MLSExecProvider, MLSRequest) (map[string]any, error)
		want string
	}{
		{name: "remove", call: func(ctx context.Context, p MLSExecProvider, req MLSRequest) (map[string]any, error) {
			return p.RemoveMember(ctx, req)
		}, want: "group remove-member --json-in -"},
		{name: "leave", call: func(ctx context.Context, p MLSExecProvider, req MLSRequest) (map[string]any, error) {
			return p.LeaveGroup(ctx, req)
		}, want: "group leave --json-in -"},
		{name: "finalize", call: func(ctx context.Context, p MLSExecProvider, req MLSRequest) (map[string]any, error) {
			return p.CommitFinalize(ctx, req)
		}, want: "group commit-finalize --json-in -"},
		{name: "abort", call: func(ctx context.Context, p MLSExecProvider, req MLSRequest) (map[string]any, error) {
			return p.CommitAbort(ctx, req)
		}, want: "group commit-abort --json-in -"},
		{name: "commit process", call: func(ctx context.Context, p MLSExecProvider, req MLSRequest) (map[string]any, error) {
			return p.ProcessCommit(ctx, req)
		}, want: "commit process --json-in -"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingMLSRunner{}
			provider := MLSExecProvider{BinaryPath: "anp-mls", Runner: runner}
			_, err := tc.call(context.Background(), provider, MLSRequest{
				APIVersion: "anp-mls/v1",
				RequestID:  "req-" + tc.name,
				AgentDID:   "did:wba:example.com:users:alice:e1_alice",
				Params: map[string]any{
					"agent_did":    "did:wba:example.com:users:alice:e1_alice",
					"group_did":    "did:wba:example.com:groups:demo:e1_group",
					"operation_id": "op-1",
				},
			})
			if err != nil {
				t.Fatalf("provider call error = %v", err)
			}
			if got := strings.Join(runner.args, " "); got != tc.want {
				t.Fatalf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMLSExecProviderCandidateDeviceIDsScansAgentScopedState(t *testing.T) {
	root := t.TempDir()
	agentDID := "did:wba:example.com:users:bob:e1"
	agentDir := filepath.Join(root, "agents", mlsAgentKey(agentDID))
	if err := os.MkdirAll(filepath.Join(agentDir, "bob-main"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(agentDir, "bob.backup"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "not-a-device"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := MLSExecProvider{DataDir: root}.candidateDeviceIDs(agentDID)
	want := map[string]bool{"default": true, "bob-main": true, "bob.backup": true}
	if len(got) != len(want) {
		t.Fatalf("candidateDeviceIDs() = %#v, want keys %#v", got, want)
	}
	for _, deviceID := range got {
		if !want[deviceID] {
			t.Fatalf("candidateDeviceIDs() returned unexpected device %q in %#v", deviceID, got)
		}
	}
	if got[0] != "default" {
		t.Fatalf("candidateDeviceIDs()[0] = %q, want default first", got[0])
	}
}

func TestMLSExecProviderRejectsNonExecutablePath(t *testing.T) {
	if os.PathSeparator == ';' {
		t.Skip("Windows executable bit semantics differ")
	}
	t.Setenv(ANPMLSBinaryEnv, "")
	nonExecutable := filepath.Join(t.TempDir(), "anp-mls")
	if err := os.WriteFile(nonExecutable, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := MLSExecProvider{BinaryPath: nonExecutable}
	if got, err := provider.ResolveBinaryPath(); err == nil {
		t.Fatalf("ResolveBinaryPath() = %q, nil error; want non-executable path rejected", got)
	}
}

func TestMLSExecProviderProbeVersionUsesStableSystemContract(t *testing.T) {
	runner := &recordingMLSRunner{}
	provider := MLSExecProvider{BinaryPath: "anp-mls", DataDir: t.TempDir(), Runner: runner}
	info, err := provider.ProbeVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.args, " "); got != "system version --json-in -" {
		t.Fatalf("ProbeVersion args = %q, want stable system version probe", got)
	}
	if strings.Contains(strings.Join(runner.args, " "), provider.DataDir) {
		t.Fatalf("ProbeVersion should not require or expose data dir args: %#v", runner.args)
	}
	if info.APIVersion != "anp-mls/v1" || info.BinaryName != "anp-mls" || info.BinaryVersion == "" {
		t.Fatalf("ProbeVersion info = %#v", info)
	}
	var req MLSRequest
	if err := json.Unmarshal(runner.stdin, &req); err != nil {
		t.Fatal(err)
	}
	if req.RequestID != "doctor-system-version" || req.Params == nil {
		t.Fatalf("ProbeVersion stdin request = %#v", req)
	}
}
