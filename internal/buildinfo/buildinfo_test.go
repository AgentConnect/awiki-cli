package buildinfo

import (
	"runtime"
	"testing"
)

func TestCurrentReturnsInjectedMetadata(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	originalCGOEnabled := CGOEnabled
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
		CGOEnabled = originalCGOEnabled
	})

	cases := []struct {
		name       string
		version    string
		commit     string
		buildDate  string
		cgoEnabled string
	}{
		{
			name:       "release build metadata",
			version:    "1.2.3",
			commit:     "abc1234",
			buildDate:  "2026-04-18T09:00:00Z",
			cgoEnabled: "0",
		},
		{
			name:       "dev build metadata",
			version:    "dev-main",
			commit:     "unknown",
			buildDate:  "unknown",
			cgoEnabled: "true",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Version = tc.version
			Commit = tc.commit
			BuildDate = tc.buildDate
			CGOEnabled = tc.cgoEnabled

			info := Current()
			if info.Version != tc.version {
				t.Fatalf("Current().Version = %q, want %q", info.Version, tc.version)
			}
			if info.Commit != tc.commit {
				t.Fatalf("Current().Commit = %q, want %q", info.Commit, tc.commit)
			}
			if info.BuildDate != tc.buildDate {
				t.Fatalf("Current().BuildDate = %q, want %q", info.BuildDate, tc.buildDate)
			}
			if info.CGOEnabled != tc.cgoEnabled {
				t.Fatalf("Current().CGOEnabled = %q, want %q", info.CGOEnabled, tc.cgoEnabled)
			}
			if info.GoVersion != runtime.Version() {
				t.Fatalf("Current().GoVersion = %q, want %q", info.GoVersion, runtime.Version())
			}
			if info.GOOS != runtime.GOOS {
				t.Fatalf("Current().GOOS = %q, want %q", info.GOOS, runtime.GOOS)
			}
			if info.GOARCH != runtime.GOARCH {
				t.Fatalf("Current().GOARCH = %q, want %q", info.GOARCH, runtime.GOARCH)
			}
			if info.Compiler != runtime.Compiler {
				t.Fatalf("Current().Compiler = %q, want %q", info.Compiler, runtime.Compiler)
			}
		})
	}
}

func TestCurrentReturnsIndependentSnapshot(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	originalCGOEnabled := CGOEnabled
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
		CGOEnabled = originalCGOEnabled
	})

	Version = "before"
	Commit = "commit-before"
	BuildDate = "date-before"
	CGOEnabled = "0"

	info := Current()

	Version = "after"
	Commit = "commit-after"
	BuildDate = "date-after"
	CGOEnabled = "1"

	if info.Version != "before" {
		t.Fatalf("snapshot Version = %q, want %q", info.Version, "before")
	}
	if info.Commit != "commit-before" {
		t.Fatalf("snapshot Commit = %q, want %q", info.Commit, "commit-before")
	}
	if info.BuildDate != "date-before" {
		t.Fatalf("snapshot BuildDate = %q, want %q", info.BuildDate, "date-before")
	}
	if info.CGOEnabled != "0" {
		t.Fatalf("snapshot CGOEnabled = %q, want %q", info.CGOEnabled, "0")
	}
}
