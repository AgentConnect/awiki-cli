package cli

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/content"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

func (a *App) contentService() (*content.Service, output.Format, error) {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return nil, output.FormatJSON, err
	}
	format := normalizedFormat(resolved.OutputFormat)
	service, err := content.NewService(resolved)
	if err != nil {
		return nil, format, err
	}
	return service, format, nil
}

func (a *App) contentExit(err error, hint string) error {
	if err == nil {
		return nil
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	var serviceErr *content.ServiceError
	if errors.As(err, &serviceErr) {
		switch {
		case serviceErr.StatusCode == 400 || serviceErr.RPCCode == -32602:
			return output.NewExitError("invalid_argument", 2, err.Error(), hint)
		case serviceErr.StatusCode == 401 || serviceErr.RPCCode == -32000:
			return output.NewExitError("auth_required", 3, err.Error(), "Use an identity with a valid JWT or DID WBA auth material.")
		case serviceErr.StatusCode == 404 || serviceErr.RPCCode == -32002:
			return output.NewExitError("not_found", 5, err.Error(), hint)
		case serviceErr.StatusCode == 409 || serviceErr.RPCCode == -32003 || serviceErr.RPCCode == -32004:
			return output.NewExitError("conflict", 1, err.Error(), hint)
		}
	}
	switch {
	case errors.Is(err, content.ErrSlugRequired), errors.Is(err, content.ErrTitleRequired), errors.Is(err, content.ErrNoUpdateFields), errors.Is(err, content.ErrVisibilityInvalid):
		return output.NewExitError("invalid_argument", 2, err.Error(), hint)
	case errors.Is(err, identity.ErrIdentityNotFound), errors.Is(err, identity.ErrNoDefaultIdentity):
		return output.NewExitError("not_found", 5, err.Error(), "Run `awiki-cli id list` to inspect available identities.")
	case errors.Is(err, identity.ErrAuthRequired):
		return output.NewExitError("auth_required", 3, err.Error(), "Use an identity with a valid JWT, or run `awiki-cli id register` / `awiki-cli id recover` first.")
	default:
		return output.NewExitError("internal_error", 1, err.Error(), hint)
	}
}

func (a *App) runPageCreate(cmd *cobra.Command, args []string) error {
	slug, _ := cmd.Flags().GetString("slug")
	title, _ := cmd.Flags().GetString("title")
	markdown, _ := cmd.Flags().GetString("markdown")
	markdownFile, _ := cmd.Flags().GetString("markdown-file")
	visibility, _ := cmd.Flags().GetString("visibility")
	body, err := resolveMarkdownBody(markdown, cmd.Flags().Changed("markdown"), markdownFile, cmd.Flags().Changed("markdown-file"))
	if err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Choose one content body source and make sure the file is readable.")
	}
	service, format, err := a.contentService()
	if err != nil {
		return a.contentExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := content.CreatePageParams{Slug: slug, Title: title, Body: body, Visibility: visibility}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "page.create",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/content/rpc",
			"rpc_method":   "create",
			"request": map[string]any{
				"slug":       strings.TrimSpace(slug),
				"title":      strings.TrimSpace(title),
				"body_bytes": len(body),
				"visibility": defaultString(visibility, "public"),
			},
		}}, "Dry run: page create planned", nil, a.identityMeta())
	}
	result, err := service.CreatePage(context.Background(), request)
	if err != nil {
		return a.contentExit(err, "Make sure the active identity has a handle and the page slug is valid.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runPageList(cmd *cobra.Command, args []string) error {
	service, format, err := a.contentService()
	if err != nil {
		return a.contentExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "page.list",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/content/rpc",
			"rpc_method":   "list",
		}}, "Dry run: page list planned", nil, a.identityMeta())
	}
	result, err := service.ListPages(context.Background())
	if err != nil {
		return a.contentExit(err, "Make sure the active identity has a handle and can access content pages.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runPageGet(cmd *cobra.Command, args []string) error {
	slug, _ := cmd.Flags().GetString("slug")
	service, format, err := a.contentService()
	if err != nil {
		return a.contentExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "page.get",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/content/rpc",
			"rpc_method":   "get",
			"request":      map[string]any{"slug": strings.TrimSpace(slug)},
		}}, "Dry run: page get planned", nil, a.identityMeta())
	}
	result, err := service.GetPage(context.Background(), slug)
	if err != nil {
		return a.contentExit(err, "Make sure the page exists and the active identity can access it.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runPageUpdate(cmd *cobra.Command, args []string) error {
	slug, _ := cmd.Flags().GetString("slug")
	title, _ := cmd.Flags().GetString("title")
	markdown, _ := cmd.Flags().GetString("markdown")
	markdownFile, _ := cmd.Flags().GetString("markdown-file")
	visibilityChanged := cmd.Flags().Changed("visibility")
	visibility, _ := cmd.Flags().GetString("visibility")
	body, err := resolveOptionalMarkdownBody(markdown, cmd.Flags().Changed("markdown"), markdownFile, cmd.Flags().Changed("markdown-file"))
	if err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Choose one content body source and make sure the file is readable.")
	}
	service, format, err := a.contentService()
	if err != nil {
		return a.contentExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := content.UpdatePageParams{Slug: slug, Title: title, Body: body}
	if visibilityChanged {
		request.Visibility = &visibility
	}
	if a.globals.DryRun {
		changedFields := make([]string, 0, 3)
		if strings.TrimSpace(title) != "" {
			changedFields = append(changedFields, "title")
		}
		if body != nil {
			changedFields = append(changedFields, "body")
		}
		if visibilityChanged {
			changedFields = append(changedFields, "visibility")
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":         "page.update",
			"identity":       a.globals.Identity,
			"rpc_endpoint":   "/content/rpc",
			"rpc_method":     "update",
			"changed_fields": changedFields,
			"request": map[string]any{
				"slug":       strings.TrimSpace(slug),
				"title":      strings.TrimSpace(title),
				"body_bytes": bodySize(body),
				"visibility": visibility,
			},
		}}, "Dry run: page update planned", nil, a.identityMeta())
	}
	result, err := service.UpdatePage(context.Background(), request)
	if err != nil {
		return a.contentExit(err, "Make sure the page exists and the updated fields are valid.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runPageRename(cmd *cobra.Command, args []string) error {
	slug, _ := cmd.Flags().GetString("slug")
	target, _ := cmd.Flags().GetString("to")
	service, format, err := a.contentService()
	if err != nil {
		return a.contentExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := content.RenamePageParams{Slug: slug, To: target}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "page.rename",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/content/rpc",
			"rpc_method":   "rename",
			"request":      map[string]any{"old_slug": strings.TrimSpace(slug), "new_slug": strings.TrimSpace(target)},
		}}, "Dry run: page rename planned", nil, a.identityMeta())
	}
	result, err := service.RenamePage(context.Background(), request)
	if err != nil {
		return a.contentExit(err, "Make sure the source page exists and the target slug is available.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runPageDelete(cmd *cobra.Command, args []string) error {
	slug, _ := cmd.Flags().GetString("slug")
	service, format, err := a.contentService()
	if err != nil {
		return a.contentExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{
			"action":       "page.delete",
			"identity":     a.globals.Identity,
			"rpc_endpoint": "/content/rpc",
			"rpc_method":   "delete",
			"request":      map[string]any{"slug": strings.TrimSpace(slug)},
		}}, "Dry run: page delete planned", nil, a.identityMeta())
	}
	result, err := service.DeletePage(context.Background(), slug)
	if err != nil {
		return a.contentExit(err, "Make sure the page exists and the active identity can delete it.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func resolveMarkdownBody(markdown string, markdownChanged bool, markdownFile string, markdownFileChanged bool) (string, error) {
	body, err := resolveOptionalMarkdownBody(markdown, markdownChanged, markdownFile, markdownFileChanged)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", nil
	}
	return *body, nil
}

func resolveOptionalMarkdownBody(markdown string, markdownChanged bool, markdownFile string, markdownFileChanged bool) (*string, error) {
	if markdownChanged && markdownFileChanged {
		return nil, content.ErrBodySourceConflict
	}
	if markdownFileChanged {
		raw, err := os.ReadFile(markdownFile)
		if err != nil {
			return nil, err
		}
		text := string(raw)
		return &text, nil
	}
	if !markdownChanged {
		return nil, nil
	}
	text := markdown
	return &text, nil
}

func bodySize(body *string) int {
	if body == nil {
		return 0
	}
	return len(*body)
}
