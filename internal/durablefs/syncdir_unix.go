//go:build !windows

package durablefs

import (
	"fmt"
	"os"
)

// SyncDirectory fsyncs a directory on platforms that support the standard
// Unix durable-rename pattern.
func SyncDirectory(path string) error {
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
