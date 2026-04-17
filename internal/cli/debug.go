package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/spf13/cobra"
)

func (a *App) openStore() (*appconfig.Resolved, *sql.DB, output.Format, error) {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return nil, nil, output.FormatJSON, err
	}
	format := normalizedFormat(resolved.OutputFormat)
	db, err := store.Open(resolved.Paths)
	if err != nil {
		return resolved, nil, format, err
	}
	return resolved, db, format, nil
}

func (a *App) storeExit(err error, hint string) error {
	if err == nil {
		return nil
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	switch {
	case errors.Is(err, store.ErrLegacyDatabaseNotFound), errors.Is(err, sql.ErrNoRows):
		return output.NewExitError("not_found", 5, err.Error(), hint)
	case errors.Is(err, store.ErrUnsafeSQL), errors.Is(err, store.ErrUnsupportedLegacySchema):
		return output.NewExitError("invalid_argument", 2, err.Error(), hint)
	default:
		return output.NewExitError("internal_error", 1, err.Error(), hint)
	}
}

func (a *App) runDebugDBQuery(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return output.NewExitError("invalid_argument", 2, "debug db query requires exactly one SQL statement.", "Usage: awiki-cli debug db query \"SELECT * FROM messages LIMIT 5\"")
	}
	resolved, db, format, err := a.openStore()
	if err != nil {
		return a.storeExit(err, "Run `awiki-cli doctor` to inspect the database path and configuration.")
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		return a.storeExit(err, "Initialize the local store before querying it.")
	}
	rows, err := store.ExecuteSQL(context.Background(), db, args[0])
	if err != nil {
		return a.storeExit(err, "Only single-statement safe SQL is allowed. Avoid destructive statements.")
	}
	data := map[string]any{
		"database_file": resolved.Paths.DatabaseFile,
		"rows":          rows,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "SQLite query executed", nil, identityMetaFromResolved(resolved))
}

func (a *App) runDebugDBImportV1(cmd *cobra.Command, args []string) error {
	pathOverride, _ := cmd.Flags().GetString("path")
	resolved, db, format, err := a.openStore()
	if err != nil {
		return a.storeExit(err, "Run `awiki-cli doctor` to inspect the database path and configuration.")
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		return a.storeExit(err, "Initialize the local store before importing legacy data.")
	}
	paths := resolved.Paths
	if strings.TrimSpace(pathOverride) != "" {
		paths.LegacyDataDir = strings.TrimSpace(pathOverride)
	}
	manager := identity.NewManager(paths)
	if a.globals.DryRun {
		scan, err := store.ScanLegacyDatabase(context.Background(), paths)
		if err != nil {
			return a.storeExit(err, "Make sure the legacy database path is correct.")
		}
		data := map[string]any{
			"plan": map[string]any{
				"action":      "import_v1_sqlite",
				"source_scan": scan,
				"target":      resolved.Paths.DatabaseFile,
			},
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: legacy SQLite import planned", nil, identityMetaFromResolved(resolved))
	}
	report, err := store.ImportLegacyDatabase(context.Background(), db, paths, manager)
	if err != nil {
		return a.storeExit(err, "Make sure the v1 database exists and identities were imported first.")
	}
	data := map[string]any{
		"database_file": resolved.Paths.DatabaseFile,
		"import_report": report,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Legacy SQLite import completed", report.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runDebugDBHandleHistory(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return output.NewExitError(
			"invalid_argument",
			2,
			"debug db handle-history requires exactly one handle.",
			"Usage: awiki-cli debug db handle-history <handle>",
		)
	}
	resolved, db, format, err := a.openStore()
	if err != nil {
		return a.storeExit(err, "Run `awiki-cli doctor` to inspect the database path and configuration.")
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		return a.storeExit(err, "Initialize the local store before querying handle history.")
	}

	handle := normalizeDebugHandle(args[0])
	if handle == "" {
		return output.NewExitError("invalid_argument", 2, "handle is required.", "Provide a handle local-part or full handle.")
	}

	rows, err := store.ExecuteSQL(
		context.Background(),
		db,
		`SELECT owner_did, handle, did, is_current, first_seen_at, last_seen_at, source_type, source_group_id, metadata
FROM contact_handle_bindings
WHERE handle = ?
ORDER BY owner_did ASC, is_current DESC, last_seen_at DESC`,
		handle,
	)
	if err != nil {
		return a.storeExit(err, "Make sure the local store schema is current before reading handle history.")
	}
	if len(rows) == 0 {
		return a.storeExit(sql.ErrNoRows, fmt.Sprintf("No local DID history is stored for handle %q.", handle))
	}

	currentByOwner := map[string]string{}
	historicalByOwner := map[string][]string{}
	for _, row := range rows {
		ownerDID := strings.TrimSpace(stringFromAny(row["owner_did"]))
		did := strings.TrimSpace(stringFromAny(row["did"]))
		if ownerDID == "" || did == "" {
			continue
		}
		if boolFromAny(row["is_current"]) {
			currentByOwner[ownerDID] = did
		}
		historicalByOwner[ownerDID] = append(historicalByOwner[ownerDID], did)
	}

	data := map[string]any{
		"database_file": resolved.Paths.DatabaseFile,
		"handle":        handle,
		"owners":        buildHandleHistoryOwners(rows, currentByOwner, historicalByOwner),
		"rows":          rows,
	}
	return a.renderSuccess(
		cmd.CommandPath(),
		format,
		a.globals.JQ,
		data,
		fmt.Sprintf("Loaded local DID history for handle %s", handle),
		nil,
		identityMetaFromResolved(resolved),
	)
}

func normalizeDebugHandle(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	value = strings.TrimPrefix(value, "wba://")
	if index := strings.Index(value, "."); index > 0 {
		return value[:index]
	}
	return value
}

func buildHandleHistoryOwners(rows []map[string]any, currentByOwner map[string]string, historicalByOwner map[string][]string) []map[string]any {
	owners := make([]map[string]any, 0, len(historicalByOwner))
	seen := map[string]struct{}{}
	for _, row := range rows {
		ownerDID := strings.TrimSpace(stringFromAny(row["owner_did"]))
		if ownerDID == "" {
			continue
		}
		if _, ok := seen[ownerDID]; ok {
			continue
		}
		seen[ownerDID] = struct{}{}
		owners = append(owners, map[string]any{
			"owner_did":        ownerDID,
			"current_did":      currentByOwner[ownerDID],
			"historical_dids":  historicalByOwner[ownerDID],
			"historical_count": len(historicalByOwner[ownerDID]),
		})
	}
	return owners
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		return typed == "1" || strings.EqualFold(typed, "true")
	default:
		return false
	}
}
