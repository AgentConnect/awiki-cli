package site

import "errors"

const (
	siteRPCEndpoint    = "/site/rpc"
	didAuthRPCEndpoint = "/user-service/did-auth/rpc"
)

var (
	ErrDomainRequired       = errors.New("domain is required")
	ErrSlugRequired         = errors.New("slug is required")
	ErrNoBodySourceProvided = errors.New("provide either inline markdown or markdown file")
	ErrBodySourceConflict   = errors.New("use either inline markdown or markdown file, not both")
)

type ServiceError = identityServiceError

type CommandResult struct {
	Data     map[string]any
	Summary  string
	Warnings []string
}

type SetRootParams struct {
	Domain string
	Body   string
}

type CreatePageParams struct {
	Domain string
	Slug   string
	Body   string
}

type UpdatePageParams struct {
	Domain string
	Slug   string
	Body   string
}

type RenamePageParams struct {
	Domain string
	Slug   string
	To     string
}
