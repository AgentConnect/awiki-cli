package mail

import "errors"

const (
	MailRPCEndpoint = "/mail/rpc"
)

var (
	ErrMessageIDRequired   = errors.New("message id is required")
	ErrRecipientRequired   = errors.New("mail recipient is required")
	ErrSubjectRequired     = errors.New("mail subject is required")
	ErrBodyRequired        = errors.New("mail body is required")
	ErrAttachmentIndexZero = errors.New("attachment index must be >= 0")
)

type CommandResult struct {
	Data     map[string]any
	Summary  string
	Warnings []string
}

type InboxRequest struct {
	IdentityName string
	Folder       string
	Limit        int
	Offset       int
	UnreadOnly   bool
}

type ReadRequest struct {
	IdentityName string
	MessageID    string
}

type MarkReadRequest struct {
	IdentityName string
	MessageIDs   []string
	IsRead       bool
}

type AccountRequest struct {
	IdentityName string
}

type AttachmentRequest struct {
	IdentityName    string
	MessageID       string
	AttachmentIndex int
}

type SendRequest struct {
	IdentityName string
	To           []string
	CC           []string
	Subject      string
	BodyText     string
	BodyHTML     string
}
