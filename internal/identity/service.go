package identity

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

var (
	phoneIntlPattern    = regexp.MustCompile(`^\+\d{1,3}\d{6,14}$`)
	phoneCNLocalPattern = regexp.MustCompile(`^1[3-9]\d{9}$`)
)

type Service struct {
	config  *appconfig.Resolved
	manager *Manager
	remote  *RemoteClient
}

func NewService(resolved *appconfig.Resolved) (*Service, error) {
	remote, err := NewRemoteClient(resolved)
	if err != nil {
		return nil, err
	}
	return &Service{
		config:  resolved,
		manager: NewManager(resolved.Paths),
		remote:  remote,
	}, nil
}

func (s *Service) Manager() *Manager {
	return s.manager
}

func (s *Service) Config() *appconfig.Resolved {
	return s.config
}

func (s *Service) Status() (*CommandResult, error) {
	current, currentErr := s.manager.Current()
	legacy, err := s.manager.ScanLegacy()
	if err != nil {
		return nil, err
	}
	identities, err := s.manager.List()
	if err != nil {
		return nil, err
	}
	data := map[string]any{
		"active_identity": current,
		"identity_count":  len(identities),
		"legacy_scan":     legacy,
	}
	warnings := make([]string, 0)
	if legacy.HasLegacy {
		warnings = append(warnings, LegacyLayoutHint)
	}
	summary := "Identity store is ready"
	if currentErr != nil {
		summary = "No default identity is configured yet"
	}
	return &CommandResult{Data: data, Summary: summary, Warnings: warnings}, nil
}

func (s *Service) List() (*CommandResult, error) {
	identities, err := s.manager.List()
	if err != nil {
		return nil, err
	}
	current, _ := s.manager.Current()
	legacy, err := s.manager.ScanLegacy()
	if err != nil {
		return nil, err
	}
	warnings := make([]string, 0)
	if legacy.HasLegacy {
		warnings = append(warnings, LegacyLayoutHint)
	}
	return &CommandResult{
		Data: map[string]any{
			"identities":       identities,
			"default_identity": current,
			"legacy_scan":      legacy,
		},
		Summary:  fmt.Sprintf("Found %d local identities", len(identities)),
		Warnings: warnings,
	}, nil
}

func (s *Service) Current() (*CommandResult, error) {
	current, err := s.manager.Current()
	if err != nil {
		if strings.Contains(err.Error(), ErrNoDefaultIdentity.Error()) {
			return &CommandResult{
				Data:    map[string]any{"identity": nil},
				Summary: "No default identity is configured",
			}, nil
		}
		return nil, err
	}
	return &CommandResult{
		Data:    map[string]any{"identity": current},
		Summary: fmt.Sprintf("Current identity is %s", current.IdentityName),
	}, nil
}

func (s *Service) Use(identityName string) (*CommandResult, error) {
	summary, err := s.manager.SetDefault(identityName)
	if err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "set_default_identity",
			"identity": summary,
		},
		Summary: fmt.Sprintf("Default identity switched to %s", summary.IdentityName),
	}, nil
}

func (s *Service) Create(displayName string, identityName string) (*CommandResult, error) {
	existing, err := s.manager.List()
	if err != nil {
		return nil, err
	}
	alias := chooseDefaultIdentityName(identityName, existing, displayName)
	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:    s.config.DIDDomain,
		PathPrefix:  []string{"user"},
		ProofDomain: s.config.DIDDomain,
	})
	if err != nil {
		return nil, err
	}
	record, err := s.manager.save(SaveInput{
		IdentityName:            alias,
		DID:                     generated.DID,
		UniqueID:                generated.UniqueID,
		DisplayName:             displayName,
		DIDDocument:             generated.DIDDocument,
		Key1PrivatePEM:          generated.Key1PrivatePEM,
		Key1PublicPEM:           generated.Key1PublicPEM,
		E2EESigningPrivatePEM:   generated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: generated.E2EEAgreementPrivatePEM,
	})
	if err != nil {
		return nil, err
	}
	summary := identitySummaryFromRecord(record)
	return &CommandResult{
		Data: map[string]any{
			"action":   "create_identity",
			"identity": summary,
		},
		Summary: fmt.Sprintf("Created local identity %s", summary.IdentityName),
	}, nil
}

func (s *Service) Register(ctx context.Context, params RegisterParams) (*CommandResult, error) {
	handle := strings.TrimSpace(params.Handle)
	if handle == "" {
		return nil, fmt.Errorf("%w: handle is required", ErrInvalidInput)
	}
	phone := strings.TrimSpace(params.Phone)
	email := strings.TrimSpace(strings.ToLower(params.Email))
	if (phone == "" && email == "") || (phone != "" && email != "") {
		return nil, fmt.Errorf("%w: exactly one of phone or email is required", ErrInvalidInput)
	}

	existing, err := s.manager.List()
	if err != nil {
		return nil, err
	}
	alias := chooseNamedIdentity(params.IdentityName, existing, handle)

	if phone != "" && strings.TrimSpace(params.OTP) == "" {
		normalizedPhone, err := normalizePhone(phone)
		if err != nil {
			return nil, err
		}
		var result map[string]any
		if err := s.remote.rpcCall(ctx, handleRPCEndpoint, "send_otp", map[string]any{"phone": normalizedPhone}, "", &result); err != nil {
			return nil, err
		}
		return &CommandResult{
			Data: map[string]any{
				"action":             "send_handle_otp",
				"identity_name":      alias,
				"handle":             handle,
				"method":             "phone",
				"phone":              normalizedPhone,
				"verification_state": "otp_sent",
				"result":             result,
			},
			Summary: fmt.Sprintf("OTP sent for handle %s", handle),
		}, nil
	}

	if email != "" {
		verified, verifiedAt, err := s.checkEmailVerified(ctx, email)
		if err != nil {
			return nil, err
		}
		if !verified {
			var sendResult map[string]any
			if err := s.remote.restPost(ctx, emailSendEndpoint, map[string]any{"email": email}, "", &sendResult); err != nil {
				return nil, err
			}
			if !params.Wait {
				return &CommandResult{
					Data: map[string]any{
						"action":             "send_registration_email",
						"identity_name":      alias,
						"handle":             handle,
						"method":             "email",
						"email":              email,
						"verification_state": "email_sent",
						"result":             sendResult,
					},
					Summary: fmt.Sprintf("Activation email sent for handle %s", handle),
				}, nil
			}
			timeout := params.VerificationTimeout
			if timeout <= 0 {
				timeout = DefaultEmailVerificationSecs
			}
			pollInterval := params.PollIntervalSeconds
			if pollInterval <= 0 {
				pollInterval = DefaultEmailPollIntervalSecs
			}
			verified, verifiedAt, err = s.waitForEmailVerification(ctx, email, timeout, pollInterval)
			if err != nil {
				return nil, err
			}
			if !verified {
				return &CommandResult{
					Data: map[string]any{
						"action":             "wait_for_registration_email",
						"identity_name":      alias,
						"handle":             handle,
						"method":             "email",
						"email":              email,
						"verification_state": "pending",
					},
					Summary: "Email verification is still pending",
				}, nil
			}
		}
		_ = verifiedAt
	}

	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:    s.config.DIDDomain,
		PathPrefix:  []string{handle},
		ProofDomain: s.config.DIDDomain,
	})
	if err != nil {
		return nil, err
	}
	registerParams := map[string]any{
		"did_document": generated.DIDDocument,
		"handle":       handle,
	}
	if phone != "" {
		normalizedPhone, err := normalizePhone(phone)
		if err != nil {
			return nil, err
		}
		registerParams["phone"] = normalizedPhone
		registerParams["otp_code"] = sanitizeOTP(params.OTP)
	}
	if email != "" {
		registerParams["email"] = email
	}
	if params.InviteCode != "" {
		registerParams["invite_code"] = params.InviteCode
	}
	var result map[string]any
	if err := s.remote.rpcCall(ctx, didAuthRPCEndpoint, "register", registerParams, "", &result); err != nil {
		return nil, err
	}
	record, err := s.manager.save(SaveInput{
		IdentityName:            alias,
		DID:                     stringValue(result["did"], generated.DID),
		UniqueID:                generated.UniqueID,
		UserID:                  stringValue(result["user_id"], ""),
		DisplayName:             handle,
		Handle:                  handle,
		JWTToken:                stringValue(result["access_token"], ""),
		DIDDocument:             generated.DIDDocument,
		Key1PrivatePEM:          generated.Key1PrivatePEM,
		Key1PublicPEM:           generated.Key1PublicPEM,
		E2EESigningPrivatePEM:   generated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: generated.E2EEAgreementPrivatePEM,
	})
	if err != nil {
		return nil, err
	}
	summary := identitySummaryFromRecord(record)
	return &CommandResult{
		Data: map[string]any{
			"action":             "register_handle",
			"identity":           summary,
			"method":             ternaryString(phone != "", "phone", "email"),
			"verification_state": "completed",
			"result":             result,
		},
		Summary: fmt.Sprintf("Handle %s registered successfully", handle),
	}, nil
}

func (s *Service) Bind(ctx context.Context, params BindParams) (*CommandResult, error) {
	record, err := s.requireActiveIdentity()
	if err != nil {
		return nil, err
	}
	if record.JWTToken == "" {
		return nil, fmt.Errorf("%w: active identity does not have a JWT yet", ErrAuthRequired)
	}
	phone := strings.TrimSpace(params.Phone)
	email := strings.TrimSpace(strings.ToLower(params.Email))
	if (phone == "" && email == "") || (phone != "" && email != "") {
		return nil, fmt.Errorf("%w: exactly one of phone or email is required", ErrInvalidInput)
	}
	if phone != "" {
		normalizedPhone, err := normalizePhone(phone)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(params.OTP) == "" {
			var result map[string]any
			if err := s.remote.restPost(ctx, phoneBindSendEndpoint, map[string]any{"phone": normalizedPhone}, record.JWTToken, &result); err != nil {
				return nil, err
			}
			return &CommandResult{
				Data: map[string]any{
					"action":             "send_bind_phone_otp",
					"identity":           identitySummaryFromRecord(record),
					"phone":              normalizedPhone,
					"verification_state": "otp_sent",
					"result":             result,
				},
				Summary: "Phone binding OTP sent",
			}, nil
		}
		var result map[string]any
		if err := s.remote.restPost(ctx, phoneBindVerifyEndpoint, map[string]any{"phone": normalizedPhone, "code": sanitizeOTP(params.OTP)}, record.JWTToken, &result); err != nil {
			return nil, err
		}
		return &CommandResult{
			Data: map[string]any{
				"action":             "bind_phone",
				"identity":           identitySummaryFromRecord(record),
				"phone":              normalizedPhone,
				"verification_state": "completed",
				"result":             result,
			},
			Summary: "Phone bound successfully",
		}, nil
	}

	verified, _, err := s.checkEmailVerified(ctx, email)
	if err != nil {
		return nil, err
	}
	if !verified {
		var sendResult map[string]any
		if err := s.remote.restPost(ctx, emailSendEndpoint, map[string]any{"email": email}, record.JWTToken, &sendResult); err != nil {
			return nil, err
		}
		if !params.Wait {
			return &CommandResult{
				Data: map[string]any{
					"action":             "send_bind_email",
					"identity":           identitySummaryFromRecord(record),
					"email":              email,
					"verification_state": "email_sent",
					"result":             sendResult,
				},
				Summary: "Binding email sent",
			}, nil
		}
		timeout := params.VerificationTimeout
		if timeout <= 0 {
			timeout = DefaultEmailVerificationSecs
		}
		pollInterval := params.PollIntervalSeconds
		if pollInterval <= 0 {
			pollInterval = DefaultEmailPollIntervalSecs
		}
		verified, _, err = s.waitForEmailVerification(ctx, email, timeout, pollInterval)
		if err != nil {
			return nil, err
		}
		if !verified {
			return &CommandResult{
				Data: map[string]any{
					"action":             "wait_for_bind_email",
					"identity":           identitySummaryFromRecord(record),
					"email":              email,
					"verification_state": "pending",
				},
				Summary: "Email verification is still pending",
			}, nil
		}
	}
	return &CommandResult{
		Data: map[string]any{
			"action":             "bind_email",
			"identity":           identitySummaryFromRecord(record),
			"email":              email,
			"verification_state": "completed",
		},
		Summary: "Email binding verified successfully",
	}, nil
}

func (s *Service) Resolve(ctx context.Context, handle string, did string) (*CommandResult, error) {
	handle = strings.TrimSpace(handle)
	did = strings.TrimSpace(did)
	if (handle == "" && did == "") || (handle != "" && did != "") {
		return nil, fmt.Errorf("%w: exactly one of handle or did is required", ErrInvalidInput)
	}
	data := map[string]any{}
	warnings := make([]string, 0)
	if handle != "" {
		var lookup map[string]any
		if err := s.remote.rpcCall(ctx, handleRPCEndpoint, "lookup", map[string]any{"handle": handle}, "", &lookup); err != nil {
			return nil, err
		}
		data["lookup"] = lookup
		did = stringValue(lookup["did"], "")
		if did == "" {
			return nil, fmt.Errorf("%w: handle %s did not resolve to a did", ErrIdentityNotFound, handle)
		}
		var profile map[string]any
		if err := s.remote.rpcCall(ctx, didProfileRPCEndpoint, "get_public_profile", map[string]any{"handle": handle}, "", &profile); err == nil {
			data["public_profile"] = profile
		} else {
			warnings = append(warnings, fmt.Sprintf("Public profile lookup failed: %v", err))
		}
	}
	if did != "" {
		var resolve map[string]any
		if err := s.remote.rpcCall(ctx, didProfileRPCEndpoint, "resolve", map[string]any{"did": did}, "", &resolve); err != nil {
			return nil, err
		}
		data["resolve"] = resolve
		if handle == "" {
			var lookup map[string]any
			if err := s.remote.rpcCall(ctx, handleRPCEndpoint, "lookup", map[string]any{"did": did}, "", &lookup); err == nil {
				data["lookup"] = lookup
			} else {
				warnings = append(warnings, fmt.Sprintf("Handle lookup failed: %v", err))
			}
			var profile map[string]any
			if err := s.remote.rpcCall(ctx, didProfileRPCEndpoint, "get_public_profile", map[string]any{"did": did}, "", &profile); err == nil {
				data["public_profile"] = profile
			} else {
				warnings = append(warnings, fmt.Sprintf("Public profile lookup failed: %v", err))
			}
		}
	}
	return &CommandResult{
		Data:     data,
		Summary:  "Identity resolved successfully",
		Warnings: warnings,
	}, nil
}

func (s *Service) Recover(ctx context.Context, params RecoverParams) (*CommandResult, error) {
	handle := strings.TrimSpace(params.Handle)
	phone := strings.TrimSpace(params.Phone)
	otp := strings.TrimSpace(params.OTP)
	if handle == "" || phone == "" || otp == "" {
		return nil, fmt.Errorf("%w: handle, phone, and otp are required", ErrInvalidInput)
	}
	existing, err := s.manager.List()
	if err != nil {
		return nil, err
	}
	alias := chooseNamedIdentity(params.IdentityName, existing, handle)
	normalizedPhone, err := normalizePhone(phone)
	if err != nil {
		return nil, err
	}
	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:    s.config.DIDDomain,
		PathPrefix:  []string{handle},
		ProofDomain: s.config.DIDDomain,
	})
	if err != nil {
		return nil, err
	}
	recoverParams := map[string]any{
		"did_document": generated.DIDDocument,
		"handle":       handle,
		"phone":        normalizedPhone,
		"otp_code":     sanitizeOTP(otp),
	}
	var result map[string]any
	if err := s.remote.rpcCall(ctx, didAuthRPCEndpoint, "recover_handle", recoverParams, "", &result); err != nil {
		return nil, err
	}
	record, err := s.manager.save(SaveInput{
		IdentityName:            alias,
		DID:                     stringValue(result["did"], generated.DID),
		UniqueID:                generated.UniqueID,
		UserID:                  stringValue(result["user_id"], ""),
		DisplayName:             handle,
		Handle:                  handle,
		JWTToken:                stringValue(result["access_token"], ""),
		DIDDocument:             generated.DIDDocument,
		Key1PrivatePEM:          generated.Key1PrivatePEM,
		Key1PublicPEM:           generated.Key1PublicPEM,
		E2EESigningPrivatePEM:   generated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: generated.E2EEAgreementPrivatePEM,
	})
	if err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "recover_handle",
			"identity": identitySummaryFromRecord(record),
			"result":   result,
		},
		Summary: fmt.Sprintf("Handle %s recovered successfully", handle),
	}, nil
}

func (s *Service) GetProfile(ctx context.Context, self bool, handle string, did string) (*CommandResult, error) {
	if !self && handle == "" && did == "" {
		self = true
	}
	if self {
		record, err := s.requireActiveIdentity()
		if err != nil {
			return nil, err
		}
		if record.JWTToken == "" {
			return nil, fmt.Errorf("%w: active identity does not have a JWT yet", ErrAuthRequired)
		}
		var result map[string]any
		if err := s.remote.rpcCall(ctx, didProfileRPCEndpoint, "get_me", map[string]any{}, record.JWTToken, &result); err != nil {
			return nil, err
		}
		return &CommandResult{
			Data: map[string]any{
				"subject": "self",
				"profile": result,
			},
			Summary: "Fetched current identity profile",
		}, nil
	}
	params := map[string]any{}
	if handle != "" {
		params["handle"] = handle
	}
	if did != "" {
		params["did"] = did
	}
	var result map[string]any
	if err := s.remote.rpcCall(ctx, didProfileRPCEndpoint, "get_public_profile", params, "", &result); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"subject": params,
			"profile": result,
		},
		Summary: "Fetched public profile",
	}, nil
}

func (s *Service) SetProfile(ctx context.Context, params UpdateProfileParams) (*CommandResult, error) {
	record, err := s.requireActiveIdentity()
	if err != nil {
		return nil, err
	}
	if record.JWTToken == "" {
		return nil, fmt.Errorf("%w: active identity does not have a JWT yet", ErrAuthRequired)
	}
	payload := map[string]any{}
	changedFields := make([]string, 0)
	if strings.TrimSpace(params.DisplayName) != "" {
		payload["nick_name"] = strings.TrimSpace(params.DisplayName)
		changedFields = append(changedFields, "display_name")
	}
	if strings.TrimSpace(params.Bio) != "" {
		payload["bio"] = strings.TrimSpace(params.Bio)
		changedFields = append(changedFields, "bio")
	}
	if strings.TrimSpace(params.TagsCSV) != "" {
		tags := splitCSV(params.TagsCSV)
		payload["tags"] = tags
		changedFields = append(changedFields, "tags")
	}
	markdown := strings.TrimSpace(params.Markdown)
	if strings.TrimSpace(params.MarkdownFile) != "" {
		raw, err := os.ReadFile(params.MarkdownFile)
		if err != nil {
			return nil, err
		}
		markdown = string(raw)
	}
	if strings.TrimSpace(markdown) != "" {
		payload["profile_md"] = markdown
		changedFields = append(changedFields, "profile_md")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("%w: no profile fields were provided", ErrInvalidInput)
	}
	var result map[string]any
	if err := s.remote.rpcCall(ctx, didProfileRPCEndpoint, "update_me", payload, record.JWTToken, &result); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.DisplayName) != "" {
		_ = s.manager.UpdateDisplayName(record.IdentityName, strings.TrimSpace(params.DisplayName))
	}
	return &CommandResult{
		Data: map[string]any{
			"action":         "update_profile",
			"identity":       identitySummaryFromRecord(record),
			"changed_fields": changedFields,
			"profile":        result,
		},
		Summary: "Profile updated successfully",
	}, nil
}

func (s *Service) ImportV1(name string, all bool) (*CommandResult, error) {
	var (
		result *ImportResult
		err    error
	)
	if all {
		result, err = s.manager.ImportAllLegacy()
	} else {
		result, err = s.manager.ImportLegacy(name)
	}
	if err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"result": result,
		},
		Summary: fmt.Sprintf("Imported %d legacy identities", len(result.Imported)),
	}, nil
}

func (s *Service) requireActiveIdentity() (*StoredIdentity, error) {
	if strings.TrimSpace(s.config.ActiveIdentity) == "" {
		current, err := s.manager.Current()
		if err != nil {
			if errors.Is(err, ErrNoDefaultIdentity) {
				return nil, fmt.Errorf("%w: no active identity is configured", ErrIdentityNotFound)
			}
			return nil, err
		}
		s.config.ActiveIdentity = current.IdentityName
	}
	record, err := s.manager.Load(s.config.ActiveIdentity)
	if err != nil {
		return nil, err
	}
	return record, nil
}

func normalizePhone(phone string) (string, error) {
	phone = strings.TrimSpace(phone)
	switch {
	case phoneIntlPattern.MatchString(phone):
		return phone, nil
	case phoneCNLocalPattern.MatchString(phone):
		return "+86" + phone, nil
	default:
		return "", fmt.Errorf("%w: invalid phone number %q", ErrInvalidInput, phone)
	}
}

func sanitizeOTP(code string) string {
	return strings.Join(strings.Fields(code), "")
}

func splitCSV(raw string) []string {
	items := strings.Split(raw, ",")
	values := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func (s *Service) checkEmailVerified(ctx context.Context, email string) (bool, string, error) {
	var result struct {
		Email      string `json:"email"`
		Verified   bool   `json:"verified"`
		VerifiedAt string `json:"verified_at"`
	}
	if err := s.remote.restGet(ctx, emailStatusEndpoint, url.Values{"email": {strings.ToLower(strings.TrimSpace(email))}}, &result); err != nil {
		if serviceErr, ok := err.(*ServiceError); ok && serviceErr.StatusCode == 404 {
			return false, "", nil
		}
		return false, "", err
	}
	return result.Verified, result.VerifiedAt, nil
}

func (s *Service) waitForEmailVerification(ctx context.Context, email string, timeoutSecs int, pollIntervalSecs float64) (bool, string, error) {
	if timeoutSecs <= 0 {
		timeoutSecs = DefaultEmailVerificationSecs
	}
	if pollIntervalSecs <= 0 {
		pollIntervalSecs = DefaultEmailPollIntervalSecs
	}
	deadline := time.Now().Add(time.Duration(timeoutSecs) * time.Second)
	for time.Now().Before(deadline) {
		verified, verifiedAt, err := s.checkEmailVerified(ctx, email)
		if err != nil {
			return false, "", err
		}
		if verified {
			return true, verifiedAt, nil
		}
		select {
		case <-ctx.Done():
			return false, "", ctx.Err()
		case <-time.After(time.Duration(pollIntervalSecs * float64(time.Second))):
		}
	}
	return false, "", nil
}

func ternaryString(condition bool, yes string, no string) string {
	if condition {
		return yes
	}
	return no
}

func identitySummaryFromRecord(record *StoredIdentity) *IdentitySummary {
	if record == nil {
		return nil
	}
	return &IdentitySummary{
		IdentityName:            record.IdentityName,
		DID:                     record.DID,
		UniqueID:                record.UniqueID,
		UserID:                  record.UserID,
		DisplayName:             record.DisplayName,
		Handle:                  record.Handle,
		CreatedAt:               record.CreatedAt,
		DirName:                 record.DirName,
		IsDefault:               record.IsDefault,
		HasJWT:                  record.JWTToken != "",
		HasDIDDocument:          record.DIDDocument != nil,
		HasKey1Private:          record.Key1PrivatePEM != "",
		HasKey1Public:           record.Key1PublicPEM != "",
		HasE2EESigningPrivate:   record.E2EESigningPrivatePEM != "",
		HasE2EEAgreementPrivate: record.E2EEAgreementPrivatePEM != "",
	}
}
