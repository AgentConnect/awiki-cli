package upgrade

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
)

type workspaceV0ToV1Migration struct{}

type legacySettingsFile struct {
	UserServiceURL   string `json:"user_service_url"`
	MoltMessageURL   string `json:"molt_message_url"`
	DidDomain        string `json:"did_domain"`
	MessageTransport struct {
		ReceiveMode string `json:"receive_mode"`
	} `json:"message_transport"`
}

func newWorkspaceV0ToV1Migration() Migration {
	return workspaceV0ToV1Migration{}
}

func (workspaceV0ToV1Migration) From() int { return 0 }

func (workspaceV0ToV1Migration) To() int { return 1 }

func (workspaceV0ToV1Migration) Name() string {
	return "workspace_0_to_1_bootstrap_local_state_upgrade"
}

func (workspaceV0ToV1Migration) IsDone(ctx context.Context, uc *Context) (bool, error) {
	meta, err := LoadMeta(uc.Paths.MetaPath)
	if err != nil {
		return false, err
	}
	return meta != nil && meta.WorkspaceSchemaVersion >= 1, nil
}

func (workspaceV0ToV1Migration) Apply(ctx context.Context, uc *Context) error {
	if uc == nil || uc.Resolved == nil {
		return fmt.Errorf("workspace upgrade requires a resolved config")
	}
	detection := uc.Inspection.Detection
	if detection.ConfigExists {
		if err := appconfig.EnsureConfigSchemaVersion(uc.Paths.ConfigFile); err != nil {
			return err
		}
	} else if !detection.HasWorkspace && detection.LegacySettingsExists {
		legacyConfig, err := loadLegacySettings(uc.Paths.LegacySettingsPath)
		if err != nil {
			return err
		}
		fileConfig := appconfig.FileConfig{}
		fileConfig.SchemaVersion = appconfig.ConfigSchemaVersion
		fileConfig.Runtime.Mode = legacyConfig.RuntimeMode
		fileConfig.Services.ServiceBaseURL = legacyConfig.ServiceBaseURL
		fileConfig.Services.DIDDomain = legacyConfig.DidDomain
		if err := appconfig.WriteFileConfig(uc.Paths.ConfigFile, fileConfig); err != nil {
			return err
		}
	}

	if !detection.HasWorkspace && detection.HasLegacy {
		manager := identity.NewManager(uc.Resolved.Paths)
		legacyScan, err := manager.ScanLegacy()
		if err != nil {
			return err
		}
		if legacyScan != nil && legacyScan.HasLegacy {
			if _, err := manager.ImportAllLegacy(); err != nil {
				return err
			}
		}

		legacyDB, err := store.ScanLegacyDatabase(ctx, uc.Resolved.Paths)
		if err != nil {
			return err
		}
		if legacyDB != nil && legacyDB.Exists {
			db, err := store.Open(uc.Resolved.Paths)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := store.EnsureSchema(ctx, db); err != nil {
				return err
			}
			if _, err := store.ImportLegacyDatabase(ctx, db, uc.Resolved.Paths, manager); err != nil {
				return err
			}
		}
	}

	if fileExists(uc.Paths.DatabaseFile) {
		if err := ensureTargetStoreSchema(ctx, uc.Resolved.Paths); err != nil {
			return err
		}
	}
	return nil
}

func (workspaceV0ToV1Migration) Validate(ctx context.Context, uc *Context) error {
	if uc == nil || uc.Resolved == nil {
		return fmt.Errorf("workspace upgrade requires a resolved config")
	}
	if fileExists(uc.Paths.ConfigFile) {
		fileConfig, _, err := appconfig.ReadFileConfig(uc.Paths.ConfigFile)
		if err != nil {
			return err
		}
		if fileConfig.SchemaVersion != appconfig.ConfigSchemaVersion {
			return fmt.Errorf("config schema version = %d, want %d", fileConfig.SchemaVersion, appconfig.ConfigSchemaVersion)
		}
	}
	if fileExists(uc.Paths.DatabaseFile) {
		db, err := store.OpenReadOnly(uc.Paths.DatabaseFile)
		if err != nil {
			return err
		}
		defer db.Close()
		version, err := store.CurrentSchemaVersion(db)
		if err != nil {
			return err
		}
		if version != store.SchemaVersion {
			return fmt.Errorf("sqlite schema version = %d, want %d", version, store.SchemaVersion)
		}
		if err := validateSQLiteHealth(ctx, db); err != nil {
			return err
		}
	}
	if uc.Inspection != nil && !uc.Inspection.Detection.HasWorkspace && uc.Inspection.Detection.LegacyIdentityExists {
		manager := identity.NewManager(uc.Resolved.Paths)
		identities, err := manager.List()
		if err != nil {
			return err
		}
		if len(identities) == 0 {
			return fmt.Errorf("expected at least one imported identity after legacy upgrade")
		}
	}
	return nil
}

type normalizedLegacySettings struct {
	ServiceBaseURL string
	DidDomain      string
	RuntimeMode    string
}

func loadLegacySettings(path string) (*normalizedLegacySettings, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read legacy settings: %w", err)
	}
	var legacy legacySettingsFile
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("parse legacy settings: %w", err)
	}
	mode := runtimecfg.ModeHTTP
	if strings.EqualFold(strings.TrimSpace(legacy.MessageTransport.ReceiveMode), "websocket") {
		mode = runtimecfg.ModeWebSocket
	}
	serviceBaseURL := strings.TrimSpace(legacy.UserServiceURL)
	if serviceBaseURL == "" {
		serviceBaseURL = strings.TrimSpace(legacy.MoltMessageURL)
	}
	return &normalizedLegacySettings{
		ServiceBaseURL: serviceBaseURL,
		DidDomain:      legacy.DidDomain,
		RuntimeMode:    mode,
	}, nil
}

func ensureTargetStoreSchema(ctx context.Context, paths appconfig.Paths) error {
	db, err := store.Open(paths)
	if err != nil {
		return err
	}
	defer db.Close()
	return store.EnsureSchema(ctx, db)
}

func validateSQLiteHealth(ctx context.Context, db *sql.DB) error {
	if err := expectSingleSQLiteOK(ctx, db, "PRAGMA integrity_check"); err != nil {
		return err
	}
	if err := expectSQLiteNoRows(ctx, db, "PRAGMA foreign_key_check"); err != nil {
		return err
	}
	return nil
}

func expectSingleSQLiteOK(ctx context.Context, db *sql.DB, query string) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("execute %s: %w", query, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return fmt.Errorf("%s returned no rows", query)
	}
	var result string
	if err := rows.Scan(&result); err != nil {
		return fmt.Errorf("scan %s result: %w", query, err)
	}
	if !strings.EqualFold(strings.TrimSpace(result), "ok") && strings.TrimSpace(result) != "" {
		return fmt.Errorf("%s failed: %s", query, result)
	}
	return nil
}

func expectSQLiteNoRows(ctx context.Context, db *sql.DB, query string) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("execute %s: %w", query, err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("%s returned foreign key violations", query)
	}
	return nil
}
