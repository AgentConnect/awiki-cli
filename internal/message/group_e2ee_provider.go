package message

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

const (
	GroupE2EEProfile              = "anp.group.e2ee.v1"
	GroupE2EESecurityProfile      = "group-e2ee"
	GroupE2EEContractArtifactMode = "contract-test"
	DefaultANPMLSBinary           = "anp-mls"
	ANPMLSBinaryEnv               = "AWIKI_ANP_MLS_BINARY"
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
		DataDir: DefaultMLSDataDir(resolved),
		Timeout: 15 * time.Second,
	}
}

func DefaultMLSDataDir(resolved *appconfig.Resolved) string {
	if resolved == nil || resolved.Paths.WorkspaceHomeDir == "" {
		return filepath.Join(".awiki-cli", "mls")
	}
	return filepath.Join(resolved.Paths.WorkspaceHomeDir, "mls")
}

func (p MLSExecProvider) ResolveBinaryPath() (string, error) {
	candidates := make([]string, 0, 3)
	if envPath := strings.TrimSpace(os.Getenv(ANPMLSBinaryEnv)); envPath != "" {
		candidates = append(candidates, envPath)
	}
	if injected := strings.TrimSpace(p.BinaryPath); injected != "" {
		candidates = append(candidates, injected)
	}
	candidates = append(candidates, DefaultANPMLSBinary)

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if filepath.IsAbs(candidate) || strings.ContainsRune(candidate, filepath.Separator) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf(
		"unable to locate anp-mls binary (checked %s, injected path, then PATH). Set %s to an absolute anp-mls path, build/install anp-mls, or run `awiki-cli doctor` for diagnostics",
		ANPMLSBinaryEnv,
		ANPMLSBinaryEnv,
	)
}

func (p MLSExecProvider) Call(ctx context.Context, domain string, action string, req MLSRequest) (*MLSResponse, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	runner := p.Runner
	if runner == nil {
		runner = OSMLSCommandRunner{}
	}
	binary := strings.TrimSpace(p.BinaryPath)
	if binary == "" || p.Runner == nil {
		resolvedBinary, err := p.ResolveBinaryPath()
		if err != nil {
			return nil, err
		}
		binary = resolvedBinary
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if p.DataDir != "" {
		if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
			return nil, fmt.Errorf("prepare anp-mls data dir %s: %w", p.DataDir, err)
		}
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

func (p MLSExecProvider) GenerateKeyPackage(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "key-package", "generate", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p MLSExecProvider) CreateGroup(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "group", "create", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p MLSExecProvider) AddMember(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "group", "add-member", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p MLSExecProvider) ProcessWelcome(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "welcome", "process", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p MLSExecProvider) Encrypt(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "message", "encrypt", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p MLSExecProvider) Decrypt(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "message", "decrypt", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p MLSExecProvider) Status(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "group", "status", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}
