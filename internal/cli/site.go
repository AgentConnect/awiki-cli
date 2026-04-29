package cli

import (
	"errors"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
	awikisite "github.com/agentconnect/awiki-cli/internal/site"
	"github.com/spf13/cobra"
)

func (a *App) siteService() (*awikisite.Service, output.Format, error) {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return nil, output.FormatJSON, err
	}
	format := normalizedFormat(resolved.OutputFormat)
	service, err := awikisite.NewService(resolved)
	if err != nil {
		return nil, format, err
	}
	return service, format, nil
}

func (a *App) siteExit(err error, hint string) error {
	if err == nil {
		return nil
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	var serviceErr *awikisite.ServiceError
	if errors.As(err, &serviceErr) {
		switch {
		case serviceErr.StatusCode == 400 || serviceErr.RPCCode == -32602:
			return output.NewExitError("invalid_argument", 2, err.Error(), hint)
		case serviceErr.StatusCode == 401 || serviceErr.RPCCode == -32000:
			return output.NewExitError("auth_required", 3, err.Error(), "Use an identity with a valid JWT or DID WBA auth material.")
		case serviceErr.StatusCode == 403 || serviceErr.RPCCode == -32001:
			return output.NewExitError("forbidden", 4, err.Error(), hint)
		case serviceErr.StatusCode == 404 || serviceErr.RPCCode == -32002:
			return output.NewExitError("not_found", 5, err.Error(), hint)
		case serviceErr.StatusCode == 409 || serviceErr.RPCCode == -32003:
			return output.NewExitError("conflict", 1, err.Error(), hint)
		case serviceErr.RPCCode == -32004:
			return output.NewExitError("invalid_argument", 2, err.Error(), hint)
		}
	}
	switch {
	case errors.Is(err, awikisite.ErrDomainRequired), errors.Is(err, awikisite.ErrSlugRequired), errors.Is(err, awikisite.ErrNoBodySourceProvided), errors.Is(err, awikisite.ErrBodySourceConflict):
		return output.NewExitError("invalid_argument", 2, err.Error(), hint)
	case errors.Is(err, identity.ErrIdentityNotFound), errors.Is(err, identity.ErrNoDefaultIdentity):
		return output.NewExitError("not_found", 5, err.Error(), "Run `awiki-cli id list` to inspect available identities.")
	case errors.Is(err, identity.ErrAuthRequired):
		return output.NewExitError("auth_required", 3, err.Error(), "Use an identity with a valid JWT, or run `awiki-cli id register` / `awiki-cli id recover` first.")
	default:
		return output.NewExitError("internal_error", 1, err.Error(), hint)
	}
}

func (a *App) renderSiteResult(cmd *cobra.Command, format output.Format, result *awikisite.CommandResult) error {
	if result == nil {
		return commandResultMissing(cmd.CommandPath())
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runSiteRootGet(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	service, format, err := a.siteService()
	if err != nil {
		return a.siteExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "site.root.get",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/site/rpc",
			"rpc_method":   "get_root",
			"request":      map[string]any{"domain": strings.TrimSpace(domain)},
		}}, "Dry run: site root get planned", nil, a.identityMeta())
	}
	result, err := service.GetRoot(cmd.Context(), domain)
	if err != nil {
		return a.siteExit(err, "Make sure the active identity is a configured tenant site admin for the requested domain.")
	}
	return a.renderSiteResult(cmd, format, result)
}

func (a *App) runSiteRootSet(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	markdown, _ := cmd.Flags().GetString("markdown")
	markdownFile, _ := cmd.Flags().GetString("markdown-file")
	if !cmd.Flags().Changed("markdown") && !cmd.Flags().Changed("markdown-file") {
		return output.NewExitError("invalid_argument", 2, awikisite.ErrNoBodySourceProvided.Error(), "Provide --markdown or --markdown-file.")
	}
	body, err := resolveMarkdownBody(markdown, cmd.Flags().Changed("markdown"), markdownFile, cmd.Flags().Changed("markdown-file"))
	if err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Choose one content body source and make sure the file is readable.")
	}
	service, format, err := a.siteService()
	if err != nil {
		return a.siteExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := awikisite.SetRootParams{Domain: domain, Body: body}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "site.root.set",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/site/rpc",
			"rpc_method":   "set_root",
			"request":      map[string]any{"domain": strings.TrimSpace(domain), "body_bytes": len(body)},
		}}, "Dry run: site root set planned", nil, a.identityMeta())
	}
	result, err := service.SetRoot(cmd.Context(), request)
	if err != nil {
		return a.siteExit(err, "Make sure the active identity is a configured tenant site admin for the requested domain.")
	}
	return a.renderSiteResult(cmd, format, result)
}

func (a *App) runSitePageList(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	service, format, err := a.siteService()
	if err != nil {
		return a.siteExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "site.page.list",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/site/rpc",
			"rpc_method":   "list_pages",
			"request":      map[string]any{"domain": strings.TrimSpace(domain)},
		}}, "Dry run: site page list planned", nil, a.identityMeta())
	}
	result, err := service.ListPages(cmd.Context(), domain)
	if err != nil {
		return a.siteExit(err, "Make sure the active identity is a configured tenant site admin for the requested domain.")
	}
	return a.renderSiteResult(cmd, format, result)
}

func (a *App) runSitePageGet(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	slug, _ := cmd.Flags().GetString("slug")
	service, format, err := a.siteService()
	if err != nil {
		return a.siteExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "site.page.get",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/site/rpc",
			"rpc_method":   "get_page",
			"request":      map[string]any{"domain": strings.TrimSpace(domain), "slug": strings.TrimSpace(slug)},
		}}, "Dry run: site page get planned", nil, a.identityMeta())
	}
	result, err := service.GetPage(cmd.Context(), domain, slug)
	if err != nil {
		return a.siteExit(err, "Make sure the page exists and the active identity can access it.")
	}
	return a.renderSiteResult(cmd, format, result)
}

func (a *App) runSitePageCreate(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	slug, _ := cmd.Flags().GetString("slug")
	markdown, _ := cmd.Flags().GetString("markdown")
	markdownFile, _ := cmd.Flags().GetString("markdown-file")
	if !cmd.Flags().Changed("markdown") && !cmd.Flags().Changed("markdown-file") {
		return output.NewExitError("invalid_argument", 2, awikisite.ErrNoBodySourceProvided.Error(), "Provide --markdown or --markdown-file.")
	}
	body, err := resolveMarkdownBody(markdown, cmd.Flags().Changed("markdown"), markdownFile, cmd.Flags().Changed("markdown-file"))
	if err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Choose one content body source and make sure the file is readable.")
	}
	service, format, err := a.siteService()
	if err != nil {
		return a.siteExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := awikisite.CreatePageParams{Domain: domain, Slug: slug, Body: body}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "site.page.create",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/site/rpc",
			"rpc_method":   "create_page",
			"request":      map[string]any{"domain": strings.TrimSpace(domain), "slug": strings.TrimSpace(slug), "body_bytes": len(body)},
		}}, "Dry run: site page create planned", nil, a.identityMeta())
	}
	result, err := service.CreatePage(cmd.Context(), request)
	if err != nil {
		return a.siteExit(err, "Make sure the active identity is a configured tenant site admin for the requested domain and the slug is available.")
	}
	return a.renderSiteResult(cmd, format, result)
}

func (a *App) runSitePageUpdate(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	slug, _ := cmd.Flags().GetString("slug")
	markdown, _ := cmd.Flags().GetString("markdown")
	markdownFile, _ := cmd.Flags().GetString("markdown-file")
	if !cmd.Flags().Changed("markdown") && !cmd.Flags().Changed("markdown-file") {
		return output.NewExitError("invalid_argument", 2, awikisite.ErrNoBodySourceProvided.Error(), "Provide --markdown or --markdown-file.")
	}
	body, err := resolveMarkdownBody(markdown, cmd.Flags().Changed("markdown"), markdownFile, cmd.Flags().Changed("markdown-file"))
	if err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Choose one content body source and make sure the file is readable.")
	}
	service, format, err := a.siteService()
	if err != nil {
		return a.siteExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := awikisite.UpdatePageParams{Domain: domain, Slug: slug, Body: body}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "site.page.update",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/site/rpc",
			"rpc_method":   "update_page",
			"request":      map[string]any{"domain": strings.TrimSpace(domain), "slug": strings.TrimSpace(slug), "body_bytes": len(body)},
		}}, "Dry run: site page update planned", nil, a.identityMeta())
	}
	result, err := service.UpdatePage(cmd.Context(), request)
	if err != nil {
		return a.siteExit(err, "Make sure the page exists and the active identity can update it.")
	}
	return a.renderSiteResult(cmd, format, result)
}

func (a *App) runSitePageRename(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	slug, _ := cmd.Flags().GetString("slug")
	target, _ := cmd.Flags().GetString("to")
	service, format, err := a.siteService()
	if err != nil {
		return a.siteExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := awikisite.RenamePageParams{Domain: domain, Slug: slug, To: target}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "site.page.rename",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/site/rpc",
			"rpc_method":   "rename_page",
			"request":      map[string]any{"domain": strings.TrimSpace(domain), "old_slug": strings.TrimSpace(slug), "new_slug": strings.TrimSpace(target)},
		}}, "Dry run: site page rename planned", nil, a.identityMeta())
	}
	result, err := service.RenamePage(cmd.Context(), request)
	if err != nil {
		return a.siteExit(err, "Make sure the source page exists and the target slug is available.")
	}
	return a.renderSiteResult(cmd, format, result)
}

func (a *App) runSitePageDelete(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	slug, _ := cmd.Flags().GetString("slug")
	service, format, err := a.siteService()
	if err != nil {
		return a.siteExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "site.page.delete",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/site/rpc",
			"rpc_method":   "delete_page",
			"request":      map[string]any{"domain": strings.TrimSpace(domain), "slug": strings.TrimSpace(slug)},
		}}, "Dry run: site page delete planned", nil, a.identityMeta())
	}
	result, err := service.DeletePage(cmd.Context(), domain, slug)
	if err != nil {
		return a.siteExit(err, "Make sure the page exists and the active identity can delete it.")
	}
	return a.renderSiteResult(cmd, format, result)
}
