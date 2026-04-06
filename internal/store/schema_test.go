package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	root := t.TempDir()
	db, err := Open(appconfig.Paths{DatabaseFile: filepath.Join(root, "awiki-cli.db")})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestEnsureSchemaCreatesVersionAndTables(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	version, err := CurrentSchemaVersion(db)
	if err != nil {
		t.Fatalf("CurrentSchemaVersion() error = %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("schema version mismatch: got=%d want=%d", version, SchemaVersion)
	}
	for _, table := range []string{"contacts", "messages", "e2ee_outbox", "groups", "group_members", "relationship_events", "e2ee_sessions"} {
		exists, err := tableExists(ctx, db, table)
		if err != nil {
			t.Fatalf("tableExists(%s) error = %v", table, err)
		}
		if !exists {
			t.Fatalf("expected table %s to exist", table)
		}
	}
	for _, view := range []string{"threads", "inbox", "outbox"} {
		exists, err := viewExists(ctx, db, view)
		if err != nil {
			t.Fatalf("viewExists(%s) error = %v", view, err)
		}
		if !exists {
			t.Fatalf("expected view %s to exist", view)
		}
	}
}
