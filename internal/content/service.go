package content

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/traceutil"
	"github.com/agentconnect/awiki-cli/internal/transportcfg"
)

type identityServiceError = identity.ServiceError

type Service struct {
	config  *appconfig.Resolved
	manager *identity.Manager
	remote  *identity.RemoteClient
}

func NewService(resolved *appconfig.Resolved) (*Service, error) {
	remote, err := identity.NewRemoteClient(resolved)
	if err != nil {
		return nil, err
	}
	return &Service{
		config:  resolved,
		manager: identity.NewManager(resolved.Paths),
		remote:  remote,
	}, nil
}

func (s *Service) Config() *appconfig.Resolved {
	return s.config
}

func (s *Service) CreatePage(ctx context.Context, params CreatePageParams) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	slug := strings.TrimSpace(params.Slug)
	title := strings.TrimSpace(params.Title)
	if slug == "" {
		return nil, ErrSlugRequired
	}
	if title == "" {
		return nil, ErrTitleRequired
	}
	visibility, err := normalizeVisibility(params.Visibility, false)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"slug":  slug,
		"title": title,
		"body":  params.Body,
	}
	if visibility != "" {
		payload["visibility"] = visibility
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCDefault, contentRPCEndpoint, "create", payload, auth, &result); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "create_page",
			"identity": identitySummaryFromRecord(record),
			"page":     result,
		},
		Summary: fmt.Sprintf("Created content page %s", slug),
	}, nil
}

func (s *Service) ListPages(ctx context.Context) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCReadHeavy, contentRPCEndpoint, "list", map[string]any{}, auth, &result); err != nil {
		return nil, err
	}
	count := 0
	switch typed := result["count"].(type) {
	case float64:
		count = int(typed)
	case int:
		count = typed
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "list_pages",
			"identity": identitySummaryFromRecord(record),
			"pages":    result["pages"],
			"count":    count,
		},
		Summary: fmt.Sprintf("Fetched %d content pages", count),
	}, nil
}

func (s *Service) GetPage(ctx context.Context, slug string) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrSlugRequired
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCReadHeavy, contentRPCEndpoint, "get", map[string]any{"slug": slug}, auth, &result); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "get_page",
			"identity": identitySummaryFromRecord(record),
			"page":     result,
		},
		Summary: fmt.Sprintf("Fetched content page %s", slug),
	}, nil
}

func (s *Service) UpdatePage(ctx context.Context, params UpdatePageParams) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	slug := strings.TrimSpace(params.Slug)
	if slug == "" {
		return nil, ErrSlugRequired
	}
	payload := map[string]any{"slug": slug}
	changedFields := make([]string, 0, 3)
	if title := strings.TrimSpace(params.Title); title != "" {
		payload["title"] = title
		changedFields = append(changedFields, "title")
	}
	if params.Body != nil {
		payload["body"] = *params.Body
		changedFields = append(changedFields, "body")
	}
	if params.Visibility != nil {
		visibility, err := normalizeVisibility(*params.Visibility, true)
		if err != nil {
			return nil, err
		}
		payload["visibility"] = visibility
		changedFields = append(changedFields, "visibility")
	}
	if len(changedFields) == 0 {
		return nil, ErrNoUpdateFields
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCDefault, contentRPCEndpoint, "update", payload, auth, &result); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":         "update_page",
			"identity":       identitySummaryFromRecord(record),
			"changed_fields": changedFields,
			"page":           result,
		},
		Summary: fmt.Sprintf("Updated content page %s", slug),
	}, nil
}

func (s *Service) RenamePage(ctx context.Context, params RenamePageParams) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	slug := strings.TrimSpace(params.Slug)
	target := strings.TrimSpace(params.To)
	if slug == "" || target == "" {
		return nil, ErrSlugRequired
	}
	var result map[string]any
	payload := map[string]any{"old_slug": slug, "new_slug": target}
	if err := s.remote.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCDefault, contentRPCEndpoint, "rename", payload, auth, &result); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "rename_page",
			"identity": identitySummaryFromRecord(record),
			"from":     slug,
			"to":       target,
			"page":     result,
		},
		Summary: fmt.Sprintf("Renamed content page %s to %s", slug, target),
	}, nil
}

func (s *Service) DeletePage(ctx context.Context, slug string) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrSlugRequired
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCDefault, contentRPCEndpoint, "delete", map[string]any{"slug": slug}, auth, &result); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "delete_page",
			"identity": identitySummaryFromRecord(record),
			"slug":     slug,
			"result":   result,
		},
		Summary: fmt.Sprintf("Deleted content page %s", slug),
	}, nil
}

func (s *Service) requireAuth(ctx context.Context) (*identity.StoredIdentity, *authsdk.Session, error) {
	record, err := s.requireActiveIdentity()
	if err != nil {
		return nil, nil, err
	}
	session, err := s.authSession(ctx, record)
	if err != nil {
		return nil, nil, err
	}
	return record, session, nil
}

func (s *Service) requireActiveIdentity() (*identity.StoredIdentity, error) {
	if strings.TrimSpace(s.config.ActiveIdentity) == "" {
		current, err := s.manager.Current()
		if err != nil {
			if errors.Is(err, identity.ErrNoDefaultIdentity) {
				return nil, fmt.Errorf("%w: no active identity is configured", identity.ErrIdentityNotFound)
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

func (s *Service) authSession(ctx context.Context, record *identity.StoredIdentity) (*authsdk.Session, error) {
	if record == nil {
		return nil, fmt.Errorf("%w: active identity is required", identity.ErrAuthRequired)
	}
	paths, err := s.manager.PathsForIdentity(record.IdentityName)
	if err != nil {
		return nil, err
	}
	session := authsdk.NewSession(
		paths.DIDDocumentPath,
		paths.Key1PrivatePath,
		record.IdentityName,
		record.DID,
		record.JWTToken,
		func(token string) error { return s.manager.UpdateJWT(record.IdentityName, token) },
	)
	session.RememberScope(s.config.ServiceBaseURL)
	session.RememberScope(appconfig.JoinBaseURL(s.config.ServiceBaseURL, didAuthRPCEndpoint))
	session.SetBearer(s.config.ServiceBaseURL, record.JWTToken)
	session.SetBearer(appconfig.JoinBaseURL(s.config.ServiceBaseURL, didAuthRPCEndpoint), record.JWTToken)
	if strings.TrimSpace(record.JWTToken) == "" {
		refreshCtx, cancel := transportcfg.WithProfileTimeout(ctx, transportcfg.ProfileAuthRefresh)
		defer cancel()
		finish := traceutil.EnsureJWTPhase(ctx, "content_bootstrap")
		defer finish()
		if _, err := session.EnsureJWT(
			refreshCtx,
			s.remote.Client(),
			appconfig.JoinBaseURL(s.config.ServiceBaseURL, didAuthRPCEndpoint),
		); err != nil {
			var httpErr *authsdk.HTTPError
			if errors.As(err, &httpErr) {
				return nil, &identityServiceError{StatusCode: httpErr.StatusCode, Message: httpErr.Message}
			}
			var rpcErr *authsdk.RPCError
			if errors.As(err, &rpcErr) {
				return nil, &identityServiceError{RPCCode: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
			}
			return nil, err
		}
	}
	return session, nil
}

func normalizeVisibility(value string, emptyAllowed bool) (string, error) {
	visibility := strings.ToLower(strings.TrimSpace(value))
	if visibility == "" && emptyAllowed {
		return "", nil
	}
	if visibility == "" {
		return "public", nil
	}
	switch visibility {
	case "public", "draft", "unlisted":
		return visibility, nil
	default:
		return "", ErrVisibilityInvalid
	}
}

func identitySummaryFromRecord(record *identity.StoredIdentity) map[string]any {
	if record == nil {
		return nil
	}
	return map[string]any{
		"identity_name": record.IdentityName,
		"did":           record.DID,
		"handle":        record.Handle,
	}
}
