package upgrader

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrUpgradeLocked = errors.New("self update is already running")

type lockMetadata struct {
	PID       int    `json:"pid"`
	Version   string `json:"version,omitempty"`
	StartedAt string `json:"started_at"`
}

func AcquireLock(path string, version string) (func() error, error) {
	if path == "" {
		return nil, fmt.Errorf("lock path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	meta := lockMetadata{PID: os.Getpid(), Version: version, StartedAt: nowUTC().Format(timeFormat())}
	if err := tryCreateLock(path, meta); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		stale, staleErr := staleLock(path)
		if staleErr != nil {
			return nil, staleErr
		}
		if !stale {
			return nil, fmt.Errorf("%w: %s", ErrUpgradeLocked, path)
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale lock: %w", err)
		}
		if err := tryCreateLock(path, meta); err != nil {
			if errors.Is(err, os.ErrExist) {
				return nil, fmt.Errorf("%w: %s", ErrUpgradeLocked, path)
			}
			return nil, err
		}
	}
	return func() error {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove lock: %w", err)
		}
		return nil
	}, nil
}

func tryCreateLock(path string, metadata lockMetadata) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	raw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lock: %w", err)
	}
	if _, err := file.Write(raw); err != nil {
		return fmt.Errorf("write lock: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync lock: %w", err)
	}
	return syncDirectory(filepath.Dir(path))
}

func staleLock(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read lock: %w", err)
	}
	var metadata lockMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return true, nil
	}
	if metadata.PID <= 0 {
		return true, nil
	}
	return !processAlive(metadata.PID), nil
}

func timeFormat() string {
	return "20060102T150405Z"
}
