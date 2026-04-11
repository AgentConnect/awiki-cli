package store

import (
	"context"
	"os"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func RebindLocalIdentityState(ctx context.Context, paths appconfig.Paths, oldOwnerDID string, newOwnerDID string) (map[string]int64, map[string]int64, error) {
	storeRebind := map[string]int64{
		"messages":            0,
		"contacts":            0,
		"relationship_events": 0,
		"groups":              0,
		"group_members":       0,
	}
	e2eeCleanup := map[string]int64{
		"e2ee_outbox":   0,
		"e2ee_sessions": 0,
	}
	if !storeFileExists(paths.DatabaseFile) {
		return storeRebind, e2eeCleanup, nil
	}

	db, err := Open(paths)
	if err != nil {
		return storeRebind, e2eeCleanup, err
	}
	defer db.Close()

	storeRebind, err = RebindOwnerDID(ctx, db, oldOwnerDID, newOwnerDID)
	if err != nil {
		return storeRebind, e2eeCleanup, err
	}
	e2eeCleanup, err = ClearOwnerE2EEData(ctx, db, oldOwnerDID)
	if err != nil {
		return storeRebind, e2eeCleanup, err
	}
	return storeRebind, e2eeCleanup, nil
}

func storeFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
