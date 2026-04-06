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
	result, err := t.call("direct.send", map[string]any{
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

func (t *WSProxyTransport) GetInbox(ctx context.Context, request InboxRequest) (map[string]any, error) {
	return t.call("inbox.get", map[string]any{
		"with":      request.With,
		"limit":     request.Limit,
		"scope":     request.Scope,
		"mark_read": request.MarkRead,
		"unread":    request.UnreadOnly,
	})
}

func (t *WSProxyTransport) GetHistory(ctx context.Context, request HistoryRequest) (map[string]any, error) {
	return t.call("direct.get_history", map[string]any{
		"with":   request.With,
		"limit":  request.Limit,
		"cursor": request.Cursor,
	})
}

func (t *WSProxyTransport) MarkRead(ctx context.Context, request MarkReadRequest) (map[string]any, error) {
	return t.call("inbox.mark_read", map[string]any{"message_ids": request.MessageIDs})
}

func (t *WSProxyTransport) call(method string, params map[string]any) (map[string]any, error) {
	result, err := runtime.CallLocalBridge(runtime.BridgeRequest{
		Method:       method,
		Params:       params,
		IdentityName: t.identityName,
	}, t.resolved)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTransportUnavailable, err)
	}
	return result, nil
}
