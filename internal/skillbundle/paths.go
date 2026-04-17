package skillbundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

type Paths struct {
	WorkspaceHomeDir string `json:"workspace_home_dir"`
	StateDir         string `json:"state_dir"`
	SkillStatePath   string `json:"skill_state_file"`
	CacheDir         string `json:"cache_dir"`
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
	return Paths{
		WorkspaceHomeDir: workspaceHomeDir,
		StateDir:         stateDir,
		SkillStatePath:   filepath.Join(stateDir, "skill-state.json"),
		CacheDir:         filepath.Join(workspaceHomeDir, "cache", "skill"),
	}, nil
}

func DefaultSkillRootDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".agents", "skills"), nil
}

func cacheFilePath(paths Paths, bundle *Bundle, docPath string) string {
	return filepath.Join(paths.CacheDir, bundle.Manifest.CLIVersion, filepath.Clean(docPath))
}
