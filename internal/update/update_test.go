package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestCompareVersionsHandlesNumericPrereleaseSegments(t *testing.T) {
	t.Parallel()

	cmp, ok := compareVersions("0.0.1-beta.9", "0.0.1-beta.10")
	if !ok {
		t.Fatal("compareVersions returned ok=false")
	}
	if cmp >= 0 {
		t.Fatalf("compareVersions(beta.9, beta.10) = %d, want < 0", cmp)
	}

	cmp, ok = compareVersions("0.0.1-beta.10", "0.0.1-beta.9")
	if !ok {
		t.Fatal("compareVersions returned ok=false")
	}
	if cmp <= 0 {
		t.Fatalf("compareVersions(beta.10, beta.9) = %d, want > 0", cmp)
	}
}

func TestCheckBlocksWhenCurrentVersionFallsBelowMinimumSupportedPrerelease(t *testing.T) {
	originalVersion := buildinfo.Version
	buildinfo.Version = "0.0.1-beta.9"
	t.Cleanup(func() {
		buildinfo.Version = originalVersion
	})

	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "update", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	meta := Metadata{
		LatestVersion:       "0.0.1-beta.10",
		MinSupportedVersion: "0.0.1-beta.10",
		RetrievedAt:         time.Now().UTC(),
		Source:              "cache",
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(cacheFile, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	resolved := &appconfig.Resolved{
		Paths: appconfig.Paths{
			CacheDir: cacheDir,
		},
	}

	decision, err := Check(resolved)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !decision.Blocked {
		t.Fatalf("Check() blocked = false, want true; decision = %+v", decision)
	}
}

func TestFetchFromRegistryURLsFallsBackToNpmmirror(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		switch r.URL.Path {
		case "/npmjs":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/npmmirror":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"1.0.9","awikiCli":{"minSupportedVersion":"1.0.8"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	meta, err := fetchFromRegistryURLs(context.Background(), server.Client(), []string{
		server.URL + "/npmjs",
		server.URL + "/npmmirror",
	})
	if err != nil {
		t.Fatalf("fetchFromRegistryURLs() error = %v", err)
	}
	if meta.LatestVersion != "1.0.9" {
		t.Fatalf("meta.LatestVersion = %q, want %q", meta.LatestVersion, "1.0.9")
	}
	if meta.MinSupportedVersion != "1.0.8" {
		t.Fatalf("meta.MinSupportedVersion = %q, want %q", meta.MinSupportedVersion, "1.0.8")
	}
	wantRequests := []string{"/npmjs", "/npmmirror"}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Fatalf("requests = %#v, want %#v", requests, wantRequests)
	}
}

func TestFetchFromRegistryURLsReturnsCombinedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := fetchFromRegistryURLs(context.Background(), server.Client(), []string{
		server.URL + "/npmjs",
		server.URL + "/npmmirror",
	})
	if err == nil {
		t.Fatal("fetchFromRegistryURLs() error = nil, want combined failure")
	}
	if !strings.Contains(err.Error(), server.URL+"/npmjs") {
		t.Fatalf("error = %q, want substring %q", err.Error(), server.URL+"/npmjs")
	}
	if !strings.Contains(err.Error(), server.URL+"/npmmirror") {
		t.Fatalf("error = %q, want substring %q", err.Error(), server.URL+"/npmmirror")
	}
}

func TestCheckFreshPrefersNetworkOverFreshCache(t *testing.T) {
	originalURLs := append([]string(nil), npmLatestURLs...)
	npmLatestURLs = nil
	t.Cleanup(func() {
		npmLatestURLs = originalURLs
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1.0.10","awikiCli":{"minSupportedVersion":"1.0.9"}}`))
	}))
	defer server.Close()
	npmLatestURLs = []string{server.URL}

	originalVersion := buildinfo.Version
	buildinfo.Version = "1.0.9"
	t.Cleanup(func() {
		buildinfo.Version = originalVersion
	})

	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "update", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	meta := Metadata{
		LatestVersion:       "1.0.9",
		MinSupportedVersion: "1.0.9",
		RetrievedAt:         time.Now().UTC(),
		Source:              "cache",
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(cacheFile, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	resolved := &appconfig.Resolved{
		Paths: appconfig.Paths{
			CacheDir: cacheDir,
		},
	}

	decision, err := CheckFresh(context.Background(), resolved)
	if err != nil {
		t.Fatalf("CheckFresh() error = %v", err)
	}
	if decision.LatestVersion != "1.0.10" {
		t.Fatalf("decision.LatestVersion = %q, want %q", decision.LatestVersion, "1.0.10")
	}
	if decision.MetadataSource != "network" {
		t.Fatalf("decision.MetadataSource = %q, want %q", decision.MetadataSource, "network")
	}
	if !decision.HasNewerVersion {
		t.Fatalf("decision.HasNewerVersion = false, want true; decision = %+v", decision)
	}
}

func TestCheckFreshFallsBackToStaleCache(t *testing.T) {
	originalURLs := append([]string(nil), npmLatestURLs...)
	npmLatestURLs = nil
	t.Cleanup(func() {
		npmLatestURLs = originalURLs
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	npmLatestURLs = []string{server.URL}

	originalVersion := buildinfo.Version
	buildinfo.Version = "1.0.9"
	t.Cleanup(func() {
		buildinfo.Version = originalVersion
	})

	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "update", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	meta := Metadata{
		LatestVersion:       "1.0.10",
		MinSupportedVersion: "1.0.9",
		RetrievedAt:         time.Now().UTC(),
		Source:              "cache",
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(cacheFile, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	resolved := &appconfig.Resolved{
		Paths: appconfig.Paths{
			CacheDir: cacheDir,
		},
	}

	decision, err := CheckFresh(context.Background(), resolved)
	if err != nil {
		t.Fatalf("CheckFresh() error = %v", err)
	}
	if decision.MetadataSource != "cache_stale" {
		t.Fatalf("decision.MetadataSource = %q, want %q", decision.MetadataSource, "cache_stale")
	}
	if decision.LatestVersion != "1.0.10" {
		t.Fatalf("decision.LatestVersion = %q, want %q", decision.LatestVersion, "1.0.10")
	}
	if !decision.HasNewerVersion {
		t.Fatalf("decision.HasNewerVersion = false, want true; decision = %+v", decision)
	}
}
