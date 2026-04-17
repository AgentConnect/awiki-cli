package skillbundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestLoadBuildsBundleFromEmbeddedSkills(t *testing.T) {
	bundle, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if bundle.Manifest.RootSkill.Name != "awiki-cli" {
		t.Fatalf("bundle.Manifest.RootSkill.Name = %q, want awiki-cli", bundle.Manifest.RootSkill.Name)
	}
	if len(bundle.Manifest.Children) == 0 {
		t.Fatal("bundle.Manifest.Children = 0, want embedded references")
	}
	for _, child := range bundle.Manifest.Children {
		if child.Kind != "reference" {
			t.Fatalf("child.Kind = %q, want reference", child.Kind)
		}
		if !strings.HasPrefix(child.Path, "references/") {
			t.Fatalf("child.Path = %q, want references/ prefix", child.Path)
		}
		if child.Path == "references/onboarding.zh.md" {
			t.Fatal("localized onboarding reference should not be published in the manifest")
		}
	}
}

func TestRenderRootSkillIncludesBootstrapSection(t *testing.T) {
	originalVersion := buildinfo.Version
	defer func() { buildinfo.Version = originalVersion }()
	buildinfo.Version = "1.8.1"

	bundle, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	rendered, _, err := RenderRootSkill(bundle)
	if err != nil {
		t.Fatalf("RenderRootSkill() error = %v", err)
	}
	for _, needle := range []string{
		"name: awiki-cli",
		"npm install -g @awiki/cli",
		"awiki-cli skill index --json",
		"https://awiki.ai/skills/awiki-cli/SKILL.md",
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("rendered root skill missing %q", needle)
		}
	}
}

func TestGetReturnsEmbeddedReferenceAndWritesCache(t *testing.T) {
	workspace := t.TempDir()
	resolved := &appconfig.Resolved{Paths: appconfig.Paths{WorkspaceHomeDir: workspace}}
	result, err := Get(resolved, "references/03-messaging.md", "auto", "")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Path != "references/03-messaging.md" {
		t.Fatalf("result.Path = %q", result.Path)
	}
	if !strings.Contains(result.Content, "msg") {
		t.Fatalf("result.Content does not look like the messaging reference")
	}
	paths, err := ResolvePaths(resolved)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	cachePath := cacheFilePath(paths, mustBundle(t), result.Path)
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file stat error = %v", err)
	}
}

func TestSyncRootSkillWritesManagedFiles(t *testing.T) {
	workspace := t.TempDir()
	resolved := &appconfig.Resolved{Paths: appconfig.Paths{WorkspaceHomeDir: workspace}}
	targetDir := filepath.Join(t.TempDir(), "skills")
	result, err := SyncRootSkill(resolved, targetDir, false, false)
	if err != nil {
		t.Fatalf("SyncRootSkill() error = %v", err)
	}
	if !result.WroteFiles {
		t.Fatal("result.WroteFiles = false, want true")
	}
	raw, err := os.ReadFile(result.SkillPath)
	if err != nil {
		t.Fatalf("ReadFile(skill) error = %v", err)
	}
	if !strings.Contains(string(raw), "npm install -g @awiki/cli") {
		t.Fatalf("root skill content missing bootstrap instructions")
	}
	managedRaw, err := os.ReadFile(result.ManagedPath)
	if err != nil {
		t.Fatalf("ReadFile(managed) error = %v", err)
	}
	var managed ManagedFile
	if err := json.Unmarshal(managedRaw, &managed); err != nil {
		t.Fatalf("json.Unmarshal(managed) error = %v", err)
	}
	if managed.SkillName != "awiki-cli" {
		t.Fatalf("managed.SkillName = %q, want awiki-cli", managed.SkillName)
	}
	paths, err := ResolvePaths(resolved)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	state, err := LoadSkillState(paths.SkillStatePath)
	if err != nil {
		t.Fatalf("LoadSkillState() error = %v", err)
	}
	if state == nil || state.RootSkillSync.LastSyncStatus != "ok" {
		t.Fatalf("skill state = %#v, want sync status ok", state)
	}
}

func TestSyncRootSkillReportsConflictForModifiedManagedFile(t *testing.T) {
	workspace := t.TempDir()
	resolved := &appconfig.Resolved{Paths: appconfig.Paths{WorkspaceHomeDir: workspace}}
	targetDir := filepath.Join(t.TempDir(), "skills")
	first, err := SyncRootSkill(resolved, targetDir, false, false)
	if err != nil {
		t.Fatalf("initial SyncRootSkill() error = %v", err)
	}
	if err := os.WriteFile(first.SkillPath, []byte("user modified skill"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	second, err := SyncRootSkill(resolved, targetDir, false, false)
	if err != nil {
		t.Fatalf("second SyncRootSkill() error = %v", err)
	}
	if !second.Conflict {
		t.Fatal("second.Conflict = false, want true")
	}
}

func TestExportWritesManifestAndSelectedDocs(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "export")
	result, err := Export(outputDir, "references/03-messaging.md", false, true, false)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if !result.ExportedRoot {
		t.Fatal("result.ExportedRoot = false, want true")
	}
	if len(result.ExportedDocs) != 1 || result.ExportedDocs[0] != "references/03-messaging.md" {
		t.Fatalf("result.ExportedDocs = %#v", result.ExportedDocs)
	}
	if _, err := os.Stat(result.ManifestPath); err != nil {
		t.Fatalf("manifest path stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "awiki-cli", "references", "03-messaging.md")); err != nil {
		t.Fatalf("exported reference stat error = %v", err)
	}
}

func mustBundle(t *testing.T) *Bundle {
	t.Helper()
	bundle, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return bundle
}
