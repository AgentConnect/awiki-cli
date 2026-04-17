package skillbundle

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func Get(resolved *appconfig.Resolved, path string, cacheMode string, outputFile string) (*GetResult, error) {
	bundle, err := Load()
	if err != nil {
		return nil, err
	}
	meta, ok := lookupChild(bundle, path)
	if !ok {
		return nil, fmt.Errorf("skill path not found: %s", path)
	}
	paths, err := ResolvePaths(resolved)
	if err != nil {
		return nil, err
	}
	mode := normalizeCacheMode(cacheMode)
	cachePath := cacheFilePath(paths, bundle, meta.Path)
	cacheHit := false
	content := ""
	if mode == "auto" {
		if raw, err := os.ReadFile(cachePath); err == nil {
			content = string(raw)
			cacheHit = true
		}
	}
	if content == "" {
		raw, err := fs.ReadFile(bundle.FS, meta.Path)
		if err != nil {
			return nil, fmt.Errorf("read embedded skill doc %s: %w", meta.Path, err)
		}
		content = string(raw)
		if mode != "bypass" {
			if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err == nil {
				_ = os.WriteFile(cachePath, []byte(content), 0o600)
			}
		}
	}
	if strings.TrimSpace(outputFile) != "" {
		if err := os.MkdirAll(filepath.Dir(outputFile), 0o700); err != nil {
			return nil, fmt.Errorf("create skill output dir: %w", err)
		}
		if err := os.WriteFile(outputFile, []byte(content), 0o600); err != nil {
			return nil, fmt.Errorf("write skill output file: %w", err)
		}
	}
	return &GetResult{
		Path:          meta.Path,
		Kind:          meta.Kind,
		CLIVersion:    bundle.Manifest.CLIVersion,
		BundleVersion: bundle.Manifest.BundleVersion,
		SHA256:        meta.SHA256,
		Content:       content,
		CacheMode:     mode,
		CacheHit:      cacheHit,
		OutputFile:    strings.TrimSpace(outputFile),
	}, nil
}

func lookupChild(bundle *Bundle, path string) (ChildDocMeta, bool) {
	needle := cleanDocPath(path)
	for _, child := range bundle.Manifest.Children {
		if child.Path == needle {
			return child, true
		}
	}
	return ChildDocMeta{}, false
}

func normalizeCacheMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", "auto":
		return "auto"
	case "refresh":
		return "refresh"
	case "bypass":
		return "bypass"
	default:
		return strings.TrimSpace(strings.ToLower(mode))
	}
}

func cleanDocPath(path string) string {
	trimmed := strings.TrimSpace(path)
	trimmed = strings.TrimPrefix(trimmed, "./")
	trimmed = filepath.ToSlash(filepath.Clean(trimmed))
	trimmed = strings.TrimPrefix(trimmed, "/")
	return trimmed
}
