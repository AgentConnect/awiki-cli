package site

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

func (s *Service) GetRoot(ctx context.Context, domain string) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	normalizedDomain, err := normalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(
		ctx,
		transportcfg.ProfileRPCReadHeavy,
		siteRPCEndpoint,
		"get_root",
		map[string]any{"domain": normalizedDomain},
		auth,
		&result,
	); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "site_root_get",
			"identity": identitySummaryFromRecord(record),
			"root":     result,
		},
		Summary: fmt.Sprintf("Fetched site root for %s", normalizedDomain),
	}, nil
}

func (s *Service) SetRoot(ctx context.Context, params SetRootParams) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	normalizedDomain, err := normalizeDomain(params.Domain)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(
		ctx,
		transportcfg.ProfileRPCDefault,
		siteRPCEndpoint,
		"set_root",
		map[string]any{"domain": normalizedDomain, "body": params.Body},
		auth,
		&result,
	); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "site_root_set",
			"identity": identitySummaryFromRecord(record),
			"root":     result,
		},
		Summary: fmt.Sprintf("Updated site root for %s", normalizedDomain),
	}, nil
}

func (s *Service) ListPages(ctx context.Context, domain string) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	normalizedDomain, err := normalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(
		ctx,
		transportcfg.ProfileRPCReadHeavy,
		siteRPCEndpoint,
		"list_pages",
		map[string]any{"domain": normalizedDomain},
		auth,
		&result,
	); err != nil {
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
			"action":   "site_page_list",
			"identity": identitySummaryFromRecord(record),
			"domain":   normalizedDomain,
			"pages":    result["pages"],
			"count":    count,
		},
		Summary: fmt.Sprintf("Fetched %d site pages for %s", count, normalizedDomain),
	}, nil
}

func (s *Service) GetPage(ctx context.Context, domain, slug string) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	normalizedDomain, err := normalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	normalizedSlug := strings.TrimSpace(slug)
	if normalizedSlug == "" {
		return nil, ErrSlugRequired
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(
		ctx,
		transportcfg.ProfileRPCReadHeavy,
		siteRPCEndpoint,
		"get_page",
		map[string]any{"domain": normalizedDomain, "slug": normalizedSlug},
		auth,
		&result,
	); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "site_page_get",
			"identity": identitySummaryFromRecord(record),
			"page":     result,
		},
		Summary: fmt.Sprintf("Fetched site page %s for %s", normalizedSlug, normalizedDomain),
	}, nil
}

func (s *Service) CreatePage(ctx context.Context, params CreatePageParams) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	normalizedDomain, err := normalizeDomain(params.Domain)
	if err != nil {
		return nil, err
	}
	normalizedSlug := strings.TrimSpace(params.Slug)
	if normalizedSlug == "" {
		return nil, ErrSlugRequired
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(
		ctx,
		transportcfg.ProfileRPCDefault,
		siteRPCEndpoint,
		"create_page",
		map[string]any{"domain": normalizedDomain, "slug": normalizedSlug, "body": params.Body},
		auth,
		&result,
	); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "site_page_create",
			"identity": identitySummaryFromRecord(record),
			"page":     result,
		},
		Summary: fmt.Sprintf("Created site page %s for %s", normalizedSlug, normalizedDomain),
	}, nil
}

func (s *Service) UpdatePage(ctx context.Context, params UpdatePageParams) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	normalizedDomain, err := normalizeDomain(params.Domain)
	if err != nil {
		return nil, err
	}
	normalizedSlug := strings.TrimSpace(params.Slug)
	if normalizedSlug == "" {
		return nil, ErrSlugRequired
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(
		ctx,
		transportcfg.ProfileRPCDefault,
		siteRPCEndpoint,
		"update_page",
		map[string]any{"domain": normalizedDomain, "slug": normalizedSlug, "body": params.Body},
		auth,
		&result,
	); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "site_page_update",
			"identity": identitySummaryFromRecord(record),
			"page":     result,
		},
		Summary: fmt.Sprintf("Updated site page %s for %s", normalizedSlug, normalizedDomain),
	}, nil
}

func (s *Service) RenamePage(ctx context.Context, params RenamePageParams) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	normalizedDomain, err := normalizeDomain(params.Domain)
	if err != nil {
		return nil, err
	}
	normalizedSlug := strings.TrimSpace(params.Slug)
	target := strings.TrimSpace(params.To)
	if normalizedSlug == "" || target == "" {
		return nil, ErrSlugRequired
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(
		ctx,
		transportcfg.ProfileRPCDefault,
		siteRPCEndpoint,
		"rename_page",
		map[string]any{"domain": normalizedDomain, "old_slug": normalizedSlug, "new_slug": target},
		auth,
		&result,
	); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "site_page_rename",
			"identity": identitySummaryFromRecord(record),
			"from":     normalizedSlug,
			"to":       target,
			"page":     result,
		},
		Summary: fmt.Sprintf("Renamed site page %s to %s for %s", normalizedSlug, target, normalizedDomain),
	}, nil
}

func (s *Service) DeletePage(ctx context.Context, domain, slug string) (*CommandResult, error) {
	record, auth, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	normalizedDomain, err := normalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	normalizedSlug := strings.TrimSpace(slug)
	if normalizedSlug == "" {
		return nil, ErrSlugRequired
	}
	var result map[string]any
	if err := s.remote.AuthenticatedRPCCallProfile(
		ctx,
		transportcfg.ProfileRPCDefault,
		siteRPCEndpoint,
		"delete_page",
		map[string]any{"domain": normalizedDomain, "slug": normalizedSlug},
		auth,
		&result,
	); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":   "site_page_delete",
			"identity": identitySummaryFromRecord(record),
			"domain":   normalizedDomain,
			"slug":     normalizedSlug,
			"result":   result,
		},
		Summary: fmt.Sprintf("Deleted site page %s for %s", normalizedSlug, normalizedDomain),
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
		finish := traceutil.EnsureJWTPhase(ctx, "site_bootstrap")
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

func normalizeDomain(value string) (string, error) {
	normalized, err := appconfig.NormalizeDIDDomain(value)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", ErrDomainRequired
	}
	return normalized, nil
}

func identitySummaryFromRecord(record *identity.StoredIdentity) map[string]any {
	if record == nil {
		return nil
	}
	return map[string]any{
		"identity_name": record.IdentityName,
		"handle":        record.Handle,
		"did":           record.DID,
	}
}
