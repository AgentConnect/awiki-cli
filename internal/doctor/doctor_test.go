package doctor

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/message"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/upgrade"
)

func TestRunReturnsStableReportContracts(t *testing.T) {
	originalCGOEnabled := buildinfo.CGOEnabled
	buildinfo.CGOEnabled = "0"
	t.Cleanup(func() {
		buildinfo.CGOEnabled = originalCGOEnabled
	})

	cases := []struct {
		name         string
		prepare      func(t *testing.T) *appconfig.Resolved
		wantSummary  string
		wantCounts   Counts
		wantStatuses map[string]string
		verify       func(t *testing.T, report Report)
	}{
		{
			name: "empty workspace keeps warnings and info separated",
			prepare: func(t *testing.T) *appconfig.Resolved {
				resolved := resolveDoctorConfig(t, false)
				resolved.EnvHits = nil
				return resolved
			},
			wantSummary: "Doctor found warnings",
			wantCounts: Counts{
				OK:    4,
				Warn:  2,
				Error: 0,
				Info:  4,
			},
			wantStatuses: map[string]string{
				"build":             "ok",
				"config_file":       "warn",
				"environment":       "info",
				"anp_service":       "ok",
				"runtime":           "ok",
				"identity_store":    "warn",
				"sqlite":            "info",
				"anp_mls":           "info",
				"workspace_upgrade": "ok",
				"legacy_paths":      "info",
			},
			verify: func(t *testing.T, report Report) {
				sqliteCheck := checkByName(t, report, "sqlite")
				if sqliteCheck.Details["contact_handle_bindings_exists"] != false {
					t.Fatalf("sqlite contact_handle_bindings_exists = %#v, want false", sqliteCheck.Details["contact_handle_bindings_exists"])
				}
				if sqliteCheck.Details["contact_handle_bindings_count"] != 0 {
					t.Fatalf("sqlite contact_handle_bindings_count = %#v, want 0", sqliteCheck.Details["contact_handle_bindings_count"])
				}
			},
		},
		{
			name: "initialized workspace reports healthy summary and stable details",
			prepare: func(t *testing.T) *appconfig.Resolved {
				resolved := resolveDoctorConfig(t, true)
				if err := appconfig.WriteFileConfig(resolved.Paths.ConfigFile, appconfig.FileConfig{}); err != nil {
					t.Fatalf("WriteFileConfig() error = %v", err)
				}
				resolved = resolveDoctorConfigForWorkspace(t, resolved.Paths.WorkspaceHomeDir, true)

				manager := identity.NewManager(resolved.Paths)
				if _, err := manager.Save(identity.SaveInput{
					IdentityName: "alice",
					DID:          "did:wba:alice.example",
					UniqueID:     "e1_alice",
					UserID:       "user-1",
					Handle:       "alice",
				}); err != nil {
					t.Fatalf("manager.Save() error = %v", err)
				}

				db, err := store.Open(resolved.Paths)
				if err != nil {
					t.Fatalf("store.Open() error = %v", err)
				}
				defer db.Close()
				if err := store.EnsureSchema(context.Background(), db); err != nil {
					t.Fatalf("EnsureSchema() error = %v", err)
				}
				if _, err := db.Exec(`
INSERT INTO contact_handle_bindings (
	owner_did,
	handle,
	did,
	first_seen_at,
	last_seen_at,
	credential_name
) VALUES (?, ?, ?, ?, ?, ?)
`, "did:wba:alice.example", "bob", "did:wba:bob.example", "2026-04-18T09:00:00Z", "2026-04-18T09:00:00Z", "alice"); err != nil {
					t.Fatalf("insert contact_handle_bindings error = %v", err)
				}

				if err := upgrade.SaveMeta(upgrade.ResolvePaths(resolved).MetaPath, upgrade.Meta{
					WorkspaceSchemaVersion: upgrade.LatestWorkspaceSchemaVersion,
					AppVersion:             "test",
					UpdatedAt:              time.Now().UTC().Format(time.RFC3339),
				}); err != nil {
					t.Fatalf("SaveMeta() error = %v", err)
				}
				return resolved
			},
			wantSummary: "Doctor completed successfully",
			wantCounts: Counts{
				OK:    8,
				Warn:  0,
				Error: 0,
				Info:  2,
			},
			wantStatuses: map[string]string{
				"build":             "ok",
				"config_file":       "ok",
				"environment":       "ok",
				"anp_service":       "ok",
				"runtime":           "ok",
				"identity_store":    "ok",
				"sqlite":            "ok",
				"anp_mls":           "info",
				"workspace_upgrade": "ok",
				"legacy_paths":      "info",
			},
			verify: func(t *testing.T, report Report) {
				identityCheck := checkByName(t, report, "identity_store")
				current, ok := identityCheck.Details["default_identity"].(*identity.IdentitySummary)
				if !ok {
					t.Fatalf("default_identity type = %T, want *identity.IdentitySummary", identityCheck.Details["default_identity"])
				}
				if current.IdentityName != "alice" {
					t.Fatalf("default identity name = %q, want %q", current.IdentityName, "alice")
				}
				if !current.UserState.ReadyForMessaging {
					t.Fatalf("default identity ready_for_messaging = %t, want true", current.UserState.ReadyForMessaging)
				}

				sqliteCheck := checkByName(t, report, "sqlite")
				if sqliteCheck.Details["contact_handle_bindings_exists"] != true {
					t.Fatalf("sqlite contact_handle_bindings_exists = %#v, want true", sqliteCheck.Details["contact_handle_bindings_exists"])
				}
				if sqliteCheck.Details["contact_handle_bindings_count"] != 1 {
					t.Fatalf("sqlite contact_handle_bindings_count = %#v, want 1", sqliteCheck.Details["contact_handle_bindings_count"])
				}
			},
		},
		{
			name: "invalid config and ANP inputs surface blocking issues",
			prepare: func(t *testing.T) *appconfig.Resolved {
				resolved := resolveDoctorConfig(t, false)
				if err := os.MkdirAll(filepath.Dir(resolved.Paths.ConfigFile), 0o700); err != nil {
					t.Fatalf("MkdirAll(config dir) error = %v", err)
				}
				if err := os.WriteFile(resolved.Paths.ConfigFile, []byte("schema_version: [\n"), 0o600); err != nil {
					t.Fatalf("WriteFile(invalid config) error = %v", err)
				}
				resolved = resolveDoctorConfigForWorkspace(t, resolved.Paths.WorkspaceHomeDir, false)
				resolved.EnvHits = nil
				resolved.ANPServiceEndpoint = "https://127.0.0.1/anp-im/rpc"
				resolved.ANPServiceDID = "did:key:z6Mkwrong"
				return resolved
			},
			wantSummary: "Doctor found blocking issues",
			wantCounts: Counts{
				OK:    2,
				Warn:  2,
				Error: 2,
				Info:  4,
			},
			wantStatuses: map[string]string{
				"build":             "ok",
				"config_file":       "error",
				"environment":       "info",
				"anp_service":       "error",
				"runtime":           "ok",
				"identity_store":    "warn",
				"sqlite":            "info",
				"anp_mls":           "info",
				"workspace_upgrade": "warn",
				"legacy_paths":      "info",
			},
			verify: func(t *testing.T, report Report) {
				configCheck := checkByName(t, report, "config_file")
				if configCheck.Details["error"] == "" {
					t.Fatal("config_file details missing parse error")
				}
				anpCheck := checkByName(t, report, "anp_service")
				if anpCheck.Details["endpoint_error"] == nil {
					t.Fatal("anp_service details missing endpoint_error")
				}
				if anpCheck.Details["service_did_error"] == nil {
					t.Fatal("anp_service details missing service_did_error")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := Run(tc.prepare(t))
			if report.Summary != tc.wantSummary {
				t.Fatalf("Run().Summary = %q, want %q", report.Summary, tc.wantSummary)
			}
			if report.Counts != tc.wantCounts {
				t.Fatalf("Run().Counts = %#v, want %#v", report.Counts, tc.wantCounts)
			}
			if len(report.Checks) != len(tc.wantStatuses) {
				t.Fatalf("len(Run().Checks) = %d, want %d", len(report.Checks), len(tc.wantStatuses))
			}
			for name, wantStatus := range tc.wantStatuses {
				check := checkByName(t, report, name)
				if check.Status != wantStatus {
					t.Fatalf("check %q status = %q, want %q", name, check.Status, wantStatus)
				}
				if check.Summary == "" {
					t.Fatalf("check %q summary is empty", name)
				}
				if check.Details == nil {
					t.Fatalf("check %q details are nil", name)
				}
			}
			if tc.verify != nil {
				tc.verify(t, report)
			}
		})
	}
}

func resolveDoctorConfig(t *testing.T, keepEnvHits bool) *appconfig.Resolved {
	t.Helper()
	return resolveDoctorConfigForWorkspace(t, t.TempDir(), keepEnvHits)
}

func resolveDoctorConfigForWorkspace(t *testing.T, workspace string, keepEnvHits bool) *appconfig.Resolved {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspace)
	t.Setenv("AWIKI_ANP_MLS_BINARY", "")
	t.Setenv("PATH", t.TempDir())
	resolved, err := appconfig.Resolve(appconfig.Overrides{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	resolved.RuntimeMode = runtimecfg.ModeHTTP
	resolved.Paths.LegacyCredentialsDir = filepath.Join(workspace, "legacy-credentials")
	resolved.Paths.LegacyDataDir = filepath.Join(workspace, "legacy-data")
	if !keepEnvHits {
		resolved.EnvHits = nil
	}
	return resolved
}

func checkByName(t *testing.T, report Report, name string) Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("check %q not found", name)
	return Check{}
}

func TestANPMLSDoctorCompatibilityAndStateDiagnostics(t *testing.T) {
	resolved := resolveDoctorConfig(t, false)
	binDir := t.TempDir()
	writeFakeANPMLS(t, binDir, `{"ok":true,"api_version":"anp-mls/v1","request_id":"doctor-system-version","result":{"api_version":"anp-mls/v1","binary_name":"anp-mls","binary_version":"test","supported_commands":["system version","key-package generate","group create"]}}`)
	t.Setenv("PATH", binDir)
	mlsDir := filepath.Join(resolved.Paths.WorkspaceHomeDir, "mls")
	if err := os.MkdirAll(mlsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mlsDir, "state.db"), []byte("sqlite placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mlsDir, "state.lock"), []byte("lock"), 0o600); err != nil {
		t.Fatal(err)
	}

	report := Run(resolved)
	check := checkByName(t, report, "anp_mls")
	if check.Status != "ok" {
		t.Fatalf("anp_mls status = %q, want ok; details=%#v", check.Status, check.Details)
	}
	if check.Details["data_dir_status"] != "ok" || check.Details["state_db_status"] != "ok" {
		t.Fatalf("unexpected MLS state details: %#v", check.Details)
	}
	version, ok := check.Details["version"].(*message.MLSVersionInfo)
	if !ok || version.BinaryName != "anp-mls" {
		t.Fatalf("version detail = %#v", check.Details["version"])
	}
	if check.Details["remediation"] != "No action required." {
		t.Fatalf("remediation = %#v", check.Details["remediation"])
	}
}

func TestANPMLSDoctorVersionMismatchIsActionableWarning(t *testing.T) {
	resolved := resolveDoctorConfig(t, false)
	binDir := t.TempDir()
	writeFakeANPMLS(t, binDir, `{"ok":true,"api_version":"anp-mls/v0","request_id":"doctor-system-version","result":{"api_version":"anp-mls/v0","binary_name":"anp-mls","binary_version":"old","supported_commands":["system version"]}}`)
	t.Setenv("PATH", binDir)

	report := Run(resolved)
	check := checkByName(t, report, "anp_mls")
	if check.Status != "warn" {
		t.Fatalf("anp_mls status = %q, want warn; details=%#v", check.Status, check.Details)
	}
	if got := check.Details["compatibility_error"]; !strings.Contains(got.(string), "api_version") {
		t.Fatalf("compatibility_error = %#v", got)
	}
	if got := check.Details["remediation"].(string); !strings.Contains(got, "api_version anp-mls/v1") {
		t.Fatalf("remediation = %q", got)
	}
}

func TestANPMLSDoctorWarnsWhenCachedE2EEGroupsHaveNoMLSState(t *testing.T) {
	resolved := resolveDoctorConfig(t, false)
	binDir := t.TempDir()
	writeFakeANPMLS(t, binDir, `{"ok":true,"api_version":"anp-mls/v1","request_id":"doctor-system-version","result":{"api_version":"anp-mls/v1","binary_name":"anp-mls","binary_version":"test","supported_commands":["system version"]}}`)
	t.Setenv("PATH", binDir)
	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertGroup(context.Background(), db, store.GroupRecord{
		OwnerDID:       "did:wba:alice.example",
		GroupID:        "group-1",
		Name:           "Secret group",
		Metadata:       `{"message_security_profile":"group-e2ee"}`,
		CredentialName: "alice",
	}); err != nil {
		t.Fatal(err)
	}

	report := Run(resolved)
	check := checkByName(t, report, "anp_mls")
	if check.Status != "warn" {
		t.Fatalf("anp_mls status = %q, want warn; details=%#v", check.Status, check.Details)
	}
	if check.Details["e2ee_group_count"] != 1 {
		t.Fatalf("e2ee_group_count = %#v, want 1", check.Details["e2ee_group_count"])
	}
	if check.Details["data_dir_status"] != "warn_missing_with_cached_groups" {
		t.Fatalf("data_dir_status = %#v", check.Details["data_dir_status"])
	}
}

func TestANPMLSDoctorAcceptsAgentDeviceScopedState(t *testing.T) {
	resolved := resolveDoctorConfig(t, false)
	binDir := t.TempDir()
	writeFakeANPMLS(t, binDir, `{"ok":true,"api_version":"anp-mls/v1","request_id":"doctor-system-version","result":{"api_version":"anp-mls/v1","binary_name":"anp-mls","binary_version":"test","supported_commands":["system version"]}}`)
	t.Setenv("PATH", binDir)
	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertGroup(context.Background(), db, store.GroupRecord{
		OwnerDID:       "did:wba:alice.example",
		GroupID:        "group-1",
		Name:           "Secret group",
		Metadata:       `{"message_security_profile":"group-e2ee"}`,
		CredentialName: "alice",
	}); err != nil {
		t.Fatal(err)
	}
	scopedDir := filepath.Join(
		resolved.Paths.WorkspaceHomeDir,
		"mls",
		"agents",
		testMLSAgentKey("did:wba:alice.example"),
		"laptop",
	)
	if err := os.MkdirAll(scopedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scopedDir, "state.db"), []byte("sqlite placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scopedDir, "state.lock"), []byte("lock"), 0o600); err != nil {
		t.Fatal(err)
	}

	report := Run(resolved)
	check := checkByName(t, report, "anp_mls")
	if check.Status != "ok" {
		t.Fatalf("anp_mls status = %q, want ok; details=%#v", check.Status, check.Details)
	}
	if check.Details["state_db_status"] != "missing" {
		t.Fatalf("root state_db_status = %#v, want missing when scoped state exists", check.Details["state_db_status"])
	}
	if check.Details["scoped_state_db_count"] != 1 {
		t.Fatalf("scoped_state_db_count = %#v, want 1", check.Details["scoped_state_db_count"])
	}
	if check.Details["scoped_state_lock_count"] != 1 {
		t.Fatalf("scoped_state_lock_count = %#v, want 1", check.Details["scoped_state_lock_count"])
	}
	scoped, ok := check.Details["scoped_states"].([]mlsScopedStateInspection)
	if !ok || len(scoped) != 1 {
		t.Fatalf("scoped_states = %#v, want one scoped state", check.Details["scoped_states"])
	}
	if scoped[0].AgentKey != testMLSAgentKey("did:wba:alice.example") || scoped[0].DeviceID != "laptop" {
		t.Fatalf("scoped state identity = %#v", scoped[0])
	}
	if check.Details["remediation"] != "No action required." {
		t.Fatalf("remediation = %#v", check.Details["remediation"])
	}
}

func TestANPMLSDoctorWarnsOnStaleScopedLock(t *testing.T) {
	resolved := resolveDoctorConfig(t, false)
	binDir := t.TempDir()
	writeFakeANPMLS(t, binDir, `{"ok":true,"api_version":"anp-mls/v1","request_id":"doctor-system-version","result":{"api_version":"anp-mls/v1","binary_name":"anp-mls","binary_version":"test","supported_commands":["system version"]}}`)
	t.Setenv("PATH", binDir)
	scopedDir := filepath.Join(
		resolved.Paths.WorkspaceHomeDir,
		"mls",
		"agents",
		testMLSAgentKey("did:wba:alice.example"),
		"default",
	)
	if err := os.MkdirAll(scopedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scopedDir, "state.db"), []byte("sqlite placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(scopedDir, "state.lock")
	if err := os.WriteFile(lockPath, []byte("lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}

	report := Run(resolved)
	check := checkByName(t, report, "anp_mls")
	if check.Status != "warn" {
		t.Fatalf("anp_mls status = %q, want warn; details=%#v", check.Status, check.Details)
	}
	if check.Details["scoped_state_warning_count"] != 1 {
		t.Fatalf("scoped_state_warning_count = %#v, want 1", check.Details["scoped_state_warning_count"])
	}
	scoped := check.Details["scoped_states"].([]mlsScopedStateInspection)
	if scoped[0].StateLockStatus != "warn_stale_candidate" {
		t.Fatalf("scoped state lock status = %#v, want stale warning", scoped[0])
	}
	if got := check.Details["remediation"].(string); !strings.Contains(got, "agent/device-scoped state.lock") {
		t.Fatalf("remediation = %q", got)
	}
}

func writeFakeANPMLS(t *testing.T, dir string, response string) string {
	t.Helper()
	if os.PathSeparator == ';' {
		t.Skip("shell-script fake anp-mls is only used on Unix-like test hosts")
	}
	path := filepath.Join(dir, "anp-mls")
	script := "#!/bin/sh\n" +
		"if [ \"$1 $2 $3 $4\" != \"system version --json-in -\" ]; then echo unexpected args >&2; exit 2; fi\n" +
		"printf '%s' '" + strings.ReplaceAll(response, "'", "'\\''") + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func testMLSAgentKey(agentDID string) string {
	sum := sha256.Sum256([]byte(agentDID))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:24]
}
