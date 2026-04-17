package upgrader

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

type Paths struct {
	WorkspaceHomeDir  string `json:"workspace_home_dir"`
	VersionsDir       string `json:"versions_dir"`
	BinDir            string `json:"bin_dir"`
	StateDir          string `json:"state_dir"`
	ReleaseStatePath  string `json:"release_state_file"`
	UpgradeDir        string `json:"upgrade_dir"`
	LocksDir          string `json:"locks_dir"`
	LockFile          string `json:"lock_file"`
	StagingDir        string `json:"staging_dir"`
	CurrentBinaryPath string `json:"current_binary_path"`
}

func ResolvePaths(resolved *appconfig.Resolved) (Paths, error) {
	if resolved == nil {
		return Paths{}, fmt.Errorf("config is nil")
	}
	workspaceHomeDir := strings.TrimSpace(resolved.Paths.WorkspaceHomeDir)
	if workspaceHomeDir == "" {
		return Paths{}, fmt.Errorf("workspace home dir is empty")
	}
	stateDir := filepath.Join(workspaceHomeDir, "state")
	upgradeDir := filepath.Join(workspaceHomeDir, "upgrade")
	binDir := filepath.Join(workspaceHomeDir, "bin")
	return Paths{
		WorkspaceHomeDir:  workspaceHomeDir,
		VersionsDir:       filepath.Join(workspaceHomeDir, "versions"),
		BinDir:            binDir,
		StateDir:          stateDir,
		ReleaseStatePath:  filepath.Join(stateDir, "release-state.json"),
		UpgradeDir:        upgradeDir,
		LocksDir:          filepath.Join(upgradeDir, "locks"),
		LockFile:          filepath.Join(upgradeDir, "locks", "self-update.lock"),
		StagingDir:        filepath.Join(upgradeDir, "staging"),
		CurrentBinaryPath: filepath.Join(binDir, binaryName()),
	}, nil
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "awiki-cli.exe"
	}
	return "awiki-cli"
}

func installedBinaryPath(paths Paths, version string) string {
	return filepath.Join(paths.VersionsDir, strings.TrimSpace(version), binaryName())
}

func stagingArtifactPath(paths Paths, version string, artifactURL string) string {
	base := filepath.Base(strings.TrimSpace(artifactURL))
	if base == "." || base == "/" || base == "" {
		base = strings.TrimSpace(version) + ".artifact"
	}
	return filepath.Join(paths.StagingDir, strings.TrimSpace(version), base)
}

func stagingExtractDir(paths Paths, version string) string {
	return filepath.Join(paths.StagingDir, strings.TrimSpace(version), "extract")
}

func currentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}
