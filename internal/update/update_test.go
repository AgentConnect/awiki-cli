package update

import (
	"encoding/json"
	"os"
	"path/filepath"
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
