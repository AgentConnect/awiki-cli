package upgrader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/update"
)

type Manager struct {
	paths  Paths
	client *http.Client
}

func NewManager(resolved *appconfig.Resolved) (*Manager, error) {
	paths, err := ResolvePaths(resolved)
	if err != nil {
		return nil, err
	}
	return &Manager{paths: paths, client: &http.Client{}}, nil
}

func (m *Manager) Paths() Paths {
	return m.paths
}

func (m *Manager) LoadReleaseState() (*ReleaseState, error) {
	return LoadReleaseState(m.paths.ReleaseStatePath)
}

func (m *Manager) RecordCheck(decision update.Decision) (*ReleaseState, error) {
	return RecordCheck(m.paths.ReleaseStatePath, decision)
}

func (m *Manager) RecordWSPush(decision update.Decision) (*ReleaseState, error) {
	return RecordWSPush(m.paths.ReleaseStatePath, decision)
}

func (m *Manager) RecordWSPushObservation(decision update.Decision) (*ReleaseState, error) {
	return RecordWSPushObservation(m.paths.ReleaseStatePath, decision)
}

func (m *Manager) Predownload(ctx context.Context, decision update.Decision) (*ReleaseState, error) {
	version := strings.TrimSpace(decision.LatestVersion)
	if version == "" {
		return nil, fmt.Errorf("latest version is empty")
	}
	if !decision.ArtifactAvailable || strings.TrimSpace(decision.ArtifactURL) == "" {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "predownload", "no installable artifact is available")
		return nil, fmt.Errorf("no installable artifact is available for version %s", version)
	}
	if strings.TrimSpace(decision.ArtifactSHA256) == "" {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "predownload", "artifact sha256 is missing")
		return nil, fmt.Errorf("artifact sha256 is missing for version %s", version)
	}
	if err := os.MkdirAll(m.paths.StagingDir, 0o700); err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	if err := os.MkdirAll(m.paths.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	unlock, err := AcquireLock(m.paths.LockFile, version)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unlock() }()

	artifactPath := stagingArtifactPath(m.paths, version, decision.ArtifactURL)
	if fileExists(artifactPath) {
		if err := VerifySHA256(artifactPath, decision.ArtifactSHA256); err == nil {
			return RecordPredownload(m.paths.ReleaseStatePath, decision)
		}
		_ = os.Remove(artifactPath)
	}
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o700); err != nil {
		return nil, fmt.Errorf("create predownload dir: %w", err)
	}
	if err := DownloadArtifact(ctx, m.client, decision.ArtifactURL, artifactPath); err != nil {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "download", err.Error())
		return nil, err
	}
	if err := VerifySHA256(artifactPath, decision.ArtifactSHA256); err != nil {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "verify", err.Error())
		return nil, err
	}
	return RecordPredownload(m.paths.ReleaseStatePath, decision)
}

func (m *Manager) Apply(ctx context.Context, decision update.Decision) (*ApplyResult, error) {
	version := strings.TrimSpace(decision.LatestVersion)
	if version == "" {
		return nil, fmt.Errorf("latest version is empty")
	}
	if !decision.ArtifactAvailable || strings.TrimSpace(decision.ArtifactURL) == "" {
		return nil, fmt.Errorf("no installable artifact is available for version %s", version)
	}
	if strings.TrimSpace(decision.ArtifactSHA256) == "" {
		return nil, fmt.Errorf("artifact sha256 is missing for version %s", version)
	}
	if err := os.MkdirAll(m.paths.VersionsDir, 0o700); err != nil {
		return nil, fmt.Errorf("create versions dir: %w", err)
	}
	if err := os.MkdirAll(m.paths.BinDir, 0o700); err != nil {
		return nil, fmt.Errorf("create bin dir: %w", err)
	}
	if err := os.MkdirAll(m.paths.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	if err := os.MkdirAll(m.paths.StagingDir, 0o700); err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	unlock, err := AcquireLock(m.paths.LockFile, version)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unlock() }()

	installedBinary := installedBinaryPath(m.paths, version)
	alreadyCurrent := samePath(ResolveCurrentTarget(m.paths.CurrentBinaryPath), installedBinary)
	if fileExists(installedBinary) {
		if err := switchCurrentBinary(installedBinary, m.paths.CurrentBinaryPath); err != nil {
			_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "switch", err.Error())
			return nil, err
		}
		state, err := RecordSuccess(m.paths.ReleaseStatePath, decision)
		if err != nil {
			return nil, err
		}
		return &ApplyResult{
			InstalledVersion:    version,
			InstalledBinaryPath: installedBinary,
			CurrentBinaryPath:   m.paths.CurrentBinaryPath,
			AlreadyCurrent:      alreadyCurrent,
			ReleaseState:        state,
		}, nil
	}

	artifactPath := stagingArtifactPath(m.paths, version, decision.ArtifactURL)
	extractDir := stagingExtractDir(m.paths, version)
	if err := os.RemoveAll(filepath.Dir(artifactPath)); err != nil {
		return nil, fmt.Errorf("clean staging dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o700); err != nil {
		return nil, fmt.Errorf("create version staging dir: %w", err)
	}
	if err := DownloadArtifact(ctx, m.client, decision.ArtifactURL, artifactPath); err != nil {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "download", err.Error())
		return nil, err
	}
	if err := VerifySHA256(artifactPath, decision.ArtifactSHA256); err != nil {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "verify", err.Error())
		return nil, err
	}
	extractedBinary, err := ExtractArtifact(artifactPath, extractDir)
	if err != nil {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "extract", err.Error())
		return nil, err
	}
	installedBinary, err = installVersionBinary(extractedBinary, installedBinary)
	if err != nil {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "install", err.Error())
		return nil, err
	}
	if err := switchCurrentBinary(installedBinary, m.paths.CurrentBinaryPath); err != nil {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "switch", err.Error())
		return nil, err
	}
	state, err := RecordSuccess(m.paths.ReleaseStatePath, decision)
	if err != nil {
		_, _ = RecordFailure(m.paths.ReleaseStatePath, version, "state_write", err.Error())
		return nil, err
	}
	_ = os.RemoveAll(filepath.Dir(artifactPath))
	return &ApplyResult{
		InstalledVersion:    version,
		InstalledBinaryPath: installedBinary,
		CurrentBinaryPath:   m.paths.CurrentBinaryPath,
		AlreadyCurrent:      alreadyCurrent,
		ReleaseState:        state,
	}, nil
}

func installVersionBinary(source string, destination string) (string, error) {
	if source == "" || destination == "" {
		return "", fmt.Errorf("source and destination are required")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", fmt.Errorf("create version dir: %w", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(destination), ".binary-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp binary: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	from, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open source binary: %w", err)
	}
	defer from.Close()
	if _, err := io.Copy(tempFile, from); err != nil {
		return "", fmt.Errorf("copy source binary: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return "", fmt.Errorf("sync temp binary: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("close temp binary: %w", err)
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod temp binary: %w", err)
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return "", fmt.Errorf("install binary: %w", err)
	}
	cleanup = false
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		return "", err
	}
	return destination, nil
}

func switchCurrentBinary(installedBinary string, currentBinaryPath string) error {
	if installedBinary == "" || currentBinaryPath == "" {
		return fmt.Errorf("installed and current binary paths are required")
	}
	if err := os.MkdirAll(filepath.Dir(currentBinaryPath), 0o700); err != nil {
		return fmt.Errorf("create current bin dir: %w", err)
	}
	if runtime.GOOS == "windows" {
		return copyCurrentBinary(installedBinary, currentBinaryPath)
	}
	return linkCurrentBinary(installedBinary, currentBinaryPath)
}

func linkCurrentBinary(installedBinary string, currentBinaryPath string) error {
	relTarget, err := filepath.Rel(filepath.Dir(currentBinaryPath), installedBinary)
	if err != nil {
		return fmt.Errorf("compute symlink target: %w", err)
	}
	tempLink := currentBinaryPath + ".tmp"
	_ = os.Remove(tempLink)
	if err := os.Symlink(relTarget, tempLink); err != nil {
		return fmt.Errorf("create current binary symlink: %w", err)
	}
	if err := os.Remove(currentBinaryPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tempLink)
		return fmt.Errorf("remove existing current binary: %w", err)
	}
	if err := os.Rename(tempLink, currentBinaryPath); err != nil {
		_ = os.Remove(tempLink)
		return fmt.Errorf("replace current binary symlink: %w", err)
	}
	return syncDirectory(filepath.Dir(currentBinaryPath))
}

func copyCurrentBinary(installedBinary string, currentBinaryPath string) error {
	source, err := os.Open(installedBinary)
	if err != nil {
		return fmt.Errorf("open installed binary: %w", err)
	}
	defer source.Close()
	tempFile, err := os.CreateTemp(filepath.Dir(currentBinaryPath), ".current-binary-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp current binary: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := io.Copy(tempFile, source); err != nil {
		return fmt.Errorf("copy installed binary: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync current binary: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close current binary: %w", err)
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		return fmt.Errorf("chmod current binary: %w", err)
	}
	if err := os.Rename(tempPath, currentBinaryPath); err != nil {
		return fmt.Errorf("replace current binary: %w", err)
	}
	cleanup = false
	return syncDirectory(filepath.Dir(currentBinaryPath))
}

func ResolveCurrentTarget(currentBinaryPath string) string {
	if currentBinaryPath == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		if fileExists(currentBinaryPath) {
			return currentBinaryPath
		}
		return ""
	}
	target, err := os.Readlink(currentBinaryPath)
	if err != nil {
		if fileExists(currentBinaryPath) {
			return currentBinaryPath
		}
		return ""
	}
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentBinaryPath), target))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func samePath(left string, right string) bool {
	return filepath.Clean(strings.TrimSpace(left)) == filepath.Clean(strings.TrimSpace(right)) &&
		strings.TrimSpace(left) != "" &&
		strings.TrimSpace(right) != ""
}
