package durablefs

import (
	"path/filepath"
	goruntime "runtime"
	"testing"
)

func TestSyncDirectoryExistingDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := SyncDirectory(dir); err != nil {
		t.Fatalf("SyncDirectory(existing) error = %v", err)
	}
}

func TestSyncDirectoryMissingDirBehaviorByPlatform(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing")
	err := SyncDirectory(path)
	if goruntime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("SyncDirectory(missing) on windows error = %v, want nil", err)
		}
		return
	}
	if err == nil {
		t.Fatal("SyncDirectory(missing) on non-windows error = nil, want error")
	}
}
