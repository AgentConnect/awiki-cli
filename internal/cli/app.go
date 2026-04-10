package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	docindex "github.com/agentconnect/awiki-cli/internal/docs"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
)

type GlobalOptions struct {
	Format          string
	FormatChanged   bool
	JQ              string
	DryRun          bool
	Identity        string
	IdentityChanged bool
	Verbose         bool
}

type App struct {
	globals GlobalOptions
	catalog *cmdmeta.Catalog
	docs    *docindex.Index

	updateWarning string
}

func Execute() int {
	app := &App{
		globals: GlobalOptions{Format: string(output.FormatJSON)},
		catalog: cmdmeta.NewCatalog(),
		docs:    docindex.NewIndex(),
	}
	rootCmd := newRootCommand(app)
	if err := rootCmd.Execute(); err != nil {
		return app.handleError(err)
	}
	return 0
}

func (a *App) handleError(err error) int {
	format := output.FormatJSON
	if resolved, resolveErr := output.NormalizeFormat(a.globals.Format); resolveErr == nil {
		format = resolved
	}
	detail := output.ErrorDetail{
		Code:      "internal_error",
		Message:   err.Error(),
		Retryable: false,
	}
	exitCode := 1
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		detail = exitErr.Detail
		exitCode = exitErr.Code
	}
	detail.Details = identity.PublicValue(detail.Details)
	envelope := output.ErrorEnvelope{
		OK:    false,
		Error: detail,
		Meta: output.Meta{
			Version: buildinfo.Version,
			DryRun:  a.globals.DryRun,
			Format:  string(format),
		},
	}
	if identity := a.identityMeta(); identity != nil {
		envelope.Meta.Identity = identity
	}
	if renderErr := output.RenderError(os.Stderr, format, a.globals.JQ, envelope); renderErr != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	return exitCode
}

func (a *App) renderSuccess(command string, format output.Format, jqExpr string, data any, summary string, warnings []string, identityMeta *output.IdentityMeta) error {
	mergedWarnings := warnings
	if a.updateWarning != "" {
		mergedWarnings = append([]string{a.updateWarning}, warnings...)
	}
	envelope := output.SuccessEnvelope{
		OK:       true,
		Command:  command,
		Data:     identity.PublicValue(data),
		Warnings: mergedWarnings,
		Summary:  summary,
		Meta: output.Meta{
			Version:  buildinfo.Version,
			Identity: identityMeta,
			DryRun:   a.globals.DryRun,
			Format:   string(format),
		},
	}
	return output.RenderSuccess(os.Stdout, format, jqExpr, envelope)
}

func (a *App) resolveConfig() (*appconfig.Resolved, error) {
	resolved, err := appconfig.Resolve(appconfig.Overrides{
		Identity:        a.globals.Identity,
		IdentityChanged: a.globals.IdentityChanged,
		Format:          a.globals.Format,
		FormatChanged:   a.globals.FormatChanged,
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolved.ActiveIdentity) == "" {
		manager := identity.NewManager(resolved.Paths)
		current, currentErr := manager.Current()
		if currentErr == nil && current != nil {
			resolved.ActiveIdentity = current.IdentityName
			if resolved.Sources == nil {
				resolved.Sources = map[string]appconfig.ValueSource{}
			}
			resolved.Sources["active_identity"] = appconfig.ValueSource{
				Source: "identity_index",
				Value:  current.IdentityName,
			}
		}
	}
	return resolved, nil
}

func (a *App) identityMeta() *output.IdentityMeta {
	if a.globals.Identity == "" {
		return nil
	}
	return &output.IdentityMeta{Name: a.globals.Identity}
}
