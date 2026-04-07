package message

import "errors"

const (
	MessageRPCEndpoint = "/rpc"
	MessageWSEndpoint  = "/ws"
)

var (
	ErrTargetRequired       = errors.New("direct message target is required")
	ErrGroupRequired        = errors.New("group target is required")
	ErrMemberRequired       = errors.New("group member target is required")
	ErrTextRequired         = errors.New("message text is required")
	ErrTransportUnavailable = errors.New("message transport is unavailable")
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
	Group        string
	Text         string
	MessageType  string
	SecureMode   string
}

type InboxRequest struct {
	IdentityName string
	Scope        string
	With         string
	Group        string
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

type GroupCreateRequest struct {
	IdentityName        string
	Name                string
	Description         string
	Discoverability     string
	AdmissionMode       string
	Slug                string
	Goal                string
	Rules               string
	MessagePrompt       string
	DocURL              string
	AttachmentsAllowed  *bool
	MaxMembers          string
	MemberMaxMessages   *int64
	MemberMaxTotalChars *int64
}

type GroupGetRequest struct {
	IdentityName string
	Group        string
}

type GroupInfoRequest struct {
	IdentityName      string
	Group             string
	IncludePolicy     bool
	IncludeMemberList bool
}

type GroupJoinRequest struct {
	IdentityName string
	Group        string
	ReasonText   string
}

type GroupMemberRequest struct {
	IdentityName string
	Group        string
	Member       string
	Role         string
	ReasonText   string
}

type GroupLeaveRequest struct {
	IdentityName string
	Group        string
}

type GroupUpdateRequest struct {
	IdentityName        string
	Group               string
	Name                string
	Description         string
	Discoverability     string
	AdmissionMode       string
	Slug                string
	Goal                string
	Rules               string
	MessagePrompt       string
	DocURL              string
	AttachmentsAllowed  *bool
	MaxMembers          string
	MemberMaxMessages   *int64
	MemberMaxTotalChars *int64
}

type GroupMembersRequest struct {
	IdentityName string
	Group        string
	Limit        int
}

type GroupMessagesRequest struct {
	IdentityName string
	Group        string
	Limit        int
	Cursor       string
}

type groupSendResult struct {
	Accepted          bool   `json:"accepted"`
	FinalAcceptance   bool   `json:"final_acceptance"`
	GroupDID          string `json:"group_did"`
	MessageID         string `json:"message_id"`
	OperationID       string `json:"operation_id"`
	GroupEventSeq     string `json:"group_event_seq"`
	GroupStateVersion string `json:"group_state_version"`
	AcceptedAt        string `json:"accepted_at"`
}
