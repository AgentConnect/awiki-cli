package upgrade

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireFileLockLeavesPersistentMetadata(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "upgrade", "upgrade.lock")
	unlock, err := AcquireFileLock(lockPath, "1.2.3")
	if err != nil {
		t.Fatalf("AcquireFileLock() error = %v", err)
	}
	metadata := readUpgradeLockMetadataForTest(t, lockPath)
	if metadata.LockScheme != osFileLockScheme {
		t.Fatalf("lock scheme = %q, want %q", metadata.LockScheme, osFileLockScheme)
	}
	if metadata.PID != os.Getpid() {
		t.Fatalf("lock pid = %d, want %d", metadata.PID, os.Getpid())
	}
	if metadata.AppVersion != "1.2.3" {
		t.Fatalf("lock app version = %q", metadata.AppVersion)
	}
	if metadata.Hostname == "" {
		t.Fatal("lock hostname should be populated")
	}
	if metadata.Executable == "" {
		t.Fatal("lock executable should be populated")
	}

	if err := unlock(); err != nil {
		t.Fatalf("unlock() error = %v", err)
	}
	if !fileExists(lockPath) {
		t.Fatal("upgrade.lock should remain as a persistent OS lock anchor")
	}

	unlock, err = AcquireFileLock(lockPath, "1.2.4")
	if err != nil {
		t.Fatalf("AcquireFileLock() after unlock error = %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("second unlock() error = %v", err)
	}
}

func TestAcquireFileLockRejectsConcurrentOSLock(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "upgrade", "upgrade.lock")
	unlock, err := AcquireFileLock(lockPath, "1.2.3")
	if err != nil {
		t.Fatalf("AcquireFileLock() error = %v", err)
	}
	defer func() {
		if err := unlock(); err != nil {
			t.Fatalf("unlock() error = %v", err)
		}
	}()

	secondUnlock, err := AcquireFileLock(lockPath, "1.2.3")
	if err == nil {
		_ = secondUnlock()
		t.Fatal("second AcquireFileLock() error = nil, want ErrUpgradeLocked")
	}
	if !errors.Is(err, ErrUpgradeLocked) {
		t.Fatalf("second AcquireFileLock() error = %v, want ErrUpgradeLocked", err)
	}
}

func TestAcquireFileLockIgnoresResidualOSLockMetadata(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "upgrade", "upgrade.lock")
	writeUpgradeLockMetadataForTest(t, lockPath, lockMetadata{
		LockScheme: osFileLockScheme,
		PID:        missingPIDForTest(),
		AppVersion: "old",
		StartedAt:  nowUTC().Add(-48 * time.Hour).Format(timeLayout),
	})

	unlock, err := AcquireFileLock(lockPath, "new")
	if err != nil {
		t.Fatalf("AcquireFileLock() error = %v", err)
	}
	defer func() {
		if err := unlock(); err != nil {
			t.Fatalf("unlock() error = %v", err)
		}
	}()

	metadata := readUpgradeLockMetadataForTest(t, lockPath)
	if metadata.LockScheme != osFileLockScheme {
		t.Fatalf("lock scheme = %q, want %q", metadata.LockScheme, osFileLockScheme)
	}
	if metadata.AppVersion != "new" {
		t.Fatalf("lock app version = %q, want new", metadata.AppVersion)
	}
	if metadata.PID != os.Getpid() {
		t.Fatalf("lock pid = %d, want %d", metadata.PID, os.Getpid())
	}
}

func TestAcquireFileLockIgnoresCorruptLegacyLock(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "upgrade", "upgrade.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("not-json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	unlock, err := AcquireFileLock(lockPath, "1.2.3")
	if err != nil {
		t.Fatalf("AcquireFileLock() error = %v", err)
	}
	defer func() {
		if err := unlock(); err != nil {
			t.Fatalf("unlock() error = %v", err)
		}
	}()

	metadata := readUpgradeLockMetadataForTest(t, lockPath)
	if metadata.LockScheme != osFileLockScheme {
		t.Fatalf("lock scheme = %q, want %q", metadata.LockScheme, osFileLockScheme)
	}
}

func TestAcquireFileLockIgnoresDeadLegacyPID(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "upgrade", "upgrade.lock")
	writeUpgradeLockMetadataForTest(t, lockPath, lockMetadata{
		PID:        missingPIDForTest(),
		AppVersion: "legacy",
		StartedAt:  nowUTC().Format(timeLayout),
	})

	unlock, err := AcquireFileLock(lockPath, "1.2.3")
	if err != nil {
		t.Fatalf("AcquireFileLock() error = %v", err)
	}
	defer func() {
		if err := unlock(); err != nil {
			t.Fatalf("unlock() error = %v", err)
		}
	}()

	metadata := readUpgradeLockMetadataForTest(t, lockPath)
	if metadata.LockScheme != osFileLockScheme {
		t.Fatalf("lock scheme = %q, want %q", metadata.LockScheme, osFileLockScheme)
	}
}

func TestAcquireFileLockIgnoresOldLegacyLivePID(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "upgrade", "upgrade.lock")
	writeUpgradeLockMetadataForTest(t, lockPath, lockMetadata{
		PID:        os.Getpid(),
		AppVersion: "legacy",
		StartedAt:  nowUTC().Add(-(legacyLockMaxAge + time.Hour)).Format(timeLayout),
	})

	unlock, err := AcquireFileLock(lockPath, "1.2.3")
	if err != nil {
		t.Fatalf("AcquireFileLock() error = %v", err)
	}
	defer func() {
		if err := unlock(); err != nil {
			t.Fatalf("unlock() error = %v", err)
		}
	}()

	metadata := readUpgradeLockMetadataForTest(t, lockPath)
	if metadata.LockScheme != osFileLockScheme {
		t.Fatalf("lock scheme = %q, want %q", metadata.LockScheme, osFileLockScheme)
	}
}

func TestAcquireFileLockRejectsRecentLegacyLivePID(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "upgrade", "upgrade.lock")
	writeUpgradeLockMetadataForTest(t, lockPath, lockMetadata{
		PID:        os.Getpid(),
		AppVersion: "legacy",
		StartedAt:  nowUTC().Format(timeLayout),
	})

	unlock, err := AcquireFileLock(lockPath, "1.2.3")
	if err == nil {
		_ = unlock()
		t.Fatal("AcquireFileLock() error = nil, want ErrUpgradeLocked")
	}
	if !errors.Is(err, ErrUpgradeLocked) {
		t.Fatalf("AcquireFileLock() error = %v, want ErrUpgradeLocked", err)
	}
	metadata := readUpgradeLockMetadataForTest(t, lockPath)
	if metadata.LockScheme != "" {
		t.Fatalf("legacy lock should not be overwritten, scheme = %q", metadata.LockScheme)
	}
	if metadata.AppVersion != "legacy" {
		t.Fatalf("legacy lock should not be overwritten, app version = %q", metadata.AppVersion)
	}
}

func writeUpgradeLockMetadataForTest(t *testing.T, path string, metadata lockMetadata) {
	t.Helper()
	raw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func readUpgradeLockMetadataForTest(t *testing.T, path string) lockMetadata {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var metadata lockMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("Unmarshal() error = %v; raw = %s", err, raw)
	}
	return metadata
}

func missingPIDForTest() int {
	for pid := 999999; pid > 1; pid-- {
		if !processAlive(pid) {
			return pid
		}
	}
	return -1
}
