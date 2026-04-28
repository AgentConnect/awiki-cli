package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func pathOverridePaths(path string) appconfig.Paths {
	return appconfig.Paths{DatabaseFile: path}
}

func TestImportLegacyDatabaseV11(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	targetPaths := appconfig.Paths{
		DatabaseFile:         filepath.Join(root, "current.db"),
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy-credentials"),
		LegacyDataDir:        filepath.Join(root, "legacy-data"),
	}
	legacyDBPath := filepath.Join(targetPaths.LegacyDataDir, "database", "awiki.db")
	if err := os.MkdirAll(filepath.Dir(legacyDBPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	legacyDB, err := Open(pathOverridePaths(legacyDBPath))
	if err != nil {
		t.Fatalf("Open(legacy) error = %v", err)
	}
	defer legacyDB.Close()
	if err := EnsureSchema(context.Background(), legacyDB); err != nil {
		t.Fatalf("EnsureSchema(legacy) error = %v", err)
	}
	if err := StoreMessage(context.Background(), legacyDB, MessageRecord{
		MsgID:          "legacy-msg",
		OwnerDID:       "did:wba:awiki.ai:user:legacy",
		ThreadID:       MakeThreadID("did:wba:awiki.ai:user:legacy", "did:wba:awiki.ai:user:peer", ""),
		Direction:      0,
		SenderDID:      "did:wba:awiki.ai:user:peer",
		ReceiverDID:    "did:wba:awiki.ai:user:legacy",
		ContentType:    "text",
		Content:        "legacy hello",
		CredentialName: "legacy",
	}); err != nil {
		t.Fatalf("StoreMessage(legacy) error = %v", err)
	}
	if err := UpsertContact(context.Background(), legacyDB, ContactRecord{
		OwnerDID: "did:wba:awiki.ai:user:legacy",
		DID:      "did:wba:awiki.ai:user:peer",
		Name:     "Legacy Peer",
	}); err != nil {
		t.Fatalf("UpsertContact(legacy) error = %v", err)
	}

	manager := identity.NewManager(targetPaths)
	generated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:   "legacy",
		DID:            "did:wba:awiki.ai:user:legacy",
		UniqueID:       generated.UniqueID,
		DisplayName:    "Legacy",
		DIDDocument:    generated.DIDDocument,
		Key1PrivatePEM: generated.Key1PrivatePEM,
		Key1PublicPEM:  generated.Key1PublicPEM,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	targetDB, err := Open(targetPaths)
	if err != nil {
		t.Fatalf("Open(target) error = %v", err)
	}
	defer targetDB.Close()
	report, err := ImportLegacyDatabase(context.Background(), targetDB, targetPaths, manager)
	if err != nil {
		t.Fatalf("ImportLegacyDatabase() error = %v", err)
	}
	if report.ImportedRows["messages"] != 1 || report.ImportedRows["contacts"] != 1 {
		raw, _ := json.MarshalIndent(report, "", "  ")
		t.Fatalf("unexpected import report: %s", raw)
	}
	row, err := GetMessageByID(context.Background(), targetDB, "legacy-msg", "did:wba:awiki.ai:user:legacy", "")
	if err != nil {
		t.Fatalf("GetMessageByID(target) error = %v", err)
	}
	if row["content"] != "legacy hello" {
		t.Fatalf("unexpected imported message: %#v", row)
	}
}

func TestImportLegacyDatabaseSkipsMissingTablesAndInfersOwnerFromCredential(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := appconfig.Paths{
		DatabaseFile:         filepath.Join(root, "current.db"),
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy-credentials"),
		LegacyDataDir:        filepath.Join(root, "legacy-data"),
	}
	legacyDBPath := filepath.Join(paths.LegacyDataDir, "database", "awiki.db")
	if err := os.MkdirAll(filepath.Dir(legacyDBPath), 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	legacyDB, err := Open(pathOverridePaths(legacyDBPath))
	if err != nil {
		t.Fatalf("Open(legacy) error = %v", err)
	}
	defer legacyDB.Close()
	if _, err := legacyDB.ExecContext(context.Background(), v6TablesSQL); err != nil {
		t.Fatalf("ExecContext(v6TablesSQL) error = %v", err)
	}
	if err := setSchemaVersion(context.Background(), legacyDB, 6); err != nil {
		t.Fatalf("setSchemaVersion() error = %v", err)
	}
	if _, err := legacyDB.ExecContext(context.Background(), `
INSERT INTO messages
    (msg_id, owner_did, thread_id, direction, sender_did, receiver_did, content_type, content, is_read, credential_name, stored_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy-msg",
		"",
		"",
		0,
		"did:peer",
		"",
		"",
		"legacy hello",
		0,
		"legacy",
		"2026-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("ExecContext(insert messages) error = %v", err)
	}
	if _, err := legacyDB.ExecContext(context.Background(), `
INSERT INTO contacts
    (owner_did, did, handle, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?, ?)`,
		"",
		"did:peer",
		"alice",
		"2026-01-01T00:00:00Z",
		"2026-01-02T00:00:00Z",
	); err != nil {
		t.Fatalf("ExecContext(insert contacts) error = %v", err)
	}

	manager := identity.NewManager(paths)
	generated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"legacy"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("identity.GenerateIdentity() error = %v", err)
	}
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:   "legacy",
		DID:            "did:owner",
		UniqueID:       generated.UniqueID,
		DisplayName:    "Legacy",
		DIDDocument:    generated.DIDDocument,
		Key1PrivatePEM: generated.Key1PrivatePEM,
		Key1PublicPEM:  generated.Key1PublicPEM,
	}); err != nil {
		t.Fatalf("manager.Save() error = %v", err)
	}

	targetDB, err := Open(paths)
	if err != nil {
		t.Fatalf("Open(target) error = %v", err)
	}
	defer targetDB.Close()
	report, err := ImportLegacyDatabase(context.Background(), targetDB, paths, manager)
	if err != nil {
		t.Fatalf("ImportLegacyDatabase() error = %v", err)
	}
	if report.ImportedRows["messages"] != 1 || report.ImportedRows["contacts"] != 1 {
		t.Fatalf("report.ImportedRows = %#v, want messages=1 contacts=1", report.ImportedRows)
	}
	if len(report.SkippedTables) != 4 {
		t.Fatalf("report.SkippedTables = %#v, want 4 missing post-v6 tables", report.SkippedTables)
	}
	row, err := GetMessageByID(context.Background(), targetDB, "legacy-msg", "did:owner", "")
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if row["thread_id"] != MakeThreadID("did:owner", "did:peer", "") {
		t.Fatalf("row[thread_id] = %#v, want inferred direct thread id", row["thread_id"])
	}
	if row["content_type"] != "text" {
		t.Fatalf("row[content_type] = %#v, want default text", row["content_type"])
	}
	handle, err := ResolveContactHandleByDID(context.Background(), targetDB, "did:owner", "did:peer")
	if err != nil {
		t.Fatalf("ResolveContactHandleByDID() error = %v", err)
	}
	if handle != "alice" {
		t.Fatalf("ResolveContactHandleByDID() = %q, want alice", handle)
	}
}

func TestImportLegacyDatabaseRejectsPreV6SchemaWithoutImportedIdentity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	legacyDBPath := filepath.Join(root, "legacy-v5.db")
	legacyDB, err := Open(pathOverridePaths(legacyDBPath))
	if err != nil {
		t.Fatalf("Open(legacy) error = %v", err)
	}
	if err := setSchemaVersion(context.Background(), legacyDB, 5); err != nil {
		t.Fatalf("setSchemaVersion() error = %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("legacyDB.Close() error = %v", err)
	}

	paths := appconfig.Paths{
		DatabaseFile:  filepath.Join(root, "target.db"),
		LegacyDataDir: legacyDBPath,
	}
	targetDB, err := Open(paths)
	if err != nil {
		t.Fatalf("Open(target) error = %v", err)
	}
	defer targetDB.Close()

	report, err := ImportLegacyDatabase(context.Background(), targetDB, paths, nil)
	if !errors.Is(err, ErrUnsupportedLegacySchema) {
		t.Fatalf("ImportLegacyDatabase() error = %v, want ErrUnsupportedLegacySchema", err)
	}
	if report != nil {
		t.Fatalf("ImportLegacyDatabase() report = %#v, want nil", report)
	}
}
