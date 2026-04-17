package upgrader

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ExtractArtifact(archivePath string, destinationDir string) (string, error) {
	if archivePath == "" {
		return "", fmt.Errorf("archive path is required")
	}
	if destinationDir == "" {
		return "", fmt.Errorf("extract destination is required")
	}
	if err := os.RemoveAll(destinationDir); err != nil {
		return "", fmt.Errorf("clean extract dir: %w", err)
	}
	if err := os.MkdirAll(destinationDir, 0o700); err != nil {
		return "", fmt.Errorf("create extract dir: %w", err)
	}
	switch {
	case strings.HasSuffix(strings.ToLower(archivePath), ".zip"):
		if err := extractZIP(archivePath, destinationDir); err != nil {
			return "", err
		}
	case strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz"), strings.HasSuffix(strings.ToLower(archivePath), ".tgz"):
		if err := extractTarGZ(archivePath, destinationDir); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported artifact archive format: %s", archivePath)
	}
	return findExtractedBinary(destinationDir)
}

func extractZIP(path string, destinationDir string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		targetPath, err := safeExtractPath(destinationDir, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o700); err != nil {
				return fmt.Errorf("create zip dir: %w", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			return fmt.Errorf("create zip parent dir: %w", err)
		}
		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("open zip entry: %w", err)
		}
		mode := file.Mode()
		if mode == 0 {
			mode = 0o600
		}
		if err := writeFileFromReader(targetPath, rc, mode); err != nil {
			_ = rc.Close()
			return err
		}
		_ = rc.Close()
	}
	return nil
}

func extractTarGZ(path string, destinationDir string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open tar.gz archive: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open gzip reader: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}
		targetPath, err := safeExtractPath(destinationDir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o700); err != nil {
				return fmt.Errorf("create tar dir: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
				return fmt.Errorf("create tar parent dir: %w", err)
			}
			mode := os.FileMode(header.Mode)
			if mode == 0 {
				mode = 0o600
			}
			if err := writeFileFromReader(targetPath, tarReader, mode); err != nil {
				return err
			}
		default:
			continue
		}
	}
}

func safeExtractPath(destinationDir string, entryName string) (string, error) {
	cleanDest := filepath.Clean(destinationDir)
	targetPath := filepath.Join(cleanDest, entryName)
	cleanTarget := filepath.Clean(targetPath)
	prefix := cleanDest + string(os.PathSeparator)
	if cleanTarget != cleanDest && !strings.HasPrefix(cleanTarget, prefix) {
		return "", fmt.Errorf("unsafe archive path: %s", entryName)
	}
	return cleanTarget, nil
}

func writeFileFromReader(path string, reader io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open extracted file: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("write extracted file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync extracted file: %w", err)
	}
	return nil
}

func findExtractedBinary(destinationDir string) (string, error) {
	targetName := binaryName()
	matches := make([]string, 0, 1)
	err := filepath.Walk(destinationDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == targetName {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("scan extracted archive: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("artifact archive does not contain %s", targetName)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("artifact archive contains multiple %s binaries", targetName)
	}
	return matches[0], nil
}
