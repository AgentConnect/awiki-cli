package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
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
				Info:  3,
			},
			wantStatuses: map[string]string{
				"build":             "ok",
				"config_file":       "warn",
				"environment":       "info",
				"anp_service":       "ok",
				"runtime":           "ok",
				"identity_store":    "warn",
				"sqlite":            "info",
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
				Info:  1,
			},
			wantStatuses: map[string]string{
				"build":             "ok",
				"config_file":       "ok",
				"environment":       "ok",
				"anp_service":       "ok",
				"runtime":           "ok",
				"identity_store":    "ok",
				"sqlite":            "ok",
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
				Info:  3,
			},
			wantStatuses: map[string]string{
				"build":             "ok",
				"config_file":       "error",
				"environment":       "info",
				"anp_service":       "error",
				"runtime":           "ok",
				"identity_store":    "warn",
				"sqlite":            "info",
				"workspace_upgrade": "warn",
				"legacy_paths":      "info",
			},
			verify: func(t *testing.T, report Report) {
				configCheck := checkByName(t, report, "config_file")
				if configCheck.Details["error"] == "" {
					t.Fatal("config_file details missing parse error")
				}
				anpCheck := checkByName(t, report, "anp_service")
				if len(anpServiceDiagnostics(t, anpCheck)) < 2 {
					t.Fatalf("anp_service diagnostics = %#v, want endpoint and service DID errors", anpCheck.Details["diagnostics"])
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

func TestANPServiceCheckWarnsForAdvancedHostedGateway(t *testing.T) {
	resolved := resolveDoctorConfig(t, false)
	resolved.ServiceBaseURL = "https://api.a.example.com"
	resolved.DIDDomain = "a.example.com"
	resolved.ANPServiceEndpoint = "https://gateway.a.example.com/anp-im/rpc"
	resolved.ANPServiceDID = "did:wba:service.a.example.com"

	check := anpServiceCheck(resolved)
	if check.Status != "warn" {
		t.Fatalf("anpServiceCheck().Status = %q, want warn", check.Status)
	}
	diagnostics := anpServiceDiagnostics(t, check)
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want two warnings", diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity != appconfig.ServiceSeverityWarn {
			t.Fatalf("diagnostic severity = %q, want warn: %#v", diagnostic.Severity, diagnostic)
		}
	}
}

func TestANPServiceCheckErrorsForInvalidHostedConfig(t *testing.T) {
	resolved := resolveDoctorConfig(t, false)
	resolved.ServiceBaseURL = "ftp://api.example.com"
	resolved.DIDDomain = "https://a.example.com"
	resolved.ANPServiceEndpoint = "http://127.0.0.1/anp-im/rpc"
	resolved.ANPServiceDID = "did:wba:a.example.com:services:message:e1"

	check := anpServiceCheck(resolved)
	if check.Status != "error" {
		t.Fatalf("anpServiceCheck().Status = %q, want error", check.Status)
	}
	if len(anpServiceDiagnostics(t, check)) < 4 {
		t.Fatalf("diagnostics = %#v, want blocking diagnostics", check.Details["diagnostics"])
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

func anpServiceDiagnostics(t *testing.T, check Check) []appconfig.ServiceDiagnostic {
	t.Helper()
	diagnostics, ok := check.Details["diagnostics"].([]appconfig.ServiceDiagnostic)
	if !ok {
		t.Fatalf("diagnostics type = %T, want []config.ServiceDiagnostic", check.Details["diagnostics"])
	}
	return diagnostics
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
