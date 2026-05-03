package message

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rpcRequestEnvelope struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      string         `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

func decodeRPCRequest(t *testing.T, r *http.Request) rpcRequestEnvelope {
	t.Helper()

	var envelope rpcRequestEnvelope
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return envelope
}

func TestHTTPTransportSendDirectBuildsRPCPayloadAndBackfillsResultFields(t *testing.T) {
	t.Parallel()

	targetDID := "did:wba:awiki.ai:user:bob:e1_bob"
	var captured rpcRequestEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != MessageRPCEndpoint {
			http.NotFound(w, r)
			return
		}
		captured = decodeRPCRequest(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      captured.ID,
			"result": map[string]any{
				"accepted":       true,
				"accepted_at":    "2026-04-18T09:00:00Z",
				"delivery_state": "accepted",
			},
		})
	}))
	defer server.Close()

	transport, resolved, record := newHTTPTransportForTest(t, server.URL)
	transport = transport.WithRPCEndpoint(server.URL + MessageRPCEndpoint)

	result, err := transport.SendDirect(context.Background(), SendRequest{
		Target:      targetDID,
		Text:        "hello over rpc",
		MessageType: "text",
	})
	if err != nil {
		t.Fatalf("SendDirect() error = %v", err)
	}
	if result.TargetDID != targetDID {
		t.Fatalf("result.TargetDID = %q, want %q", result.TargetDID, targetDID)
	}
	if !strings.HasPrefix(result.MessageID, "msg-") {
		t.Fatalf("result.MessageID = %q, want generated msg-* fallback", result.MessageID)
	}
	if !strings.HasPrefix(result.OperationID, "op-") {
		t.Fatalf("result.OperationID = %q, want generated op-* fallback", result.OperationID)
	}

	if captured.Method != "direct.send" {
		t.Fatalf("rpc method = %q, want direct.send", captured.Method)
	}
	meta := mustMapValue(t, captured.Params["meta"], "params.meta")
	if got := stringFromAny(meta["profile"]); got != "anp.direct.base.v1" {
		t.Fatalf("meta.profile = %q, want anp.direct.base.v1", got)
	}
	if got := stringFromAny(meta["sender_did"]); got != record.DID {
		t.Fatalf("meta.sender_did = %q, want %q", got, record.DID)
	}
	if got := stringFromAny(meta["content_type"]); got != "text/plain" {
		t.Fatalf("meta.content_type = %q, want text/plain", got)
	}
	target := mustMapValue(t, meta["target"], "meta.target")
	if got := stringFromAny(target["kind"]); got != "agent" {
		t.Fatalf("meta.target.kind = %q, want agent", got)
	}
	if got := stringFromAny(target["did"]); got != targetDID {
		t.Fatalf("meta.target.did = %q, want %q", got, targetDID)
	}
	body := mustMapValue(t, captured.Params["body"], "params.body")
	if got := stringFromAny(body["text"]); got != "hello over rpc" {
		t.Fatalf("body.text = %q, want hello over rpc", got)
	}
	auth := mustMapValue(t, captured.Params["auth"], "params.auth")
	if got := stringFromAny(auth["scheme"]); got != OriginProofScheme {
		t.Fatalf("auth.scheme = %q, want %q", got, OriginProofScheme)
	}
	if _, ok := auth["origin_proof"]; !ok {
		t.Fatalf("auth.origin_proof missing: %#v", auth)
	}
	if resolved.ServiceBaseURL != server.URL {
		t.Fatalf("resolved.ServiceBaseURL = %q, want %q", resolved.ServiceBaseURL, server.URL)
	}
}

func TestHTTPTransportGetHistoryBuildsLocalRPCBody(t *testing.T) {
	t.Parallel()

	var captured rpcRequestEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = decodeRPCRequest(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      captured.ID,
			"result": map[string]any{
				"messages": []any{},
				"total":    0,
			},
		})
	}))
	defer server.Close()

	transport, _, record := newHTTPTransportForTest(t, server.URL)
	transport = transport.WithRPCEndpoint(server.URL + MessageRPCEndpoint)

	_, err := transport.GetHistory(context.Background(), HistoryRequest{
		With:   "did:wba:awiki.ai:user:bob:e1_bob",
		Limit:  25,
		Cursor: "42",
		Skip:   7,
	})
	if err != nil {
		t.Fatalf("GetHistory() error = %v", err)
	}

	if captured.Method != "direct.get_history" {
		t.Fatalf("rpc method = %q, want direct.get_history", captured.Method)
	}
	meta := mustMapValue(t, captured.Params["meta"], "params.meta")
	if got := stringFromAny(meta["profile"]); got != "anp.direct.local.v1" {
		t.Fatalf("meta.profile = %q, want anp.direct.local.v1", got)
	}
	if got := stringFromAny(meta["sender_did"]); got != record.DID {
		t.Fatalf("meta.sender_did = %q, want %q", got, record.DID)
	}
	body := mustMapValue(t, captured.Params["body"], "params.body")
	if got := stringFromAny(body["user_did"]); got != record.DID {
		t.Fatalf("body.user_did = %q, want %q", got, record.DID)
	}
	if got := stringFromAny(body["peer_did"]); got != "did:wba:awiki.ai:user:bob:e1_bob" {
		t.Fatalf("body.peer_did = %q, want direct peer did", got)
	}
	if got := intValueFromAny(body["limit"], 0); got != 25 {
		t.Fatalf("body.limit = %d, want 25", got)
	}
	if got := stringFromAny(body["since_seq"]); got != "42" {
		t.Fatalf("body.since_seq = %q, want 42", got)
	}
	if got := intValueFromAny(body["skip"], 0); got != 7 {
		t.Fatalf("body.skip = %d, want 7", got)
	}
}

func TestHTTPTransportRPCErrorReturnsServiceError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      "req-1",
			"error": map[string]any{
				"code":    1403,
				"message": "policy denied",
				"data": map[string]any{
					"reason": "group_policy",
				},
			},
		})
	}))
	defer server.Close()

	transport, _, _ := newHTTPTransportForTest(t, server.URL)
	transport = transport.WithRPCEndpoint(server.URL + MessageRPCEndpoint)

	_, err := transport.SendDirect(context.Background(), SendRequest{
		Target: "did:wba:awiki.ai:user:bob:e1_bob",
		Text:   "hello",
	})
	if err == nil {
		t.Fatal("SendDirect() error = nil, want ServiceError")
	}

	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("errors.As(ServiceError) = false, err = %T %v", err, err)
	}
	if serviceErr.RPCCode != 1403 {
		t.Fatalf("serviceErr.RPCCode = %d, want 1403", serviceErr.RPCCode)
	}
	if serviceErr.Message != "policy denied" {
		t.Fatalf("serviceErr.Message = %q, want policy denied", serviceErr.Message)
	}
	data := mustMapValue(t, serviceErr.Data, "serviceErr.Data")
	if got := stringFromAny(data["reason"]); got != "group_policy" {
		t.Fatalf("serviceErr.Data.reason = %q, want group_policy", got)
	}
}

func TestHTTPTransportHTTPErrorReturnsServiceError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	transport, _, _ := newHTTPTransportForTest(t, server.URL)
	transport = transport.WithRPCEndpoint(server.URL + MessageRPCEndpoint)

	_, err := transport.SendDirect(context.Background(), SendRequest{
		Target: "did:wba:awiki.ai:user:bob:e1_bob",
		Text:   "hello",
	})
	if err == nil {
		t.Fatal("SendDirect() error = nil, want ServiceError")
	}

	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("errors.As(ServiceError) = false, err = %T %v", err, err)
	}
	if serviceErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("serviceErr.StatusCode = %d, want %d", serviceErr.StatusCode, http.StatusServiceUnavailable)
	}
	if got := serviceErr.Error(); !strings.Contains(got, "message service http error 503") {
		t.Fatalf("serviceErr.Error() = %q, want http status text", got)
	}
}

func TestHTTPTransportGroupMethodsUseExpectedRPCMethods(t *testing.T) {
	cases := []struct {
		name       string
		call       func(*HTTPTransport) error
		wantMethod string
		verifyBody func(t *testing.T, body map[string]any)
	}{
		{
			name:       "get group info",
			wantMethod: "group.get_info",
			call: func(transport *HTTPTransport) error {
				_, err := transport.GetGroupInfo(context.Background(), GroupInfoRequest{Group: "did:group", IncludePolicy: true, IncludeMemberList: true})
				return err
			},
			verifyBody: func(t *testing.T, body map[string]any) {
				if body["include_policy"] != true || body["include_member_list"] != true {
					t.Fatalf("body = %#v, want include flags", body)
				}
			},
		},
		{
			name:       "join group",
			wantMethod: "group.join",
			call: func(transport *HTTPTransport) error {
				_, err := transport.JoinGroup(context.Background(), GroupJoinRequest{Group: "did:group", ReasonText: "because"})
				return err
			},
			verifyBody: func(t *testing.T, body map[string]any) {
				if body["reason_text"] != "because" {
					t.Fatalf("body = %#v, want reason", body)
				}
			},
		},
		{
			name:       "add member",
			wantMethod: "group.add",
			call: func(transport *HTTPTransport) error {
				_, err := transport.AddGroupMember(context.Background(), GroupMemberRequest{Group: "did:group", Member: "did:member", Role: "admin", ReasonText: "invite"})
				return err
			},
			verifyBody: func(t *testing.T, body map[string]any) {
				if body["member_did"] != "did:member" || body["role"] != "admin" {
					t.Fatalf("body = %#v, want member/role", body)
				}
			},
		},
		{
			name:       "remove member",
			wantMethod: "group.remove",
			call: func(transport *HTTPTransport) error {
				_, err := transport.RemoveGroupMember(context.Background(), GroupMemberRequest{Group: "did:group", Member: "did:member", ReasonText: "cleanup"})
				return err
			},
			verifyBody: func(t *testing.T, body map[string]any) {
				if body["member_did"] != "did:member" || body["reason_text"] != "cleanup" {
					t.Fatalf("body = %#v, want member/reason", body)
				}
			},
		},
		{
			name:       "e2ee remove member",
			wantMethod: "group.e2ee.remove",
			call: func(transport *HTTPTransport) error {
				_, err := transport.RemoveGroupE2EE(context.Background(), "did:group", "did:member", map[string]any{
					"operation_id":         "op-remove",
					"pending_commit_id":    "pc-remove",
					"crypto_group_id_b64u": "Y3J5cHRv",
					"from_epoch":           "1",
					"to_epoch":             "2",
					"commit_b64u":          "Y29tbWl0",
				}, "cleanup")
				return err
			},
			verifyBody: func(t *testing.T, body map[string]any) {
				if body["member_did"] != "did:member" || body["commit_b64u"] != "Y29tbWl0" || body["reason_text"] != "cleanup" {
					t.Fatalf("body = %#v, want member/commit/reason", body)
				}
			},
		},
		{
			name:       "e2ee leave group",
			wantMethod: "group.e2ee.leave",
			call: func(transport *HTTPTransport) error {
				_, err := transport.LeaveGroupE2EE(context.Background(), "did:group", map[string]any{
					"operation_id":         "op-leave",
					"pending_commit_id":    "pc-leave",
					"crypto_group_id_b64u": "Y3J5cHRv",
					"from_epoch":           "2",
					"to_epoch":             "3",
					"commit_b64u":          "Y29tbWl0LWxlYXZl",
				})
				return err
			},
			verifyBody: func(t *testing.T, body map[string]any) {
				if body["group_did"] != "did:group" || body["subject_status"] != "left" || body["commit_b64u"] != "Y29tbWl0LWxlYXZl" {
					t.Fatalf("body = %#v, want group/left/commit", body)
				}
			},
		},
		{
			name:       "list members",
			wantMethod: "group.list_members",
			call: func(transport *HTTPTransport) error {
				_, err := transport.ListGroupMembers(context.Background(), GroupMembersRequest{Group: "did:group", Limit: 7})
				return err
			},
			verifyBody: func(t *testing.T, body map[string]any) {
				if body["group_did"] != "did:group" || intValueFromAny(body["limit"], 0) != 7 {
					t.Fatalf("body = %#v, want group/limit", body)
				}
			},
		},
		{
			name:       "list messages",
			wantMethod: "group.list_messages",
			call: func(transport *HTTPTransport) error {
				_, err := transport.ListGroupMessages(context.Background(), GroupMessagesRequest{Group: "did:group", Limit: 8, Cursor: "3", Skip: 2})
				return err
			},
			verifyBody: func(t *testing.T, body map[string]any) {
				if body["group_did"] != "did:group" || body["since_seq"] != "3" || intValueFromAny(body["skip"], 0) != 2 {
					t.Fatalf("body = %#v, want group/cursor/skip", body)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured rpcRequestEnvelope
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = decodeRPCRequest(t, r)
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": captured.ID, "result": map[string]any{"ok": true}})
			}))
			defer server.Close()

			transport, _, _ := newHTTPTransportForTest(t, server.URL)
			transport = transport.WithRPCEndpoint(server.URL + MessageRPCEndpoint)
			if err := tc.call(transport); err != nil {
				t.Fatalf("transport call error = %v", err)
			}
			if captured.Method != tc.wantMethod {
				t.Fatalf("captured.Method = %q, want %q", captured.Method, tc.wantMethod)
			}
			body := mustMapValue(t, captured.Params["body"], "params.body")
			if tc.verifyBody != nil {
				tc.verifyBody(t, body)
			}
		})
	}
}

func TestHTTPTransportGetMessageServiceDIDUsesConfiguredOrCapabilities(t *testing.T) {
	t.Parallel()

	transport, _, _ := newHTTPTransportForTest(t, "https://awiki.test")
	transport.resolved.ANPServiceDID = "did:wba:configured.example"
	got, err := transport.GetMessageServiceDID(context.Background())
	if err != nil {
		t.Fatalf("GetMessageServiceDID(configured) error = %v", err)
	}
	if got != "did:wba:configured.example" {
		t.Fatalf("GetMessageServiceDID(configured) = %q", got)
	}

	var captured rpcRequestEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = decodeRPCRequest(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": captured.ID, "result": map[string]any{"service_did": "did:wba:capabilities.example"}})
	}))
	defer server.Close()

	transport, _, _ = newHTTPTransportForTest(t, server.URL)
	transport = transport.WithRPCEndpoint(server.URL + MessageRPCEndpoint)
	got, err = transport.GetMessageServiceDID(context.Background())
	if err != nil {
		t.Fatalf("GetMessageServiceDID(capabilities) error = %v", err)
	}
	if got != "did:wba:capabilities.example" {
		t.Fatalf("GetMessageServiceDID(capabilities) = %q", got)
	}
	if captured.Method != "anp.get_capabilities" {
		t.Fatalf("captured.Method = %q, want anp.get_capabilities", captured.Method)
	}
}
