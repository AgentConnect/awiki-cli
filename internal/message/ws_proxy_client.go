package message

import (
	"context"
	"fmt"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/runtime"
)

type WSProxyTransport struct {
	resolved     *appconfig.Resolved
	identityName string
}

func NewWSProxyTransport(resolved *appconfig.Resolved, identityName string) *WSProxyTransport {
	return &WSProxyTransport{resolved: resolved, identityName: identityName}
}

func (t *WSProxyTransport) SendDirect(ctx context.Context, request SendRequest) (*directSendResult, error) {
	result, err := t.call(ctx, "direct.send", map[string]any{
		"target": request.Target,
		"text":   request.Text,
		"type":   request.MessageType,
	})
	if err != nil {
		return nil, err
	}
	sendResult := &directSendResult{}
	decodeMapInto(result, sendResult)
	return sendResult, nil
}

func (t *WSProxyTransport) SendGroup(ctx context.Context, request SendRequest) (*groupSendResult, error) {
	result, err := t.call(ctx, "group.send", map[string]any{
		"group": request.Group,
		"text":  request.Text,
		"type":  request.MessageType,
	})
	if err != nil {
		return nil, err
	}
	sendResult := &groupSendResult{}
	decodeMapInto(result, sendResult)
	return sendResult, nil
}

func (t *WSProxyTransport) GetInbox(ctx context.Context, request InboxRequest) (map[string]any, error) {
	return t.call(ctx, "inbox.get", map[string]any{
		"with":      request.With,
		"limit":     request.Limit,
		"scope":     request.Scope,
		"mark_read": request.MarkRead,
		"unread":    request.UnreadOnly,
	})
}

func (t *WSProxyTransport) GetHistory(ctx context.Context, request HistoryRequest) (map[string]any, error) {
	params := map[string]any{
		"with":   request.With,
		"limit":  request.Limit,
		"cursor": request.Cursor,
	}
	if request.Skip > 0 {
		params["skip"] = request.Skip
	}
	return t.call(ctx, "direct.get_history", params)
}

func (t *WSProxyTransport) MarkRead(ctx context.Context, request MarkReadRequest) (map[string]any, error) {
	return t.call(ctx, "inbox.mark_read", map[string]any{"message_ids": request.MessageIDs})
}

func (t *WSProxyTransport) CreateGroup(ctx context.Context, request GroupCreateRequest) (map[string]any, error) {
	return t.call(ctx, "group.create", map[string]any{
		"name":                   request.Name,
		"description":            request.Description,
		"discoverability":        request.Discoverability,
		"admission_mode":         request.AdmissionMode,
		"slug":                   request.Slug,
		"goal":                   request.Goal,
		"rules":                  request.Rules,
		"message_prompt":         request.MessagePrompt,
		"doc_url":                request.DocURL,
		"attachments_allowed":    request.AttachmentsAllowed,
		"max_members":            request.MaxMembers,
		"member_max_messages":    request.MemberMaxMessages,
		"member_max_total_chars": request.MemberMaxTotalChars,
	})
}

func (t *WSProxyTransport) GetGroupInfo(ctx context.Context, request GroupInfoRequest) (map[string]any, error) {
	return t.call(ctx, "group.get_info", map[string]any{
		"group":               request.Group,
		"include_policy":      request.IncludePolicy,
		"include_member_list": request.IncludeMemberList,
	})
}

func (t *WSProxyTransport) JoinGroup(ctx context.Context, request GroupJoinRequest) (map[string]any, error) {
	return t.call(ctx, "group.join", map[string]any{"group": request.Group, "reason_text": request.ReasonText})
}

func (t *WSProxyTransport) AddGroupMember(ctx context.Context, request GroupMemberRequest) (map[string]any, error) {
	return t.call(ctx, "group.add", map[string]any{
		"group":       request.Group,
		"member":      request.Member,
		"role":        request.Role,
		"reason_text": request.ReasonText,
	})
}

func (t *WSProxyTransport) RemoveGroupMember(ctx context.Context, request GroupMemberRequest) (map[string]any, error) {
	return t.call(ctx, "group.remove", map[string]any{
		"group":       request.Group,
		"member":      request.Member,
		"reason_text": request.ReasonText,
	})
}

func (t *WSProxyTransport) LeaveGroup(ctx context.Context, request GroupLeaveRequest) (map[string]any, error) {
	return t.call(ctx, "group.leave", map[string]any{"group": request.Group})
}

func (t *WSProxyTransport) GetGroup(ctx context.Context, request GroupGetRequest) (map[string]any, error) {
	return t.call(ctx, "group.get", map[string]any{"group": request.Group})
}

func (t *WSProxyTransport) ListGroupMembers(ctx context.Context, request GroupMembersRequest) (map[string]any, error) {
	return t.call(ctx, "group.list_members", map[string]any{"group": request.Group, "limit": request.Limit})
}

func (t *WSProxyTransport) ListGroupMessages(ctx context.Context, request GroupMessagesRequest) (map[string]any, error) {
	params := map[string]any{
		"group":  request.Group,
		"limit":  request.Limit,
		"cursor": request.Cursor,
	}
	if request.Skip > 0 {
		params["skip"] = request.Skip
	}
	return t.call(ctx, "group.list_messages", params)
}

func (t *WSProxyTransport) UpdateGroupProfile(ctx context.Context, request GroupGetRequest, patch map[string]any) (map[string]any, error) {
	return t.call(ctx, "group.update_profile", map[string]any{"group": request.Group, "patch": patch})
}

func (t *WSProxyTransport) UpdateGroupPolicy(ctx context.Context, request GroupGetRequest, patch map[string]any) (map[string]any, error) {
	return t.call(ctx, "group.update_policy", map[string]any{"group": request.Group, "patch": patch})
}

func (t *WSProxyTransport) call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	result, err := runtime.CallLocalBridge(ctx, runtime.BridgeRequest{
		Method:       method,
		Params:       params,
		IdentityName: t.identityName,
	}, t.resolved)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTransportUnavailable, err)
	}
	return result, nil
}
