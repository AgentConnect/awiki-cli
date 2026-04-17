package skillbundle

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func Export(dir string, path string, all bool, includeRoot bool, clean bool) (*ExportResult, error) {
	bundle, err := Load()
	if err != nil {
		return nil, err
	}
	outputDir := strings.TrimSpace(dir)
	if outputDir == "" {
		return nil, fmt.Errorf("export dir is required")
	}
	if fileExists(outputDir) {
		return nil, fmt.Errorf("export dir points to a file: %s", outputDir)
	}
	if clean {
		if err := os.RemoveAll(outputDir); err != nil {
			return nil, fmt.Errorf("clean export dir: %w", err)
		}
	}
	if dirExists(outputDir) {
		entries, err := os.ReadDir(outputDir)
		if err != nil {
			return nil, fmt.Errorf("read export dir: %w", err)
		}
		if len(entries) > 0 && !clean {
			return nil, fmt.Errorf("export dir is not empty: %s", outputDir)
		}
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return nil, fmt.Errorf("create export dir: %w", err)
	}
	selected := make([]ChildDocMeta, 0)
	if all {
		selected = append(selected, bundle.Manifest.Children...)
	} else {
		meta, ok := lookupChild(bundle, path)
		if !ok {
			return nil, fmt.Errorf("skill path not found: %s", path)
		}
		selected = append(selected, meta)
	}
	result := &ExportResult{Dir: outputDir, ExportedDocs: make([]string, 0, len(selected))}
	if includeRoot {
		rootContent, _, err := RenderRootSkill(bundle)
		if err != nil {
			return nil, err
		}
		rootPath := filepath.Join(outputDir, bundle.Manifest.RootSkill.Name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(rootPath), 0o700); err != nil {
			return nil, fmt.Errorf("create export root dir: %w", err)
		}
		if err := os.WriteFile(rootPath, []byte(rootContent), 0o600); err != nil {
			return nil, fmt.Errorf("write exported root skill: %w", err)
		}
		result.ExportedRoot = true
	}
	for _, child := range selected {
		raw, err := fs.ReadFile(bundle.FS, child.Path)
		if err != nil {
			return nil, fmt.Errorf("read embedded skill doc %s: %w", child.Path, err)
		}
		targetPath := filepath.Join(outputDir, bundle.Manifest.RootSkill.Name, child.Path)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			return nil, fmt.Errorf("create export doc dir: %w", err)
		}
		if err := os.WriteFile(targetPath, raw, 0o600); err != nil {
			return nil, fmt.Errorf("write exported skill doc: %w", err)
		}
		result.ExportedDocs = append(result.ExportedDocs, child.Path)
	}
	manifestPath := filepath.Join(outputDir, "skill-manifest.json")
	manifestRaw, err := json.MarshalIndent(bundle.Manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal export skill manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestRaw, 0o600); err != nil {
		return nil, fmt.Errorf("write export skill manifest: %w", err)
	}
	result.ManifestPath = manifestPath
	return result, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
