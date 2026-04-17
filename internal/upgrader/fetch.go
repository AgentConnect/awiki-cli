package upgrader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func DownloadArtifact(ctx context.Context, client *http.Client, artifactURL string, destination string) error {
	if artifactURL == "" {
		return fmt.Errorf("artifact url is required")
	}
	if destination == "" {
		return fmt.Errorf("artifact destination is required")
	}
	if client == nil {
		client = &http.Client{}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return fmt.Errorf("build artifact request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download artifact: http status %d", resp.StatusCode)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(destination), ".artifact-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp artifact file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		return fmt.Errorf("write artifact file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync artifact file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close artifact file: %w", err)
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return fmt.Errorf("move artifact file: %w", err)
	}
	cleanup = false
	return syncDirectory(filepath.Dir(destination))
}
