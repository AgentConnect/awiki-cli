package message

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/runtime"
)

func TestWSProxyTransportCallsLocalBridgeAndDecodesResponses(t *testing.T) {
	cases := []struct {
		name       string
		call       func(*testing.T, *WSProxyTransport) error
		wantMethod string
		verify     func(t *testing.T, params map[string]any)
	}{
		{
			name:       "send direct",
			wantMethod: "direct.send",
			call: func(t *testing.T, transport *WSProxyTransport) error {
				t.Helper()
				result, err := transport.SendDirect(context.Background(), SendRequest{Target: "did:bob", Text: "hello", MessageType: "text"})
				if err != nil {
					return err
				}
				if result.MessageID != "msg-1" || result.OperationID != "op-1" {
					t.Fatalf("SendDirect() result = %#v, want decoded response", result)
				}
				return nil
			},
			verify: func(t *testing.T, params map[string]any) {
				if params["target"] != "did:bob" || params["text"] != "hello" {
					t.Fatalf("params = %#v, want target/text", params)
				}
			},
		},
		{
			name:       "get history preserves skip",
			wantMethod: "direct.get_history",
			call: func(t *testing.T, transport *WSProxyTransport) error {
				t.Helper()
				_, err := transport.GetHistory(context.Background(), HistoryRequest{With: "bob", Limit: 5, Cursor: "seq-2", Skip: 3})
				return err
			},
			verify: func(t *testing.T, params map[string]any) {
				if params["with"] != "bob" || params["cursor"] != "seq-2" || params["skip"] != float64(3) {
					t.Fatalf("params = %#v, want with/cursor/skip", params)
				}
			},
		},
		{
			name:       "list group messages preserves cursor and skip",
			wantMethod: "group.list_messages",
			call: func(t *testing.T, transport *WSProxyTransport) error {
				t.Helper()
				_, err := transport.ListGroupMessages(context.Background(), GroupMessagesRequest{Group: "did:group", Limit: 10, Cursor: "7", Skip: 2})
				return err
			},
			verify: func(t *testing.T, params map[string]any) {
				if params["group"] != "did:group" || params["cursor"] != "7" || params["skip"] != float64(2) {
					t.Fatalf("params = %#v, want group/cursor/skip", params)
				}
			},
		},
		{
			name:       "list groups preserves limit",
			wantMethod: "group.list",
			call: func(t *testing.T, transport *WSProxyTransport) error {
				t.Helper()
				_, err := transport.ListGroups(context.Background(), GroupListRequest{Limit: 12})
				return err
			},
			verify: func(t *testing.T, params map[string]any) {
				if params["limit"] != float64(12) {
					t.Fatalf("params = %#v, want limit", params)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved := testResolvedConfig(t)
			resolved.RuntimeMode = runtime.ModeWebSocket
			bridgePath := runtime.Resolve(resolved).SocketPath
			listener, err := runtime.ListenBridge(bridgePath)
			if err != nil {
				t.Fatalf("ListenBridge() error = %v", err)
			}
			defer listener.Close()

			requests := make(chan runtime.BridgeRequest, 1)
			go serveBridgeOnce(t, listener, requests, runtime.BridgeResponse{OK: true, Result: map[string]any{
				"message_id":   "msg-1",
				"operation_id": "op-1",
			}})

			transport := NewWSProxyTransport(resolved, "alice")
			if err := tc.call(t, transport); err != nil {
				t.Fatalf("transport call error = %v", err)
			}

			request := <-requests
			if request.Method != tc.wantMethod {
				t.Fatalf("request.Method = %q, want %q", request.Method, tc.wantMethod)
			}
			if request.IdentityName != "alice" {
				t.Fatalf("request.IdentityName = %q, want alice", request.IdentityName)
			}
			if tc.verify != nil {
				tc.verify(t, request.Params)
			}
		})
	}
}

func TestWSProxyTransportWrapsBridgeFailures(t *testing.T) {
	t.Parallel()

	resolved := &appconfig.Resolved{RuntimeMode: runtime.ModeWebSocket, Paths: appconfig.Paths{WorkspaceHomeDir: t.TempDir(), StateDir: t.TempDir()}}
	transport := NewWSProxyTransport(resolved, "alice")
	_, err := transport.MarkRead(context.Background(), MarkReadRequest{MessageIDs: []string{"msg-1"}})
	if err == nil {
		t.Fatal("MarkRead() error = nil, want transport unavailable")
	}
	if !strings.Contains(err.Error(), ErrTransportUnavailable.Error()) {
		t.Fatalf("MarkRead() error = %v, want ErrTransportUnavailable wrapper", err)
	}
}

func serveBridgeOnce(t *testing.T, listener net.Listener, requests chan<- runtime.BridgeRequest, response runtime.BridgeResponse) {
	t.Helper()
	for {
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("listener.Accept() error = %v", err)
			return
		}

		handled, retry := func() (bool, bool) {
			defer conn.Close()

			var request runtime.BridgeRequest
			if err := json.NewDecoder(conn).Decode(&request); err != nil {
				if err == io.EOF {
					// Health probes connect and close without sending a request.
					return false, true
				}
				t.Errorf("Decode() error = %v", err)
				return false, false
			}
			requests <- request
			if err := json.NewEncoder(conn).Encode(response); err != nil {
				t.Errorf("Encode() error = %v", err)
			}
			return true, false
		}()
		if handled || !retry {
			return
		}
	}
}
