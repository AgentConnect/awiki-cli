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
	for _, table := range []string{"contacts", "contact_handle_bindings", "messages", "e2ee_outbox", "groups", "group_members", "relationship_events", "e2ee_sessions"} {
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

func TestEnsureSchemaUpgradesV11AndBackfillsContactHandleBindings(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	for _, script := range []string{v6TablesSQL, v7TablesSQL, v8TablesSQL, v11TablesSQL} {
		if _, err := db.ExecContext(ctx, script); err != nil {
			t.Fatalf("ExecContext(schema script) error = %v", err)
		}
	}
	if err := setSchemaVersion(ctx, db, 11); err != nil {
		t.Fatalf("setSchemaVersion() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO contacts
    (owner_did, did, handle, first_seen_at, last_seen_at, metadata)
VALUES (?, ?, ?, ?, ?, ?)`,
		"did:owner",
		"did:peer-old",
		"alice",
		"2026-01-01T00:00:00Z",
		"2026-01-02T00:00:00Z",
		`{"source":"test"}`,
	); err != nil {
		t.Fatalf("insert contacts error = %v", err)
	}

	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	version, err := CurrentSchemaVersion(db)
	if err != nil {
		t.Fatalf("CurrentSchemaVersion() error = %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("schema version mismatch after upgrade: got=%d want=%d", version, SchemaVersion)
	}
	row := db.QueryRowContext(ctx, `
SELECT handle, did, is_current
FROM contact_handle_bindings
WHERE owner_did = ? AND handle = ?`,
		"did:owner",
		"alice",
	)
	var (
		handle    string
		did       string
		isCurrent int
	)
	if err := row.Scan(&handle, &did, &isCurrent); err != nil {
		t.Fatalf("Scan(contact_handle_bindings) error = %v", err)
	}
	if handle != "alice" || did != "did:peer-old" || isCurrent != 1 {
		t.Fatalf("upgraded binding = (%q, %q, %d), want (alice, did:peer-old, 1)", handle, did, isCurrent)
	}
}
