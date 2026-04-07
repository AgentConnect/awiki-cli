package content

import "errors"

const (
	contentRPCEndpoint = "/content/rpc"
	didAuthRPCEndpoint = "/user-service/did-auth/rpc"
)

var (
	ErrSlugRequired         = errors.New("slug is required")
	ErrTitleRequired        = errors.New("title is required")
	ErrBodySourceConflict   = errors.New("use either inline markdown or markdown file, not both")
	ErrNoUpdateFields       = errors.New("no page fields were provided for update")
	ErrVisibilityInvalid    = errors.New("visibility must be one of public, draft, or unlisted")
	ErrAuthIdentityRequired = errors.New("active identity is required")
)

type ServiceError = identityServiceError

type CommandResult struct {
	Data     map[string]any
	Summary  string
	Warnings []string
}

type CreatePageParams struct {
	Slug       string
	Title      string
	Body       string
	Visibility string
}

type UpdatePageParams struct {
	Slug       string
	Title      string
	Body       *string
	Visibility *string
}

type RenamePageParams struct {
	Slug string
	To   string
}
