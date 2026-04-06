package message

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

type Transport interface {
	SendDirect(context.Context, SendRequest) (*directSendResult, error)
	GetInbox(context.Context, InboxRequest) (map[string]any, error)
	GetHistory(context.Context, HistoryRequest) (map[string]any, error)
	MarkRead(context.Context, MarkReadRequest) (map[string]any, error)
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
	baseMessageURL string
}

func NewHTTPTransport(resolved *appconfig.Resolved, auth *authContext, httpClient *http.Client) *HTTPTransport {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &HTTPTransport{
		resolved:       resolved,
		auth:           auth,
		httpClient:     httpClient,
		baseMessageURL: strings.TrimRight(resolved.MessageServiceURL, "/"),
	}
}

func (t *HTTPTransport) SendDirect(ctx context.Context, request SendRequest) (*directSendResult, error) {
	payload, err := buildDirectTextPayload(t.auth.record.DID, request.Target, request.Text, contentTypeForMessageType(request.MessageType))
	if err != nil {
		return nil, err
	}
	senderProof, err := buildSenderProof(t.auth, payload, request.Target)
	if err != nil {
		return nil, err
	}
	params := map[string]any{
		"meta": payload.Meta,
		"auth": map[string]any{
			"scheme":       "anp-rfc9421-sender-proof-v1",
			"sender_proof": senderProof,
		},
		"body": payload.Body,
	}
	var result directSendResult
	if err := t.rpcCall(ctx, payload.Method, params, &result); err != nil {
		return nil, err
	}
	if result.MessageID == "" {
		result.MessageID = stringFromAny(payload.Meta["message_id"])
	}
	if result.OperationID == "" {
		result.OperationID = stringFromAny(payload.Meta["operation_id"])
	}
	if result.TargetDID == "" {
		result.TargetDID = request.Target
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
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      generateOperationID(),
		"method":  method,
		"params":  params,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	requestURL := t.baseMessageURL + MessageRPCEndpoint
	headers, err := t.auth.authHeaders(requestURL, http.MethodPost, body, false)
	if err != nil {
		return err
	}
	headers["Content-Type"] = "application/json"
	response, err := t.doPost(ctx, requestURL, body, headers)
	if err != nil && t.auth.record.JWTToken != "" {
		var firstErr *ServiceError
		if errors.As(err, &firstErr) && firstErr.StatusCode == http.StatusUnauthorized {
			headers, headerErr := t.auth.authHeaders(requestURL, http.MethodPost, body, true)
			if headerErr != nil {
				return err
			}
			headers["Content-Type"] = "application/json"
			response, err = t.doPost(ctx, requestURL, body, headers)
		}
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	t.auth.captureResponseToken(response)
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	var rpcResult rpcResponse
	if err := json.Unmarshal(raw, &rpcResult); err != nil {
		return fmt.Errorf("parse message rpc response: %w", err)
	}
	if rpcResult.Error != nil {
		return &ServiceError{RPCCode: rpcResult.Error.Code, Message: rpcResult.Error.Message, Data: rpcResult.Error.Data}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(rpcResult.Result, out)
}

func (t *HTTPTransport) doPost(ctx context.Context, requestURL string, body []byte, headers map[string]string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := t.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 400 {
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		return nil, &ServiceError{StatusCode: response.StatusCode, Message: strings.TrimSpace(string(raw))}
	}
	return response, nil
}
