package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	doccheck "github.com/agentconnect/awiki-cli/internal/doctor"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/upgrade"
	"github.com/spf13/cobra"
)

const rootLong = `awiki-cli — Agent-native identity and messaging CLI.

Phase 1 currently provides the pure-Go CLI shell, global output contract,
static schema introspection, built-in docs, and baseline environment checks.

Use "awiki-cli schema" to inspect the frozen command contract and
"awiki-cli doctor" to inspect paths, env compatibility, and migration hints.`

func newRootCommand(app *App) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "awiki-cli",
		Short:         "awiki CLI phase-1 shell",
		Long:          rootLong,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			app.globals.FormatChanged = cmd.Flags().Changed("format")
			app.globals.IdentityChanged = cmd.Flags().Changed("identity")
			_, err := output.NormalizeFormat(app.globals.Format)
			if err != nil {
				return output.NewExitError("invalid_argument", 2, err.Error(), "Use --format json, pretty, ndjson, or table.")
			}
			return nil
		},
	}
	rootCmd.PersistentFlags().StringVar(&app.globals.Format, "format", string(output.FormatJSON), "Output format: json | pretty | ndjson | table")
	rootCmd.PersistentFlags().StringVar(&app.globals.JQ, "jq", "", "Apply a jq expression to the JSON envelope")
	rootCmd.PersistentFlags().BoolVar(&app.globals.DryRun, "dry-run", false, "Render the execution plan without mutating state")
	rootCmd.PersistentFlags().StringVar(&app.globals.Identity, "identity", "", "Select the active identity")
	rootCmd.PersistentFlags().BoolVar(&app.globals.Verbose, "verbose", false, "Enable verbose output")

	commandsByName := map[string]*cobra.Command{"": rootCmd}
	specs := app.catalog.Specs()
	sort.Slice(specs, func(i, j int) bool {
		leftDepth := strings.Count(specs[i].Name, ".")
		rightDepth := strings.Count(specs[j].Name, ".")
		if leftDepth == rightDepth {
			return specs[i].Name < specs[j].Name
		}
		return leftDepth < rightDepth
	})

	for _, spec := range specs {
		command := app.commandFromSpec(spec)
		parent := commandsByName[parentName(spec.Name)]
		if parent == nil {
			panic(fmt.Sprintf("missing parent command for %s", spec.Name))
		}
		parent.AddCommand(command)
		commandsByName[strings.ToLower(spec.Name)] = command
	}
	if runtimeListener := commandsByName["runtime.listener"]; runtimeListener != nil {
		runtimeListener.AddCommand(&cobra.Command{
			Use:    "run",
			Short:  "Run the websocket listener in the foreground",
			Hidden: true,
			RunE:   app.runRuntimeListenerRun,
		})
	}
	return rootCmd
}

func parentName(name string) string {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	index := strings.LastIndex(trimmed, ".")
	if index < 0 {
		return ""
	}
	return trimmed[:index]
}

func (a *App) commandFromSpec(spec cmdmeta.CommandSpec) *cobra.Command {
	command := &cobra.Command{
		Use:     spec.Use,
		Short:   spec.Short,
		Long:    spec.Long,
		Aliases: spec.Aliases,
		Hidden:  spec.Hidden,
	}
	for _, flag := range spec.Flags {
		switch flag.Type {
		case "string":
			command.Flags().String(flag.Name, flag.Default, flag.Usage)
		case "bool":
			defaultValue := strings.EqualFold(flag.Default, "true")
			command.Flags().Bool(flag.Name, defaultValue, flag.Usage)
		case "int":
			defaultValue := 0
			if strings.TrimSpace(flag.Default) != "" {
				if parsed, err := strconv.Atoi(flag.Default); err == nil {
					defaultValue = parsed
				}
			}
			command.Flags().Int(flag.Name, defaultValue, flag.Usage)
		default:
			command.Flags().String(flag.Name, flag.Default, flag.Usage)
		}
		if flag.Required {
			_ = command.MarkFlagRequired(flag.Name)
		}
	}
	if handler := a.handlerFor(spec); handler != nil {
		command.RunE = handler
	}
	return command
}

func (a *App) handlerFor(spec cmdmeta.CommandSpec) func(*cobra.Command, []string) error {
	switch spec.Handler {
	case "init":
		return a.runInit
	case "status":
		return a.runStatus
	case "docs":
		return a.runDocs
	case "schema":
		return a.runSchema
	case "doctor":
		return a.runDoctor
	case "version":
		return a.runVersion
	case "config.show":
		return a.runConfigShow
	case "id.status":
		return a.runIDStatus
	case "id.create":
		return a.runIDCreate
	case "id.register":
		return a.runIDRegister
	case "id.bind":
		return a.runIDBind
	case "id.resolve":
		return a.runIDResolve
	case "id.recover":
		return a.runIDRecover
	case "id.list":
		return a.runIDList
	case "id.current":
		return a.runIDCurrent
	case "id.use":
		return a.runIDUse
	case "id.profile.get":
		return a.runIDProfileGet
	case "id.profile.set":
		return a.runIDProfileSet
	case "id.import-v1":
		return a.runIDImportV1
	case "msg.send":
		return a.runMsgSend
	case "msg.attachment.download":
		return a.runMsgAttachmentDownload
	case "msg.inbox":
		return a.runMsgInbox
	case "msg.history":
		return a.runMsgHistory
	case "msg.mark-read":
		return a.runMsgMarkRead
	case "group.create":
		return a.runGroupCreate
	case "group.show":
		return a.runGroupShow
	case "group.get":
		return a.runGroupShow
	case "group.join":
		return a.runGroupJoin
	case "group.add":
		return a.runGroupAdd
	case "group.kick":
		return a.runGroupKick
	case "group.remove":
		return a.runGroupKick
	case "group.leave":
		return a.runGroupLeave
	case "group.update":
		return a.runGroupUpdate
	case "group.members":
		return a.runGroupMembers
	case "group.messages":
		return a.runGroupMessages
	case "page.create":
		return a.runPageCreate
	case "page.list":
		return a.runPageList
	case "page.get":
		return a.runPageGet
	case "page.update":
		return a.runPageUpdate
	case "page.rename":
		return a.runPageRename
	case "page.delete":
		return a.runPageDelete
	case "runtime.status":
		return a.runRuntimeStatus
	case "runtime.setup":
		return a.runRuntimeSetup
	case "runtime.mode.get":
		return a.runRuntimeModeGet
	case "runtime.mode.set":
		return a.runRuntimeModeSet
	case "runtime.listener.status":
		return a.runRuntimeListenerStatus
	case "runtime.listener.install":
		return a.runRuntimeListenerInstall
	case "runtime.listener.start":
		return a.runRuntimeListenerStart
	case "runtime.listener.stop":
		return a.runRuntimeListenerStop
	case "runtime.listener.restart":
		return a.runRuntimeListenerRestart
	case "runtime.listener.uninstall":
		return a.runRuntimeListenerUninstall
	case "debug.db.query":
		return a.runDebugDBQuery
	case "debug.db.import-v1":
		return a.runDebugDBImportV1
	case "completion.bash":
		return func(cmd *cobra.Command, args []string) error { return cmd.Root().GenBashCompletion(cmd.OutOrStdout()) }
	case "completion.zsh":
		return func(cmd *cobra.Command, args []string) error { return cmd.Root().GenZshCompletion(cmd.OutOrStdout()) }
	case "completion.fish":
		return func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
		}
	case "completion.powershell":
		return func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		}
	case "stub":
		return a.runStub
	default:
		return nil
	}
}

func (a *App) runStatus(cmd *cobra.Command, args []string) error {
	service, format, err := a.identityService()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check your local configuration and environment variables.")
	}
	resolved := service.Config()
	result, err := service.Status()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the local identity store.")
	}
	data := map[string]any{
		"cli": map[string]any{
			"phase":   "phase1-shell",
			"version": buildinfo.Current(),
		},
		"paths": resolved.Paths,
		"state": result.Data,
		"config": map[string]any{
			"config_exists": resolved.ConfigExists,
			"config_error":  resolved.ConfigError,
			"env_hits":      resolved.EnvHits,
			"sources":       resolved.Sources,
		},
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, result.Summary, result.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runDocs(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check your local configuration and environment variables.")
	}
	format := normalizedFormat(resolved.OutputFormat)
	if len(args) == 0 {
		data := map[string]any{"topics": a.docs.All()}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Available documentation topics", nil, identityMetaFromResolved(resolved))
	}
	if len(args) > 1 {
		return output.NewExitError("invalid_argument", 2, "docs accepts at most one topic.", "Run `awiki-cli docs` without arguments to list topics.")
	}
	topic, ok := a.docs.Lookup(args[0])
	if !ok {
		return output.NewExitError("not_found", 5, fmt.Sprintf("Unknown docs topic %q", args[0]), "Run `awiki-cli docs` to list available topics.")
	}
	data := map[string]any{"topic": topic}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, fmt.Sprintf("Documentation topic %s", topic.Name), nil, identityMetaFromResolved(resolved))
}

func (a *App) runSchema(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check your local configuration and environment variables.")
	}
	format := normalizedFormat(resolved.OutputFormat)
	if len(args) == 0 {
		data := map[string]any{
			"commands": a.catalog.Specs(),
			"phase":    "phase1-shell",
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Static command contract", nil, identityMetaFromResolved(resolved))
	}
	lookupTarget := strings.Join(args, " ")
	spec, ok := a.catalog.Lookup(lookupTarget)
	if !ok {
		return output.NewExitError("not_found", 5, fmt.Sprintf("Unknown command schema target %q", lookupTarget), "Use `awiki-cli schema` to list command contracts.")
	}
	data := map[string]any{
		"command":  spec,
		"children": a.catalog.ChildrenOf(spec.Name),
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, fmt.Sprintf("Static contract for %s", spec.Name), nil, identityMetaFromResolved(resolved))
}

func (a *App) runDoctor(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check your local configuration and environment variables.")
	}
	format := normalizedFormat(resolved.OutputFormat)
	report := doccheck.Run(resolved)
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, report, report.Summary, nil, identityMetaFromResolved(resolved))
}

func (a *App) runVersion(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check your local configuration and environment variables.")
	}
	format := normalizedFormat(resolved.OutputFormat)
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, buildinfo.Current(), "Build information", nil, identityMetaFromResolved(resolved))
}

func (a *App) runConfigShow(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check your local configuration and environment variables.")
	}
	format := normalizedFormat(resolved.OutputFormat)
	manager := identity.NewManager(resolved.Paths)
	current, _ := manager.Current()
	legacy, _ := manager.ScanLegacy()
	upgradeState, _ := upgrade.Inspect(context.Background(), resolved, buildinfo.Version)
	data := appconfig.Snapshot(resolved)
	database := map[string]any{
		"database_file": resolved.Paths.DatabaseFile,
		"exists":        fileExists(resolved.Paths.DatabaseFile),
	}
	if database["exists"] == true {
		if db, err := store.OpenReadOnly(resolved.Paths.DatabaseFile); err == nil {
			defer db.Close()
			if version, err := store.CurrentSchemaVersion(db); err == nil {
				database["schema_version"] = version
				database["target_schema_version"] = store.SchemaVersion
			} else {
				database["schema_error"] = err.Error()
			}
		} else {
			database["open_error"] = err.Error()
		}
	}
	data["identity_store"] = map[string]any{
		"identity_dir":     resolved.Paths.IdentityDir,
		"index_file":       filepathJoin(resolved.Paths.IdentityDir, "index.json"),
		"default_identity": current,
		"legacy_scan":      legacy,
	}
	data["database"] = database
	data["workspace_upgrade"] = upgradeState
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Resolved configuration", nil, identityMetaFromResolved(resolved))
}

func (a *App) runStub(cmd *cobra.Command, args []string) error {
	spec, ok := a.catalog.Lookup(strings.TrimPrefix(cmd.CommandPath(), "awiki-cli "))
	if !ok {
		return output.NewExitError("internal_error", 1, "Command metadata is missing.", "Run `awiki-cli schema` to inspect the current catalog.")
	}
	hint := fmt.Sprintf("%s is planned for %s. Use `awiki-cli schema %s` to inspect the frozen contract.", cmd.CommandPath(), strings.ToUpper(spec.Phase), spec.Name)
	return output.NewExitError("internal_error", 1, fmt.Sprintf("%s is not implemented yet.", cmd.CommandPath()), hint)
}

func identityMetaFromResolved(resolved *appconfig.Resolved) *output.IdentityMeta {
	if resolved == nil || strings.TrimSpace(resolved.ActiveIdentity) == "" {
		return nil
	}
	return &output.IdentityMeta{Name: resolved.ActiveIdentity}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func filepathJoin(parts ...string) string {
	return filepath.Join(parts...)
}

func normalizedFormat(raw string) output.Format {
	format, err := output.NormalizeFormat(raw)
	if err != nil {
		return output.FormatJSON
	}
	return format
}
