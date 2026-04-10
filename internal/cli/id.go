package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

func (a *App) identityService() (*identity.Service, output.Format, error) {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return nil, output.FormatJSON, err
	}
	format := normalizedFormat(resolved.OutputFormat)
	service, err := identity.NewService(resolved)
	if err != nil {
		return nil, format, err
	}
	return service, format, nil
}

func (a *App) renderIdentityResult(cmd *cobra.Command, format output.Format, result *identity.CommandResult) error {
	if result == nil {
		return nil
	}
	meta := a.identityMeta()
	if meta == nil {
		meta = identityMetaFromData(result.Data)
	}
	return a.renderSuccess(
		cmd.CommandPath(),
		format,
		a.globals.JQ,
		identity.PublicData(result.Data),
		result.Summary,
		result.Warnings,
		meta,
	)
}

func (a *App) identityExit(err error, fallbackHint string) error {
	if err == nil {
		return nil
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	var serviceErr *identity.ServiceError
	if errors.As(err, &serviceErr) {
		switch {
		case serviceErr.StatusCode == 400:
			return output.NewExitError("invalid_argument", 2, serviceErr.Error(), fallbackHint)
		case serviceErr.StatusCode == 401:
			return output.NewExitError("auth_required", 3, serviceErr.Error(), "Use an identity with a valid JWT, or run `awiki-cli id register` / `awiki-cli id recover` first.")
		case serviceErr.StatusCode == 404:
			return output.NewExitError("not_found", 5, serviceErr.Error(), fallbackHint)
		case serviceErr.StatusCode == 409:
			return output.NewExitError("conflict", 1, serviceErr.Error(), fallbackHint)
		case serviceErr.RPCCode != 0:
			code := "internal_error"
			exitCode := 1
			switch serviceErr.RPCCode {
			case -32602:
				code, exitCode = "invalid_argument", 2
			case -32000:
				code, exitCode = "auth_required", 3
			case -32002:
				code, exitCode = "not_found", 5
			case -32003, -32004:
				code = "conflict"
			}
			return output.NewExitError(code, exitCode, serviceErr.Error(), fallbackHint)
		}
	}
	switch {
	case errors.Is(err, identity.ErrInvalidInput):
		return output.NewExitError("invalid_argument", 2, err.Error(), fallbackHint)
	case errors.Is(err, identity.ErrIdentityNotFound), errors.Is(err, identity.ErrLegacyNotFound), errors.Is(err, identity.ErrNoDefaultIdentity):
		return output.NewExitError("not_found", 5, err.Error(), fallbackHint)
	case errors.Is(err, identity.ErrIdentityConflict):
		return output.NewExitError("conflict", 1, err.Error(), fallbackHint)
	case errors.Is(err, identity.ErrAuthRequired):
		return output.NewExitError("auth_required", 3, err.Error(), "Use an identity with a valid JWT, or run `awiki-cli id register` / `awiki-cli id recover` first.")
	default:
		return output.NewExitError("internal_error", 1, err.Error(), fallbackHint)
	}
}

func (a *App) runIDStatus(cmd *cobra.Command, args []string) error {
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the local identity store.")
	}
	result, err := service.Status()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the local identity store.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDList(cmd *cobra.Command, args []string) error {
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the local identity store.")
	}
	result, err := service.List()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the local identity store.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDCurrent(cmd *cobra.Command, args []string) error {
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli id list` to inspect available identities.")
	}
	result, err := service.Current()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli id list` to inspect available identities.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDUse(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return output.NewExitError("invalid_argument", 2, "id use requires exactly one identity name.", "Usage: awiki-cli id use <identity>")
	}
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli id list` to inspect available identities.")
	}
	if a.globals.DryRun {
		result := &identity.CommandResult{
			Data: map[string]any{
				"plan": map[string]any{
					"action":          "set_default_identity",
					"identity_name":   args[0],
					"writes":          []string{"index.json"},
					"side_effect":     true,
					"previous_source": "identity_index",
				},
			},
			Summary: "Dry run: default identity switch planned",
		}
		return a.renderIdentityResult(cmd, format, result)
	}
	result, err := service.Use(args[0])
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli id list` to inspect available identities.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	identityName, _ := cmd.Flags().GetString("identity")
	if strings.TrimSpace(name) == "" {
		return output.NewExitError("invalid_argument", 2, "id create requires --name.", "Usage: awiki-cli id create --name \"Alice\" [--identity alice]")
	}
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect configuration and storage paths.")
	}
	if a.globals.DryRun {
		existing, _ := service.Manager().List()
		alias := identity.PreviewDefaultIdentityName(identityName, existing, name)
		result := &identity.CommandResult{
			Data: map[string]any{
				"plan": map[string]any{
					"action":        "create_identity",
					"identity_name": alias,
					"display_name":  name,
					"writes": []string{
						"index.json",
						"identity.json",
						"auth.json",
						"did_document.json",
						"key-1-private.pem",
						"key-1-public.pem",
						"e2ee-signing-private.pem",
						"e2ee-agreement-private.pem",
					},
				},
			},
			Summary: "Dry run: local DID identity creation planned",
		}
		return a.renderIdentityResult(cmd, format, result)
	}
	result, err := service.Create(name, identityName)
	if err != nil {
		return a.identityExit(err, "Use a different --identity value if the alias is already occupied.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDRegister(cmd *cobra.Command, args []string) error {
	handle, _ := cmd.Flags().GetString("handle")
	phone, _ := cmd.Flags().GetString("phone")
	email, _ := cmd.Flags().GetString("email")
	otp, _ := cmd.Flags().GetString("otp")
	inviteCode, _ := cmd.Flags().GetString("invite-code")
	wait, _ := cmd.Flags().GetBool("wait")
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect configuration and storage paths.")
	}
	params := identity.RegisterParams{
		IdentityName:        a.globals.Identity,
		Handle:              handle,
		Phone:               phone,
		Email:               email,
		OTP:                 otp,
		InviteCode:          inviteCode,
		Wait:                wait,
		VerificationTimeout: identity.DefaultEmailVerificationSecs,
		PollIntervalSeconds: identity.DefaultEmailPollIntervalSecs,
	}
	if a.globals.DryRun {
		existing, _ := service.Manager().List()
		alias := identity.PreviewNamedIdentity(a.globals.Identity, existing, handle)
		action := "register_handle"
		remoteCalls := []string{"did-auth.register"}
		if phone != "" && strings.TrimSpace(otp) == "" {
			action = "send_handle_otp"
			remoteCalls = []string{"handle.send_otp"}
		}
		if email != "" && !wait {
			action = "send_registration_email"
			remoteCalls = []string{"POST /user-service/auth/email-send"}
		}
		if email != "" && wait {
			remoteCalls = []string{"GET /user-service/auth/email-status", "POST /user-service/auth/email-send", "did-auth.register"}
		}
		result := &identity.CommandResult{
			Data: map[string]any{
				"plan": map[string]any{
					"action":        action,
					"identity_name": alias,
					"handle":        handle,
					"phone":         phone,
					"email":         email,
					"remote_calls":  remoteCalls,
				},
			},
			Summary: "Dry run: handle registration flow planned",
		}
		return a.renderIdentityResult(cmd, format, result)
	}
	result, err := service.Register(context.Background(), params)
	if err != nil {
		return a.identityExit(err, "Ensure the handle, verification method, and local alias are valid.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDBind(cmd *cobra.Command, args []string) error {
	phone, _ := cmd.Flags().GetString("phone")
	email, _ := cmd.Flags().GetString("email")
	otp, _ := cmd.Flags().GetString("otp")
	wait, _ := cmd.Flags().GetBool("wait")
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli id current` to confirm the active identity.")
	}
	params := identity.BindParams{
		Phone:               phone,
		Email:               email,
		OTP:                 otp,
		Wait:                wait,
		VerificationTimeout: identity.DefaultEmailVerificationSecs,
		PollIntervalSeconds: identity.DefaultEmailPollIntervalSecs,
	}
	if a.globals.DryRun {
		action := "bind_contact"
		remoteCalls := []string{}
		if phone != "" && strings.TrimSpace(otp) == "" {
			action = "send_bind_phone_otp"
			remoteCalls = []string{"POST /user-service/auth/phone-bind-send"}
		} else if phone != "" {
			action = "bind_phone"
			remoteCalls = []string{"POST /user-service/auth/phone-bind-verify"}
		} else if email != "" && !wait {
			action = "send_bind_email"
			remoteCalls = []string{"POST /user-service/auth/email-send"}
		} else if email != "" {
			action = "bind_email"
			remoteCalls = []string{"GET /user-service/auth/email-status", "POST /user-service/auth/email-send"}
		}
		result := &identity.CommandResult{
			Data: map[string]any{
				"plan": map[string]any{
					"action":       action,
					"phone":        phone,
					"email":        email,
					"remote_calls": remoteCalls,
				},
			},
			Summary: "Dry run: contact binding flow planned",
		}
		return a.renderIdentityResult(cmd, format, result)
	}
	result, err := service.Bind(context.Background(), params)
	if err != nil {
		return a.identityExit(err, "Use an identity that already has a valid JWT.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDResolve(cmd *cobra.Command, args []string) error {
	handle, _ := cmd.Flags().GetString("handle")
	did, _ := cmd.Flags().GetString("did")
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the configured service endpoints.")
	}
	result, err := service.Resolve(context.Background(), handle, did)
	if err != nil {
		return a.identityExit(err, "Provide either --handle or --did, and make sure the target exists.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDRecover(cmd *cobra.Command, args []string) error {
	handle, _ := cmd.Flags().GetString("handle")
	phone, _ := cmd.Flags().GetString("phone")
	otp, _ := cmd.Flags().GetString("otp")
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the configured service endpoints.")
	}
	params := identity.RecoverParams{
		IdentityName: a.globals.Identity,
		Handle:       handle,
		Phone:        phone,
		OTP:          otp,
	}
	if a.globals.DryRun {
		existing, _ := service.Manager().List()
		alias := identity.PreviewNamedIdentity(a.globals.Identity, existing, handle)
		result := &identity.CommandResult{
			Data: map[string]any{
				"plan": map[string]any{
					"action":        "recover_handle",
					"identity_name": alias,
					"handle":        handle,
					"phone":         phone,
					"remote_calls":  []string{"did-auth.recover_handle"},
				},
			},
			Summary: "Dry run: handle recovery planned",
		}
		return a.renderIdentityResult(cmd, format, result)
	}
	result, err := service.Recover(context.Background(), params)
	if err != nil {
		return a.identityExit(err, "Make sure the handle exists and the recovery OTP is valid.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDProfileGet(cmd *cobra.Command, args []string) error {
	self, _ := cmd.Flags().GetBool("self")
	handle, _ := cmd.Flags().GetString("handle")
	did, _ := cmd.Flags().GetString("did")
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the configured service endpoints.")
	}
	result, err := service.GetProfile(context.Background(), self, handle, did)
	if err != nil {
		return a.identityExit(err, "Use `--self`, `--handle`, or `--did` to select one profile target.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDProfileSet(cmd *cobra.Command, args []string) error {
	displayName, _ := cmd.Flags().GetString("display-name")
	bio, _ := cmd.Flags().GetString("bio")
	tags, _ := cmd.Flags().GetString("tags")
	markdown, _ := cmd.Flags().GetString("markdown")
	markdownFile, _ := cmd.Flags().GetString("markdown-file")
	if strings.TrimSpace(markdown) != "" && strings.TrimSpace(markdownFile) != "" {
		return output.NewExitError("invalid_argument", 2, "Use either --markdown or --markdown-file, not both.", "Choose one profile body source.")
	}
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli id current` to confirm the active identity.")
	}
	params := identity.UpdateProfileParams{
		DisplayName:  displayName,
		Bio:          bio,
		TagsCSV:      tags,
		Markdown:     markdown,
		MarkdownFile: markdownFile,
	}
	if a.globals.DryRun {
		result := &identity.CommandResult{
			Data: map[string]any{
				"plan": map[string]any{
					"action":        "update_profile",
					"display_name":  displayName,
					"bio":           bio,
					"tags":          tags,
					"markdown":      markdown,
					"markdown_file": markdownFile,
					"remote_calls":  []string{"did.profile.update_me"},
					"local_writes":  []string{"identity.json", "index.json"},
				},
			},
			Summary: "Dry run: profile update planned",
		}
		return a.renderIdentityResult(cmd, format, result)
	}
	result, err := service.SetProfile(context.Background(), params)
	if err != nil {
		return a.identityExit(err, "Use an identity that already has a valid DID JWT.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func (a *App) runIDImportV1(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	importAll, _ := cmd.Flags().GetBool("all")
	service, format, err := a.identityService()
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the legacy credential paths.")
	}
	if a.globals.DryRun {
		result := &identity.CommandResult{
			Data: map[string]any{
				"plan": map[string]any{
					"action": "import_v1_identities",
					"name":   name,
					"all":    importAll,
				},
			},
			Summary: "Dry run: v1 credential import planned",
		}
		return a.renderIdentityResult(cmd, format, result)
	}
	result, err := service.ImportV1(name, importAll)
	if err != nil {
		return a.identityExit(err, "Run `awiki-cli doctor` to inspect the detected v1 credential layout.")
	}
	return a.renderIdentityResult(cmd, format, result)
}

func identityMetaFromData(data map[string]any) *output.IdentityMeta {
	if data == nil {
		return nil
	}
	for _, key := range []string{"identity", "active_identity", "default_identity"} {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case *identity.IdentitySummary:
			return &output.IdentityMeta{Name: typed.IdentityName, DID: typed.DID}
		case identity.IdentitySummary:
			return &output.IdentityMeta{Name: typed.IdentityName, DID: typed.DID}
		case map[string]any:
			name, _ := typed["identity_name"].(string)
			did, _ := typed["did"].(string)
			if name != "" || did != "" {
				return &output.IdentityMeta{Name: name, DID: did}
			}
		}
	}
	return nil
}
