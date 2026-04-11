package cli

import (
	"context"
	"database/sql"
	"errors"
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
