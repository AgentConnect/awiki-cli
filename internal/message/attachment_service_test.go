package message

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/testenv"
)

func writeTestAttachmentFile(t *testing.T, name string, payload []byte) string {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filePath, payload, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return filePath
}

func TestServiceSendDirectAttachmentPersistsManifestAndDelivery(t *testing.T) {
	t.Parallel()

	attachmentPayload := []byte("attachment body")
	filePath := writeTestAttachmentFile(t, "report.txt", attachmentPayload)
	targetDID := "did:wba:awiki.ai:user:bob:e1_bob"
	serviceDID := "did:wba:awiki.ai:services:message:e1_service"
	var uploadBody []byte

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case MessageRPCEndpoint:
			envelope := decodeRPCRequest(t, r)
			switch envelope.Method {
			case "attachment.create_slot":
				meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
				target := mustMapValue(t, meta["target"], "meta.target")
				if got := stringFromAny(target["did"]); got != serviceDID {
					t.Fatalf("meta.target.did = %q, want %q", got, serviceDID)
				}
				body := mustMapValue(t, envelope.Params["body"], "params.body")
				intendedTarget := mustMapValue(t, body["intended_target"], "body.intended_target")
				if got := stringFromAny(intendedTarget["kind"]); got != "agent" {
					t.Fatalf("body.intended_target.kind = %q, want agent", got)
				}
				if got := stringFromAny(intendedTarget["did"]); got != targetDID {
					t.Fatalf("body.intended_target.did = %q, want %q", got, targetDID)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      envelope.ID,
					"result": map[string]any{
						"attachment_id": "att-1",
						"slot_id":       "slot-1",
						"upload_uri":    server.URL + "/upload/object-1",
						"upload_headers": map[string]any{
							"X-Test-Upload": "slot-1",
						},
						"object_uri":   testenv.SubdomainURL("objects") + "/object-1",
						"commit_token": "commit-1",
						"expires_at":   "2026-04-18T10:00:00Z",
					},
				})
			case "attachment.commit_object":
				body := mustMapValue(t, envelope.Params["body"], "params.body")
				if got := stringFromAny(body["attachment_id"]); got != "att-1" {
					t.Fatalf("body.attachment_id = %q, want att-1", got)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      envelope.ID,
					"result": map[string]any{
						"committed":     true,
						"attachment_id": "att-1",
						"object_uri":    testenv.SubdomainURL("objects") + "/object-1",
						"committed_at":  "2026-04-18T09:01:00Z",
					},
				})
			case "direct.send":
				meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
				if got := stringFromAny(meta["content_type"]); got != attachmentManifestContentType {
					t.Fatalf("meta.content_type = %q, want %q", got, attachmentManifestContentType)
				}
				target := mustMapValue(t, meta["target"], "meta.target")
				if got := stringFromAny(target["did"]); got != targetDID {
					t.Fatalf("meta.target.did = %q, want %q", got, targetDID)
				}
				body := mustMapValue(t, envelope.Params["body"], "params.body")
				payload := mustMapValue(t, body["payload"], "body.payload")
				if got := stringFromAny(payload["caption"]); got != "Quarterly report" {
					t.Fatalf("payload.caption = %q, want Quarterly report", got)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      envelope.ID,
					"result": map[string]any{
						"accepted":         true,
						"message_id":       "msg-direct-1",
						"operation_id":     "op-direct-1",
						"target_did":       targetDID,
						"accepted_at":      "2026-04-18T09:02:00Z",
						"final_acceptance": true,
						"delivery_state":   "accepted",
					},
				})
			default:
				t.Fatalf("unexpected rpc method %q", envelope.Method)
			}
		case "/upload/object-1":
			if r.Method != http.MethodPut {
				t.Fatalf("upload method = %q, want PUT", r.Method)
			}
			var err error
			uploadBody, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll(upload body) error = %v", err)
			}
			if got := r.Header.Get("X-Test-Upload"); got != "slot-1" {
				t.Fatalf("upload X-Test-Upload = %q, want slot-1", got)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service, resolved, record := newMessageServiceForTest(t, server.URL)
	resolved.ANPServiceDID = serviceDID
	resolved.RuntimeMode = "http"

	result, err := service.Send(context.Background(), SendRequest{
		IdentityName: "alice",
		Target:       targetDID,
		Text:         "Quarterly report",
		FilePath:     filePath,
		MIMEType:     "text/plain",
	})
	if err != nil {
		t.Fatalf("service.Send() error = %v", err)
	}
	if string(uploadBody) != string(attachmentPayload) {
		t.Fatalf("uploaded payload = %q, want %q", string(uploadBody), string(attachmentPayload))
	}
	if result.Summary != "Sent a direct attachment message" {
		t.Fatalf("result.Summary = %q, want direct attachment summary", result.Summary)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("result.Warnings = %#v, want no warnings", result.Warnings)
	}
	attachment := mustMapValue(t, result.Data["attachment"], "result.Data.attachment")
	if got := stringFromAny(attachment["attachment_id"]); got != "att-1" {
		t.Fatalf("attachment.attachment_id = %q, want att-1", got)
	}
	if got := stringFromAny(attachment["filename"]); got != "report.txt" {
		t.Fatalf("attachment.filename = %q, want report.txt", got)
	}
	message := mustMapValue(t, result.Data["message"], "result.Data.message")
	if got := stringFromAny(message["id"]); got != "msg-direct-1" {
		t.Fatalf("message.id = %q, want msg-direct-1", got)
	}
	if got := stringFromAny(message["content_type"]); got != attachmentManifestContentType {
		t.Fatalf("message.content_type = %q, want %q", got, attachmentManifestContentType)
	}

	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	row, err := store.GetMessageByID(context.Background(), db, "msg-direct-1", record.DID, record.IdentityName)
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got := stringFromAny(row["content_type"]); got != attachmentManifestContentType {
		t.Fatalf("stored content_type = %q, want %q", got, attachmentManifestContentType)
	}
	if got := stringFromAny(row["receiver_did"]); got != targetDID {
		t.Fatalf("stored receiver_did = %q, want %q", got, targetDID)
	}
	content := mustMapValue(t, mustDecodeJSONMap(t, stringFromAny(row["content"])), "stored content")
	if got := stringFromAny(content["caption"]); got != "Quarterly report" {
		t.Fatalf("stored caption = %q, want Quarterly report", got)
	}
}

func TestServiceSendDirectAttachmentUploadFailureReturnsServiceError(t *testing.T) {
	t.Parallel()

	filePath := writeTestAttachmentFile(t, "broken.txt", []byte("attachment body"))
	serviceDID := "did:wba:awiki.ai:services:message:e1_service"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case MessageRPCEndpoint:
			envelope := decodeRPCRequest(t, r)
			if envelope.Method != "attachment.create_slot" {
				t.Fatalf("unexpected rpc method %q", envelope.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"attachment_id": "att-2",
					"slot_id":       "slot-2",
					"upload_uri":    server.URL + "/upload/object-2",
					"object_uri":    testenv.SubdomainURL("objects") + "/object-2",
					"commit_token":  "commit-2",
				},
			})
		case "/upload/object-2":
			http.Error(w, "upload gateway failed", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service, resolved, _ := newMessageServiceForTest(t, server.URL)
	resolved.ANPServiceDID = serviceDID
	resolved.RuntimeMode = "http"

	_, err := service.Send(context.Background(), SendRequest{
		IdentityName: "alice",
		Target:       "did:wba:awiki.ai:user:bob:e1_bob",
		Text:         "Broken upload",
		FilePath:     filePath,
		MIMEType:     "text/plain",
	})
	if err == nil {
		t.Fatal("service.Send() error = nil, want ServiceError")
	}

	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("errors.As(ServiceError) = false, err = %T %v", err, err)
	}
	if serviceErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("serviceErr.StatusCode = %d, want %d", serviceErr.StatusCode, http.StatusBadGateway)
	}
	if serviceErr.Message != "upload gateway failed" {
		t.Fatalf("serviceErr.Message = %q, want upload gateway failed", serviceErr.Message)
	}
}

func TestServiceSendGroupAttachmentAddsHTTPWarningAndBackfillsMessageID(t *testing.T) {
	t.Parallel()

	filePath := writeTestAttachmentFile(t, "group.txt", []byte("group attachment"))
	groupDID := "did:wba:awiki.ai:groups:demo:e1_group"
	serviceDID := "did:wba:awiki.ai:services:message:e1_service"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case MessageRPCEndpoint:
			envelope := decodeRPCRequest(t, r)
			switch envelope.Method {
			case "attachment.create_slot":
				body := mustMapValue(t, envelope.Params["body"], "params.body")
				intendedTarget := mustMapValue(t, body["intended_target"], "body.intended_target")
				if got := stringFromAny(intendedTarget["kind"]); got != "group" {
					t.Fatalf("body.intended_target.kind = %q, want group", got)
				}
				if got := stringFromAny(intendedTarget["did"]); got != groupDID {
					t.Fatalf("body.intended_target.did = %q, want %q", got, groupDID)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      envelope.ID,
					"result": map[string]any{
						"attachment_id": "att-group-1",
						"slot_id":       "slot-group-1",
						"upload_uri":    server.URL + "/upload/group-1",
						"object_uri":    testenv.SubdomainURL("objects") + "/group-1",
						"commit_token":  "commit-group-1",
					},
				})
			case "attachment.commit_object":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      envelope.ID,
					"result": map[string]any{
						"committed": true,
					},
				})
			case "group.send":
				meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
				target := mustMapValue(t, meta["target"], "meta.target")
				if got := stringFromAny(target["did"]); got != groupDID {
					t.Fatalf("meta.target.did = %q, want %q", got, groupDID)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      envelope.ID,
					"result": map[string]any{
						"accepted":            true,
						"group_did":           groupDID,
						"group_event_seq":     "42",
						"group_state_version": "7",
						"accepted_at":         "2026-04-18T09:03:00Z",
					},
				})
			default:
				t.Fatalf("unexpected rpc method %q", envelope.Method)
			}
		case "/upload/group-1":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service, resolved, record := newMessageServiceForTest(t, server.URL)
	resolved.ANPServiceDID = serviceDID
	resolved.RuntimeMode = "websocket"

	result, err := service.Send(context.Background(), SendRequest{
		IdentityName: "alice",
		Group:        groupDID,
		Text:         "Shared in group",
		FilePath:     filePath,
		MIMEType:     "text/plain",
	})
	if err != nil {
		t.Fatalf("service.Send() error = %v", err)
	}
	if !warningContains(result.Warnings, "Attachment messages use HTTP transport even when runtime.mode is websocket.") {
		t.Fatalf("result.Warnings = %#v, want websocket attachment warning", result.Warnings)
	}
	message := mustMapValue(t, result.Data["message"], "result.Data.message")
	if got := stringFromAny(message["id"]); got != groupDID+":42" {
		t.Fatalf("message.id = %q, want %q", got, groupDID+":42")
	}
	target := mustMapValue(t, result.Data["target"], "result.Data.target")
	if got := stringFromAny(target["kind"]); got != "group" {
		t.Fatalf("target.kind = %q, want group", got)
	}

	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	row, err := store.GetMessageByID(context.Background(), db, groupDID+":42", record.DID, record.IdentityName)
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got := stringFromAny(row["group_did"]); got != groupDID {
		t.Fatalf("stored group_did = %q, want %q", got, groupDID)
	}
}

func mustDecodeJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return decoded
}
