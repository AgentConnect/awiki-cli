package cli

import "strings"

const windowsDirSyncCompatibilityHint = "This looks like a Windows workspace directory-sync compatibility failure inside awiki-cli rather than a normal write-permission problem. Upgrade to the latest awiki-cli build with the Windows durable-write fix; running as Administrator usually should not be necessary."

func refineWorkspaceWriteHint(err error, fallback string) string {
	if !isWindowsDirSyncCompatibilityError(err) {
		return fallback
	}
	return windowsDirSyncCompatibilityHint
}

func isWindowsDirSyncCompatibilityError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "access is denied") {
		return false
	}
	for _, marker := range []string{
		"sync config dir",
		"sync route registry dir",
		"sync dir",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
