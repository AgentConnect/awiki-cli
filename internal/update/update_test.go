package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestCheckUsesServiceAPIAndBearerWhenJWTExists(t *testing.T) {
	workspace := t.TempDir()
	paths := testPaths(workspace)
	if err := os.MkdirAll(paths.CacheDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(cache) error = %v", err)
	}

	manager := identity.NewManager(paths)
	if _, err := manager.Save(identity.SaveInput{
		IdentityName: "alice",
		DID:          "did:wba:awiki.ai:user:alice",
		UniqueID:     "e1_alice",
		JWTToken:     "token-123",
	}); err != nil {
		t.Fatalf("manager.Save() error = %v", err)
	}

	var gotAuth string
	var gotDID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotDID, _ = payload["current_did"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":1,"latest_version":"1.8.1","min_supported_version":"1.8.0","channel":"stable","artifact":{"url":"https://downloads.example.com/awiki-cli.tar.gz","sha256":"abc123"},"skill_bundle":{"bundle_version":"2026.04.17","bundle_sha256":"bundle-1","root_skill_sha256":"root-1"}}`))
	}))
	defer server.Close()

	originalVersion := buildinfo.Version
	defer func() { buildinfo.Version = originalVersion }()
	buildinfo.Version = "1.8.0"

	decision, err := Check(&appconfig.Resolved{
		Paths:          paths,
		ServiceBaseURL: server.URL,
		ActiveIdentity: "alice",
		UpdateChannel:  "stable",
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if gotAuth != "Bearer token-123" {
		t.Fatalf("Authorization header = %q, want Bearer token-123", gotAuth)
	}
	if gotDID != "did:wba:awiki.ai:user:alice" {
		t.Fatalf("current_did = %q, want did:wba:awiki.ai:user:alice", gotDID)
	}
	if !decision.HasNewerVersion {
		t.Fatal("decision.HasNewerVersion = false, want true")
	}
	if !decision.ArtifactAvailable {
		t.Fatal("decision.ArtifactAvailable = false, want true")
	}
	if decision.Source != "network" {
		t.Fatalf("decision.Source = %q, want network", decision.Source)
	}
}

func TestCheckFallsBackToAnonymousWhenJWTMissing(t *testing.T) {
	workspace := t.TempDir()
	paths := testPaths(workspace)
	if err := os.MkdirAll(paths.CacheDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(cache) error = %v", err)
	}

	manager := identity.NewManager(paths)
	if _, err := manager.Save(identity.SaveInput{
		IdentityName: "alice",
		DID:          "did:wba:awiki.ai:user:alice",
		UniqueID:     "e1_alice",
	}); err != nil {
		t.Fatalf("manager.Save() error = %v", err)
	}

	var gotAuth string
	var gotDID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotDID, _ = payload["current_did"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":1,"latest_version":"1.8.0","min_supported_version":"1.8.0"}`))
	}))
	defer server.Close()

	originalVersion := buildinfo.Version
	defer func() { buildinfo.Version = originalVersion }()
	buildinfo.Version = "1.8.0"

	_, err := Check(&appconfig.Resolved{
		Paths:          paths,
		ServiceBaseURL: server.URL,
		ActiveIdentity: "alice",
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization header = %q, want empty string", gotAuth)
	}
	if gotDID != "did:wba:awiki.ai:user:alice" {
		t.Fatalf("current_did = %q, want did:wba:awiki.ai:user:alice", gotDID)
	}
}

func TestCheckUsesStaleCacheWhenServiceFails(t *testing.T) {
	workspace := t.TempDir()
	paths := testPaths(workspace)
	cacheFile := filepath.Join(paths.CacheDir, "update", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	cached := Metadata{
		LatestVersion:       "1.8.2",
		MinSupportedVersion: "1.8.0",
		RetrievedAt:         time.Now().UTC().Add(-2 * time.Hour),
		Source:              "network",
	}
	raw, err := json.Marshal(cached)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(cacheFile, raw, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	originalVersion := buildinfo.Version
	defer func() { buildinfo.Version = originalVersion }()
	buildinfo.Version = "1.8.0"

	decision, err := Check(&appconfig.Resolved{
		Paths:                         paths,
		ServiceBaseURL:                "http://127.0.0.1:1",
		UpdateMetadataCacheTTLSeconds: 1,
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !decision.HasNewerVersion {
		t.Fatal("decision.HasNewerVersion = false, want true")
	}
	if decision.Source != "cache_stale" {
		t.Fatalf("decision.Source = %q, want cache_stale", decision.Source)
	}
}

func testPaths(workspace string) appconfig.Paths {
	return appconfig.Paths{
		WorkspaceHomeDir: workspace,
		RootDir:          workspace,
		ConfigDir:        workspace,
		DataDir:          filepath.Join(workspace, "data"),
		StateDir:         filepath.Join(workspace, "runtime"),
		CacheDir:         filepath.Join(workspace, "cache"),
		LogsDir:          filepath.Join(workspace, "logs"),
		ConfigFile:       filepath.Join(workspace, "config.yaml"),
		IdentityDir:      filepath.Join(workspace, "identities"),
		DatabaseFile:     filepath.Join(workspace, "data", "awiki-cli.db"),
	}
}
