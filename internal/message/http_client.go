package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

type Transport interface {
	SendDirect(context.Context, SendRequest) (*directSendResult, error)
	SendGroup(context.Context, SendRequest) (*groupSendResult, error)
	GetInbox(context.Context, InboxRequest) (map[string]any, error)
	GetHistory(context.Context, HistoryRequest) (map[string]any, error)
	MarkRead(context.Context, MarkReadRequest) (map[string]any, error)
	CreateGroup(context.Context, GroupCreateRequest) (map[string]any, error)
	GetGroupInfo(context.Context, GroupInfoRequest) (map[string]any, error)
	JoinGroup(context.Context, GroupJoinRequest) (map[string]any, error)
	AddGroupMember(context.Context, GroupMemberRequest) (map[string]any, error)
	RemoveGroupMember(context.Context, GroupMemberRequest) (map[string]any, error)
	LeaveGroup(context.Context, GroupLeaveRequest) (map[string]any, error)
	GetGroup(context.Context, GroupGetRequest) (map[string]any, error)
	ListGroupMembers(context.Context, GroupMembersRequest) (map[string]any, error)
	ListGroupMessages(context.Context, GroupMessagesRequest) (map[string]any, error)
	UpdateGroupProfile(context.Context, GroupGetRequest, map[string]any) (map[string]any, error)
	UpdateGroupPolicy(context.Context, GroupGetRequest, map[string]any) (map[string]any, error)
}

type ServiceError struct {
	StatusCode int
	RPCCode    int
	Message    string
	Data       any
}

func (e *ServiceError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.RPCCode != 0:
		return fmt.Sprintf("message service rpc error %d: %s", e.RPCCode, e.Message)
	case e.StatusCode != 0:
		return fmt.Sprintf("message service http error %d: %s", e.StatusCode, e.Message)
	default:
		return e.Message
	}
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

type HTTPTransport struct {
	resolved       *appconfig.Resolved
	auth           *authContext
	httpClient     *http.Client
	rpcEndpointURL string
}

func NewHTTPTransport(resolved *appconfig.Resolved, auth *authContext, httpClient *http.Client) *HTTPTransport {
	return NewHTTPTransportForRPCEndpoint(
		resolved,
		auth,
		httpClient,
		appconfig.JoinBaseURL(resolved.ServiceBaseURL, MessageRPCEndpoint),
	)
}

func NewHTTPTransportForRPCEndpoint(
	resolved *appconfig.Resolved,
	auth *authContext,
	httpClient *http.Client,
	rpcEndpointURL string,
) *HTTPTransport {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &HTTPTransport{
		resolved:       resolved,
		auth:           auth,
		httpClient:     httpClient,
		rpcEndpointURL: strings.TrimSpace(rpcEndpointURL),
	}
}

func (t *HTTPTransport) WithRPCEndpoint(rpcEndpointURL string) *HTTPTransport {
	if t == nil {
		return nil
	}
	return NewHTTPTransportForRPCEndpoint(
		t.resolved,
		t.auth,
		t.httpClient,
		rpcEndpointURL,
	)
}

func (t *HTTPTransport) SendDirect(ctx context.Context, request SendRequest) (*directSendResult, error) {
	params, err := BuildDirectSendRPCParams(
		t.auth.record,
		nil,
		request.Target,
		request.Text,
		request.MessageType,
	)
	if err != nil {
		return nil, err
	}
	meta, _ := params["meta"].(map[string]any)
	var result directSendResult
	if err := t.rpcCall(ctx, "direct.send", params, &result); err != nil {
		return nil, err
	}
	if result.MessageID == "" {
		result.MessageID = stringFromAny(meta["message_id"])
	}
	if result.OperationID == "" {
		result.OperationID = stringFromAny(meta["operation_id"])
	}
	if result.TargetDID == "" {
		result.TargetDID = request.Target
	}
	return &result, nil
}

func (t *HTTPTransport) SendGroup(ctx context.Context, request SendRequest) (*groupSendResult, error) {
	params, err := BuildGroupSendRPCParams(t.auth.record, nil, request.Group, request.Text, request.MessageType)
	if err != nil {
		return nil, err
	}
	var result groupSendResult
	if err := t.rpcCall(ctx, "group.send", params, &result); err != nil {
		return nil, err
	}
	if result.GroupDID == "" {
		result.GroupDID = request.Group
	}
	return &result, nil
}

func (t *HTTPTransport) GetInbox(ctx context.Context, request InboxRequest) (map[string]any, error) {
	params := map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.inbox.local.v1",
			"security_profile": "transport-protected",
			"sender_did":       t.auth.record.DID,
			"operation_id":     "op-" + generateOperationID(),
			"created_at":       nowRFC3339(),
		},
		"body": map[string]any{
			"user_did": t.auth.record.DID,
			"limit":    request.Limit,
		},
	}
	return t.rpcMapCall(ctx, "inbox.get", params)
}

func (t *HTTPTransport) GetHistory(ctx context.Context, request HistoryRequest) (map[string]any, error) {
	body := map[string]any{
		"user_did": t.auth.record.DID,
		"peer_did": request.With,
		"limit":    request.Limit,
	}
	if strings.TrimSpace(request.Cursor) != "" {
		body["since_seq"] = request.Cursor
	}
	if request.Skip > 0 {
		body["skip"] = request.Skip
	}
	params := map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.direct.local.v1",
			"security_profile": "transport-protected",
			"sender_did":       t.auth.record.DID,
			"operation_id":     "op-" + generateOperationID(),
			"created_at":       nowRFC3339(),
		},
		"body": body,
	}
	return t.rpcMapCall(ctx, "direct.get_history", params)
}

func (t *HTTPTransport) MarkRead(ctx context.Context, request MarkReadRequest) (map[string]any, error) {
	params := map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.inbox.local.v1",
			"security_profile": "transport-protected",
			"sender_did":       t.auth.record.DID,
			"operation_id":     "op-" + generateOperationID(),
			"created_at":       nowRFC3339(),
		},
		"body": map[string]any{
			"user_did":    t.auth.record.DID,
			"message_ids": request.MessageIDs,
		},
	}
	return t.rpcMapCall(ctx, "inbox.mark_read", params)
}

func (t *HTTPTransport) CreateGroup(ctx context.Context, request GroupCreateRequest) (map[string]any, error) {
	serviceDID, err := t.GetMessageServiceDID(ctx)
	if err != nil {
		return nil, err
	}
	params, err := BuildGroupCreateRPCParams(t.auth.record, nil, serviceDID, request)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.create", params)
}

func (t *HTTPTransport) GetGroupInfo(ctx context.Context, request GroupInfoRequest) (map[string]any, error) {
	params, err := BuildGroupGetInfoRPCParams(t.auth.record, request)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.get_info", params)
}

func (t *HTTPTransport) JoinGroup(ctx context.Context, request GroupJoinRequest) (map[string]any, error) {
	params, err := BuildGroupJoinRPCParams(t.auth.record, nil, request)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.join", params)
}

func (t *HTTPTransport) AddGroupMember(ctx context.Context, request GroupMemberRequest) (map[string]any, error) {
	params, err := BuildGroupAddRPCParams(t.auth.record, nil, request)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.add", params)
}

func (t *HTTPTransport) RemoveGroupMember(ctx context.Context, request GroupMemberRequest) (map[string]any, error) {
	params, err := BuildGroupRemoveRPCParams(t.auth.record, nil, request)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.remove", params)
}

func (t *HTTPTransport) LeaveGroup(ctx context.Context, request GroupLeaveRequest) (map[string]any, error) {
	params, err := BuildGroupLeaveRPCParams(t.auth.record, nil, request)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.leave", params)
}

func (t *HTTPTransport) GetGroup(ctx context.Context, request GroupGetRequest) (map[string]any, error) {
	params, err := BuildGroupGetRPCParams(t.auth.record, request)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.get", params)
}

func (t *HTTPTransport) ListGroupMembers(ctx context.Context, request GroupMembersRequest) (map[string]any, error) {
	params, err := BuildGroupMembersRPCParams(t.auth.record, request)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.list_members", params)
}

func (t *HTTPTransport) ListGroupMessages(ctx context.Context, request GroupMessagesRequest) (map[string]any, error) {
	params, err := BuildGroupMessagesRPCParams(t.auth.record, request)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.list_messages", params)
}

func (t *HTTPTransport) UpdateGroupProfile(ctx context.Context, request GroupGetRequest, patch map[string]any) (map[string]any, error) {
	params, err := BuildGroupUpdateProfileRPCParams(t.auth.record, nil, request.Group, patch)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.update_profile", params)
}

func (t *HTTPTransport) UpdateGroupPolicy(ctx context.Context, request GroupGetRequest, patch map[string]any) (map[string]any, error) {
	params, err := BuildGroupUpdatePolicyRPCParams(t.auth.record, nil, request.Group, patch)
	if err != nil {
		return nil, err
	}
	return t.rpcMapCall(ctx, "group.update_policy", params)
}

func (t *HTTPTransport) GetMessageServiceDID(ctx context.Context) (string, error) {
	if t != nil && t.resolved != nil {
		if configured := strings.TrimSpace(t.resolved.ANPServiceDID); configured != "" {
			return configured, nil
		}
	}
	result, err := t.rpcMapCall(ctx, "anp.get_capabilities", map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.core.binding.v1",
			"security_profile": "transport-protected",
			"sender_did":       t.auth.record.DID,
			"operation_id":     "op-" + generateOperationID(),
			"created_at":       nowRFC3339(),
		},
		"body": map[string]any{},
		"client": map[string]any{
			"response_mode": "wait-final",
		},
	})
	if err != nil {
		return "", err
	}
	serviceDID := stringFromAny(result["service_did"])
	if serviceDID == "" {
		return "", fmt.Errorf("message service capabilities response is missing service_did")
	}
	return serviceDID, nil
}

func (t *HTTPTransport) rpcMapCall(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	var result map[string]any
	if err := t.rpcCall(ctx, method, params, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, nil
}

func (t *HTTPTransport) rpcCall(ctx context.Context, method string, params map[string]any, out any) error {
	requestURL := t.rpcEndpointURL
	err := t.auth.session.DoJSONRPC(ctx, t.httpClient, requestURL, http.MethodPost, method, params, out)
	if err == nil {
		t.auth.record.JWTToken = t.auth.session.CurrentJWT()
		return nil
	}
	var rpcErr *authsdk.RPCError
	if errors.As(err, &rpcErr) {
		return &ServiceError{RPCCode: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
	}
	var httpErr *authsdk.HTTPError
	if errors.As(err, &httpErr) {
		return &ServiceError{StatusCode: httpErr.StatusCode, Message: httpErr.Message}
	}
	return err
}
