package message

import "errors"

const (
	MessageRPCEndpoint = "/message/rpc"
)

var (
	ErrTargetRequired       = errors.New("direct message target is required")
	ErrTextRequired         = errors.New("message text is required")
	ErrTransportUnavailable = errors.New("message transport is unavailable")
	ErrGroupNotSupported    = errors.New("group messaging is not implemented yet")
	ErrSecureNotSupported   = errors.New("direct secure messaging is not implemented yet")
	ErrMessageNotFound      = errors.New("message not found")
)

type CommandResult struct {
	Data     map[string]any
	Summary  string
	Warnings []string
}

type SendRequest struct {
	IdentityName string
	Target       string
	Text         string
	MessageType  string
	SecureMode   string
}

type InboxRequest struct {
	IdentityName string
	Scope        string
	With         string
	Limit        int
	UnreadOnly   bool
	MarkRead     bool
}

type HistoryRequest struct {
	IdentityName string
	With         string
	Limit        int
	Cursor       string
}

type MarkReadRequest struct {
	IdentityName string
	MessageIDs   []string
}

type directSendResult struct {
	Accepted        bool   `json:"accepted"`
	MessageID       string `json:"message_id"`
	OperationID     string `json:"operation_id"`
	TargetDID       string `json:"target_did"`
	AcceptedAt      string `json:"accepted_at"`
	FinalAcceptance bool   `json:"final_acceptance"`
	DeliveryState   string `json:"delivery_state"`
}
