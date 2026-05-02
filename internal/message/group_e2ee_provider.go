package message

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

const (
	GroupE2EEProfile              = "anp.group.e2ee.v1"
	GroupE2EESecurityProfile      = "group-e2ee"
	GroupE2EEContractArtifactMode = "contract-test"
	DefaultANPMLSBinary           = "anp-mls"
)

type MLSRequest struct {
	APIVersion          string         `json:"api_version"`
	RequestID           string         `json:"request_id"`
	AgentDID            string         `json:"agent_did,omitempty"`
	DeviceID            string         `json:"device_id,omitempty"`
	ContractTestEnabled bool           `json:"contract_test_enabled,omitempty"`
	Params              map[string]any `json:"params"`
}

type MLSResponse struct {
	OK         bool           `json:"ok"`
	APIVersion string         `json:"api_version"`
	RequestID  string         `json:"request_id"`
	Result     map[string]any `json:"result,omitempty"`
	Error      *MLSError      `json:"error,omitempty"`
}

type MLSError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type MLSCommandRunner interface {
	Run(ctx context.Context, binary string, args []string, stdin []byte) (stdout []byte, stderr []byte, err error)
}

type OSMLSCommandRunner struct{}

func (OSMLSCommandRunner) Run(ctx context.Context, binary string, args []string, stdin []byte) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type MLSExecProvider struct {
	BinaryPath string
	DataDir    string
	Timeout    time.Duration
	Runner     MLSCommandRunner
}

func NewDefaultMLSExecProvider(resolved *appconfig.Resolved) MLSExecProvider {
	return MLSExecProvider{
		BinaryPath: DefaultANPMLSBinary,
		DataDir:    DefaultMLSDataDir(resolved),
		Timeout:    15 * time.Second,
	}
}

func DefaultMLSDataDir(resolved *appconfig.Resolved) string {
	if resolved == nil || resolved.Paths.WorkspaceHomeDir == "" {
		return filepath.Join(".awiki-cli", "mls")
	}
	return filepath.Join(resolved.Paths.WorkspaceHomeDir, "mls")
}

func (p MLSExecProvider) Call(ctx context.Context, domain string, action string, req MLSRequest) (*MLSResponse, error) {
	binary := p.BinaryPath
	if binary == "" {
		binary = DefaultANPMLSBinary
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	runner := p.Runner
	if runner == nil {
		runner = OSMLSCommandRunner{}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{domain, action, "--json-in", "-"}
	if p.DataDir != "" {
		args = append(args, "--data-dir", p.DataDir)
	}
	stdout, stderr, err := runner.Run(ctx, binary, args, body)
	if err != nil && len(stdout) == 0 {
		return nil, fmt.Errorf("anp-mls exec failed: %w: %s", err, string(stderr))
	}
	var resp MLSResponse
	if decodeErr := json.Unmarshal(stdout, &resp); decodeErr != nil {
		return nil, fmt.Errorf("decode anp-mls response: %w: stderr=%s", decodeErr, string(stderr))
	}
	if !resp.OK {
		if resp.Error != nil {
			return &resp, fmt.Errorf("anp-mls error %s: %s", resp.Error.Code, resp.Error.Message)
		}
		return &resp, fmt.Errorf("anp-mls returned ok=false")
	}
	return &resp, nil
}
