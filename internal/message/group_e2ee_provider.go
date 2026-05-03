package message

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

const (
	GroupE2EEProfile              = "anp.group.e2ee.v1"
	GroupE2EESecurityProfile      = "group-e2ee"
	GroupE2EETransportProfile     = "transport-protected"
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

// MLSVersionInfo is the machine-readable compatibility contract returned by
// `anp-mls system version --json-in -`.
type MLSVersionInfo struct {
	APIVersion        string   `json:"api_version"`
	BinaryName        string   `json:"binary_name"`
	BinaryVersion     string   `json:"binary_version,omitempty"`
	BuildVersion      string   `json:"build_version,omitempty"`
	SupportedCommands []string `json:"supported_commands"`
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
			if info, err := os.Stat(candidate); err == nil && isExecutableFile(info) {
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

func isExecutableFile(info os.FileInfo) bool {
	if info == nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func (p MLSExecProvider) ProbeVersion(ctx context.Context) (*MLSVersionInfo, error) {
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
	req := MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "doctor-system-version",
		Params:     map[string]any{},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, stderr, err := runner.Run(ctx, binary, []string{"system", "version", "--json-in", "-"}, body)
	if err != nil && len(stdout) == 0 {
		return nil, fmt.Errorf("anp-mls version probe failed: %w: %s", err, string(stderr))
	}
	var resp MLSResponse
	if decodeErr := json.Unmarshal(stdout, &resp); decodeErr != nil {
		return nil, fmt.Errorf("decode anp-mls version response: %w: stderr=%s", decodeErr, string(stderr))
	}
	if !resp.OK {
		if resp.Error != nil {
			return nil, fmt.Errorf("anp-mls version probe error %s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("anp-mls version probe returned ok=false")
	}
	info, err := versionInfoFromResponse(resp)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func versionInfoFromResponse(resp MLSResponse) (*MLSVersionInfo, error) {
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	var info MLSVersionInfo
	if err := json.Unmarshal(resultBytes, &info); err != nil {
		return nil, fmt.Errorf("decode anp-mls version result: %w", err)
	}
	if info.APIVersion == "" {
		info.APIVersion = resp.APIVersion
	}
	if strings.TrimSpace(info.APIVersion) == "" {
		return nil, fmt.Errorf("anp-mls version response missing api_version")
	}
	if strings.TrimSpace(info.BinaryName) == "" {
		return nil, fmt.Errorf("anp-mls version response missing binary_name")
	}
	if strings.TrimSpace(info.BinaryVersion) == "" && strings.TrimSpace(info.BuildVersion) == "" {
		return nil, fmt.Errorf("anp-mls version response missing binary_version/build_version")
	}
	if len(info.SupportedCommands) == 0 {
		return nil, fmt.Errorf("anp-mls version response missing supported_commands")
	}
	return &info, nil
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
	dataDir := p.effectiveDataDir(req)
	if dataDir != "" {
		if err := os.MkdirAll(dataDir, 0o700); err != nil {
			return nil, fmt.Errorf("prepare anp-mls data dir %s: %w", dataDir, err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{domain, action, "--json-in", "-"}
	if dataDir != "" {
		args = append(args, "--data-dir", dataDir)
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

func (p MLSExecProvider) effectiveDataDir(req MLSRequest) string {
	baseDir := strings.TrimSpace(p.DataDir)
	if baseDir == "" {
		return ""
	}
	agentDID := strings.TrimSpace(req.AgentDID)
	if agentDID == "" {
		for _, key := range []string{"agent_did", "owner_did", "actor_did", "sender_did", "recipient_did"} {
			if value := stringFromAny(req.Params[key]); value != "" {
				agentDID = value
				break
			}
		}
	}
	if agentDID == "" {
		return baseDir
	}
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		deviceID = stringFromAny(req.Params["device_id"])
	}
	if deviceID == "" {
		deviceID = "default"
	}
	return filepath.Join(baseDir, "agents", mlsAgentKey(agentDID), safeMLSPathComponent(deviceID))
}

func (p MLSExecProvider) candidateDeviceIDs(agentDID string) []string {
	agentDID = strings.TrimSpace(agentDID)
	if agentDID == "" {
		return []string{"default"}
	}
	candidates := []string{"default"}
	baseDir := strings.TrimSpace(p.DataDir)
	if baseDir == "" {
		return candidates
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "agents", mlsAgentKey(agentDID)))
	if err != nil {
		return candidates
	}
	seen := map[string]struct{}{"default": {}}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		deviceID := strings.TrimSpace(entry.Name())
		if deviceID == "" {
			continue
		}
		if _, ok := seen[deviceID]; ok {
			continue
		}
		seen[deviceID] = struct{}{}
		candidates = append(candidates, deviceID)
	}
	return candidates
}

func mlsAgentKey(agentDID string) string {
	sum := sha256.Sum256([]byte(agentDID))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:24]
}

func safeMLSPathComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "default"
	}
	return builder.String()
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

func (p MLSExecProvider) RemoveMember(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "group", "remove-member", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p MLSExecProvider) LeaveGroup(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "group", "leave", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p MLSExecProvider) CommitFinalize(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "group", "commit-finalize", req)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p MLSExecProvider) CommitAbort(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "group", "commit-abort", req)
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

func (p MLSExecProvider) ProcessCommit(ctx context.Context, req MLSRequest) (map[string]any, error) {
	resp, err := p.Call(ctx, "commit", "process", req)
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
