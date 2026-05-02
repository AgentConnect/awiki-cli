package message

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type recordingMLSRunner struct {
	args  []string
	stdin []byte
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
		ContractTestEnabled: true,
		Params: map[string]any{
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
	if got := runner.args[len(runner.args)-2]; got != "--data-dir" {
		t.Fatalf("args missing --data-dir before final value: %#v", runner.args)
	}
}
