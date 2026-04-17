package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/agentconnect/awiki-cli/internal/skillbundle"
	"github.com/spf13/cobra"
)

func (a *App) runSkillIndex(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return a.configCommandExit(err)
	}
	kind, _ := cmd.Flags().GetString("kind")
	tag, _ := cmd.Flags().GetString("tag")
	if err := skillbundle.ValidateKind(kind); err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Use an empty kind or `reference`.")
	}
	result, err := skillbundle.Index(kind, tag)
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Failed to load the embedded skill bundle.")
	}
	format := normalizedFormat(resolved.OutputFormat)
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result, fmt.Sprintf("Loaded %d embedded skill documents", len(result.Children)), nil, identityMetaFromResolved(resolved))
}

func (a *App) runSkillGet(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return a.configCommandExit(err)
	}
	if len(args) != 1 {
		return output.NewExitError("invalid_argument", 2, "skill get requires exactly one path argument.", "Run `awiki-cli skill index` to list embedded document paths.")
	}
	cacheMode, _ := cmd.Flags().GetString("cache")
	writePath, _ := cmd.Flags().GetString("write")
	cacheMode = strings.TrimSpace(strings.ToLower(cacheMode))
	if cacheMode != "" && cacheMode != "auto" && cacheMode != "refresh" && cacheMode != "bypass" {
		return output.NewExitError("invalid_argument", 2, fmt.Sprintf("unsupported cache mode %q", cacheMode), "Use --cache auto, refresh, or bypass.")
	}
	result, err := skillbundle.Get(resolved, args[0], cacheMode, writePath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return output.NewExitError("not_found", 5, err.Error(), "Run `awiki-cli skill index` to list embedded document paths.")
		}
		return output.NewExitError("internal_error", 1, err.Error(), "Failed to load the embedded skill document.")
	}
	format := normalizedFormat(resolved.OutputFormat)
	warnings := []string(nil)
	if strings.TrimSpace(writePath) != "" {
		warnings = append(warnings, "The skill document content was also returned inside the JSON envelope.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result, fmt.Sprintf("Loaded embedded skill document %s", result.Path), warnings, identityMetaFromResolved(resolved))
}

func (a *App) runSkillSync(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return a.configCommandExit(err)
	}
	dir, _ := cmd.Flags().GetString("dir")
	force, _ := cmd.Flags().GetBool("force")
	adopt, _ := cmd.Flags().GetBool("adopt")
	format := normalizedFormat(resolved.OutputFormat)
	if a.globals.DryRun {
		result, renderErr := previewSkillSync(resolved, dir)
		if renderErr != nil {
			return output.NewExitError("internal_error", 1, renderErr.Error(), "Failed to render the root skill preview.")
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result, "Dry run: root skill sync planned", nil, identityMetaFromResolved(resolved))
	}
	result, err := skillbundle.SyncRootSkill(resolved, dir, force, adopt)
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Failed to sync the public root skill.")
	}
	if result.Conflict {
		return output.NewExitError("conflict", 1, result.ConflictReason, "Use --adopt or --force after reviewing the existing skill directory.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result, "Root skill synced successfully", nil, identityMetaFromResolved(resolved))
}

func (a *App) runSkillExport(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return a.configCommandExit(err)
	}
	dir, _ := cmd.Flags().GetString("dir")
	all, _ := cmd.Flags().GetBool("all")
	path, _ := cmd.Flags().GetString("path")
	includeRoot, _ := cmd.Flags().GetBool("include-root")
	clean, _ := cmd.Flags().GetBool("clean")
	if all == (strings.TrimSpace(path) != "") {
		return output.NewExitError("invalid_argument", 2, "skill export requires either --all or --path.", "Use --all to export every reference, or --path references/<file>.md to export one document.")
	}
	format := normalizedFormat(resolved.OutputFormat)
	if a.globals.DryRun {
		data := map[string]any{
			"dir":          dir,
			"all":          all,
			"path":         path,
			"include_root": includeRoot,
			"clean":        clean,
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: skill export planned", nil, identityMetaFromResolved(resolved))
	}
	result, err := skillbundle.Export(dir, path, all, includeRoot, clean)
	if err != nil {
		if strings.Contains(err.Error(), "not empty") || strings.Contains(err.Error(), "not found") {
			return output.NewExitError("invalid_argument", 2, err.Error(), "Use --clean to reset the output directory or run `awiki-cli skill index` to inspect valid paths.")
		}
		return output.NewExitError("internal_error", 1, err.Error(), "Failed to export the offline skill package.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result, fmt.Sprintf("Exported %d embedded skill documents", len(result.ExportedDocs)), nil, identityMetaFromResolved(resolved))
}

func previewSkillSync(resolved *appconfig.Resolved, dir string) (map[string]any, error) {
	bundle, err := skillbundle.Load()
	if err != nil {
		return nil, err
	}
	root, rootSHA, err := skillbundle.RenderRootSkill(bundle)
	if err != nil {
		return nil, err
	}
	defaultDir := strings.TrimSpace(dir)
	if defaultDir == "" {
		defaultDir, err = skillbundle.DefaultSkillRootDir()
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"dir":               defaultDir,
		"skill_dir":         filepath.Join(defaultDir, bundle.Manifest.RootSkill.Name),
		"root_skill_sha256": rootSHA,
		"preview":           root,
		"state_path":        mustSkillStatePath(resolved),
	}, nil
}

func mustSkillStatePath(resolved *appconfig.Resolved) string {
	paths, err := skillbundle.ResolvePaths(resolved)
	if err != nil {
		return ""
	}
	return paths.SkillStatePath
}
