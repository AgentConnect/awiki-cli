package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrUpgradeLocked = errors.New("workspace upgrade is already running")

const (
	osFileLockScheme = "os_file_lock_v1"
	legacyLockMaxAge = 24 * time.Hour
)

func AcquireFileLock(path string, appVersion string) (func() error, error) {
	if path == "" {
		return nil, fmt.Errorf("workspace lock path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create upgrade lock dir: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open upgrade lock: %w", err)
	}
	cleanup := func() {
		_ = file.Close()
	}

	releaseOSLock, err := acquireOSFileLock(file)
	if err != nil {
		cleanup()
		if errors.Is(err, ErrUpgradeLocked) {
			return nil, fmt.Errorf("%w: %s", ErrUpgradeLocked, path)
		}
		return nil, fmt.Errorf("acquire OS upgrade lock: %w", err)
	}
	unlockAndClose := func() error {
		var unlockErr error
		if err := releaseOSLock(); err != nil {
			unlockErr = fmt.Errorf("release OS upgrade lock: %w", err)
		}
		if err := file.Close(); err != nil {
			closeErr := fmt.Errorf("close upgrade lock: %w", err)
			if unlockErr != nil {
				return errors.Join(unlockErr, closeErr)
			}
			return closeErr
		}
		return unlockErr
	}
	cleanupLocked := func() {
		_ = unlockAndClose()
	}

	existing, parsed, err := readLockMetadata(file)
	if err != nil {
		cleanupLocked()
		return nil, err
	}
	if parsed && isActiveLegacyLock(*existing) {
		cleanupLocked()
		return nil, fmt.Errorf("%w: %s", ErrUpgradeLocked, path)
	}

	metadata := newLockMetadata(appVersion)
	if err := writeLockMetadata(file, metadata); err != nil {
		cleanupLocked()
		return nil, err
	}

	return unlockAndClose, nil
}

func newLockMetadata(appVersion string) lockMetadata {
	hostname, _ := os.Hostname()
	executable, _ := os.Executable()
	return lockMetadata{
		LockScheme: osFileLockScheme,
		PID:        os.Getpid(),
		AppVersion: appVersion,
		StartedAt:  nowUTC().Format(timeLayout),
		Hostname:   hostname,
		Executable: executable,
	}
}

func readLockMetadata(file *os.File) (*lockMetadata, bool, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, false, fmt.Errorf("seek upgrade lock: %w", err)
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, false, fmt.Errorf("read upgrade lock: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, false, fmt.Errorf("rewind upgrade lock: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, false, nil
	}
	var metadata lockMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, false, nil
	}
	return &metadata, true, nil
}

func writeLockMetadata(file *os.File, metadata lockMetadata) error {
	raw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal upgrade lock: %w", err)
	}
	raw = append(raw, '\n')
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate upgrade lock: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind upgrade lock: %w", err)
	}
	if _, err := file.Write(raw); err != nil {
		return fmt.Errorf("write upgrade lock: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync upgrade lock: %w", err)
	}
	return nil
}

func isActiveLegacyLock(metadata lockMetadata) bool {
	if metadata.LockScheme == osFileLockScheme {
		return false
	}
	if metadata.PID <= 0 {
		return false
	}
	if !processAlive(metadata.PID) {
		return false
	}
	startedAt, ok := parseLockStartedAt(metadata.StartedAt)
	if !ok {
		return false
	}
	if age := nowUTC().Sub(startedAt); age > legacyLockMaxAge {
		return false
	}
	return true
}

func parseLockStartedAt(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []string{timeLayout, time.RFC3339}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}
