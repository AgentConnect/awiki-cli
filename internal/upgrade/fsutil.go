package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func writeAtomicFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".upgrade-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	cleanup = false
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open dir: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync dir: %w", err)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create dst dir: %w", err)
	}
	from, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src file: %w", err)
	}
	defer from.Close()
	to, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open dst file: %w", err)
	}
	defer to.Close()
	if _, err := io.Copy(to, from); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	if err := to.Sync(); err != nil {
		return fmt.Errorf("sync dst file: %w", err)
	}
	return nil
}

func copyTree(src, dst string) error {
	if !dirExists(src) {
		return nil
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return fmt.Errorf("create dst tree: %w", err)
	}
	if err := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		if info.IsDir() {
			if rel == "." {
				return nil
			}
			return os.MkdirAll(targetPath, 0o700)
		}
		mode := info.Mode()
		if mode == 0 {
			mode = 0o600
		}
		return copyFile(path, targetPath, mode)
	}); err != nil {
		return fmt.Errorf("copy tree: %w", err)
	}
	return syncDirectory(filepath.Dir(dst))
}

func writeJSONFile(path string, payload any) error {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	return writeAtomicFile(path, raw, 0o600)
}
