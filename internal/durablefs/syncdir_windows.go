//go:build windows

package durablefs

// SyncDirectory is intentionally a no-op on Windows.
//
// The Unix durable-write pattern fsyncs the parent directory after atomic
// rename, but Windows directory handles commonly reject Sync with "Access is
// denied". Cross-platform CLI tools therefore treat parent-dir sync as a
// Unix-only durability step.
func SyncDirectory(path string) error {
	_ = path
	return nil
}
