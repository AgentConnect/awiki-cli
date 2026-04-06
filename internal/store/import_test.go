package store

import (
	"context"
	"encoding/json"
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
