package upgrader

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/update"
)

func TestResolvePathsUsesWorkspaceHome(t *testing.T) {
	workspace := t.TempDir()
	paths, err := ResolvePaths(&appconfig.Resolved{Paths: appconfig.Paths{WorkspaceHomeDir: workspace}})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if paths.VersionsDir != filepath.Join(workspace, "versions") {
		t.Fatalf("paths.VersionsDir = %q", paths.VersionsDir)
	}
	if paths.CurrentBinaryPath != filepath.Join(workspace, "bin", binaryName()) {
		t.Fatalf("paths.CurrentBinaryPath = %q", paths.CurrentBinaryPath)
	}
	if paths.ReleaseStatePath != filepath.Join(workspace, "state", "release-state.json") {
		t.Fatalf("paths.ReleaseStatePath = %q", paths.ReleaseStatePath)
	}
}

func TestRecordCheckWritesPendingUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release-state.json")
	state, err := RecordCheck(path, update.Decision{
		LatestVersion:     "1.8.1",
		ArtifactAvailable: true,
		ArtifactURL:       "https://downloads.example.com/awiki-cli.tar.gz",
		ArtifactSHA256:    "abc123",
		HasNewerVersion:   true,
		Channel:           "stable",
		Source:            "network",
	})
	if err != nil {
		t.Fatalf("RecordCheck() error = %v", err)
	}
	if state.PendingUpdate == nil {
		t.Fatal("state.PendingUpdate = nil, want value")
	}
	if state.LastCheckSource != "check_api" {
		t.Fatalf("state.LastCheckSource = %q, want check_api", state.LastCheckSource)
	}
}

func TestManagerApplyInstallsTarGZAndSwitchesCurrent(t *testing.T) {
	workspace := t.TempDir()
	resolved := &appconfig.Resolved{Paths: appconfig.Paths{WorkspaceHomeDir: workspace}}
	manager, err := NewManager(resolved)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	archivePath := filepath.Join(t.TempDir(), "awiki-cli.tar.gz")
	binaryContent := []byte("#!/bin/sh\necho test\n")
	if err := writeTestTarGZ(archivePath, filepath.Join("release", binaryName()), binaryContent); err != nil {
		t.Fatalf("writeTestTarGZ() error = %v", err)
	}
	hash := sha256.Sum256(mustReadFile(t, archivePath))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, archivePath)
	}))
	defer server.Close()

	result, err := manager.Apply(context.Background(), update.Decision{
		LatestVersion:     "1.8.1",
		ArtifactAvailable: true,
		ArtifactURL:       server.URL + "/awiki-cli.tar.gz",
		ArtifactSHA256:    hex.EncodeToString(hash[:]),
		Channel:           "stable",
		Source:            "network",
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.InstalledVersion != "1.8.1" {
		t.Fatalf("result.InstalledVersion = %q", result.InstalledVersion)
	}
	if !fileExists(result.InstalledBinaryPath) {
		t.Fatalf("installed binary %q does not exist", result.InstalledBinaryPath)
	}
	if target := ResolveCurrentTarget(manager.Paths().CurrentBinaryPath); target != result.InstalledBinaryPath {
		t.Fatalf("ResolveCurrentTarget() = %q, want %q", target, result.InstalledBinaryPath)
	}
	state, err := manager.LoadReleaseState()
	if err != nil {
		t.Fatalf("LoadReleaseState() error = %v", err)
	}
	if state == nil || state.CurrentVersion != "1.8.1" {
		t.Fatalf("release state = %#v, want current_version 1.8.1", state)
	}
}

func TestManagerApplyFailsOnHashMismatch(t *testing.T) {
	workspace := t.TempDir()
	resolved := &appconfig.Resolved{Paths: appconfig.Paths{WorkspaceHomeDir: workspace}}
	manager, err := NewManager(resolved)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	archivePath := filepath.Join(t.TempDir(), "awiki-cli.tar.gz")
	if err := writeTestTarGZ(archivePath, binaryName(), []byte("binary")); err != nil {
		t.Fatalf("writeTestTarGZ() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, archivePath)
	}))
	defer server.Close()

	_, err = manager.Apply(context.Background(), update.Decision{
		LatestVersion:     "1.8.1",
		ArtifactAvailable: true,
		ArtifactURL:       server.URL + "/awiki-cli.tar.gz",
		ArtifactSHA256:    "deadbeef",
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want hash mismatch")
	}
	state, loadErr := manager.LoadReleaseState()
	if loadErr != nil {
		t.Fatalf("LoadReleaseState() error = %v", loadErr)
	}
	if state == nil || state.LastFailedUpdate == nil || state.LastFailedUpdate.Stage != "verify" {
		t.Fatalf("release state = %#v, want verify failure", state)
	}
}

func writeTestTarGZ(archivePath string, binaryPath string, content []byte) error {
	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()
	header := &tar.Header{
		Name: binaryPath,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	_, err = tarWriter.Write(content)
	return err
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open() error = %v", err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return content
}
