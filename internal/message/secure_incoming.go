package message

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func isDirectE2EEWireContentType(contentType string) bool {
	switch contentType {
	case "application/anp-direct-init+json", "application/anp-direct-cipher+json":
		return true
	default:
		return false
	}
}

func (s *Service) maybeDecryptDirectE2EEMessages(ctx context.Context, record *identity.StoredIdentity, messages []map[string]any) []string {
	if len(messages) == 0 || !containsDirectE2EEMessages(messages) {
		return nil
	}
	transport, _, err := s.httpTransport(record)
	if err != nil {
		return compactWarnings([]string{fmt.Sprintf("Failed to initialize secure direct decryptor: %v", err)})
	}
	client, err := s.secureE2EEClient(ctx, record, transport)
	if err != nil {
		return compactWarnings([]string{fmt.Sprintf("Failed to initialize secure direct decryptor: %v", err)})
	}
	ordered := append([]map[string]any(nil), messages...)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftSeq := int64Value(ordered[i]["server_seq"])
		rightSeq := int64Value(ordered[j]["server_seq"])
		if leftSeq == rightSeq {
			return stringFromAny(ordered[i]["id"]) < stringFromAny(ordered[j]["id"])
		}
		if leftSeq == 0 {
			return false
		}
		if rightSeq == 0 {
			return true
		}
		return leftSeq < rightSeq
	})
	warnings := make([]string, 0)
	for _, message := range ordered {
		contentType := stringFromAny(message["content_type"])
		if !isDirectE2EEWireContentType(contentType) {
			continue
		}
		notification, err := directE2EENotificationFromMessageView(message)
		if err != nil {
			if !isDirectE2EEWireControlMessage(message) {
				warnings = append(warnings, fmt.Sprintf("Skipped secure direct message %s: %v", stringFromAny(message["id"]), err))
			}
			continue
		}
		result, err := client.ProcessIncoming(ctx, notification)
		if err != nil {
			if !isDirectE2EEWireControlMessage(message) {
				warnings = append(warnings, fmt.Sprintf("Failed to decrypt secure direct message %s: %v", stringFromAny(message["id"]), err))
			}
			continue
		}
		warnings = append(warnings, s.maybeFlushPollingSecureAck(ctx, record, transport, message, result)...)
		warnings = append(warnings, s.maybeAckPollingDirectInit(ctx, record, transport, client, message, result)...)
		applyDirectE2EEProcessingResult(message, result)
	}
	return compactWarnings(warnings)
}

func (s *Service) maybeFlushPollingSecureAck(ctx context.Context, record *identity.StoredIdentity, transport *HTTPTransport, message map[string]any, result map[string]any) []string {
	if record == nil || transport == nil {
		return nil
	}
	if stringFromAny(result["state"]) != "decrypted" {
		return nil
	}
	plaintext, err := mapFromAny(result["plaintext"])
	if err != nil || !IsSecureAckPlaintext(plaintext) {
		return nil
	}
	peerDID := stringFromAny(message["sender_did"])
	if peerDID == "" || peerDID == record.DID {
		return nil
	}
	return FlushQueuedSecureOutbox(ctx, s.resolved, s.manager, record, peerDID, func(method string, params map[string]any) (map[string]any, error) {
		return transport.rpcMapCall(ctx, method, params)
	})
}

func (s *Service) maybeAckPollingDirectInit(ctx context.Context, record *identity.StoredIdentity, transport *HTTPTransport, client *anpsdk.MessageServiceE2EEClient, message map[string]any, result map[string]any) []string {
	if record == nil || transport == nil || client == nil {
		return nil
	}
	if stringFromAny(message["content_type"]) != "application/anp-direct-init+json" {
		return nil
	}
	if stringFromAny(result["state"]) != "decrypted" {
		return nil
	}
	sessionID := directInitSessionIDFromMessage(message)
	messageID := stringFromAny(message["id"])
	peerDID := stringFromAny(message["sender_did"])
	if sessionID == "" || messageID == "" || peerDID == "" || peerDID == record.DID {
		return nil
	}
	ackID := "ack-" + sessionID
	if _, err := client.SendJSON(ctx, peerDID, BuildSecureAckPayload(sessionID, messageID), ackID, ackID); err != nil {
		return []string{fmt.Sprintf("Failed to send secure direct ACK for %s: %v", messageID, err)}
	}
	return FlushQueuedSecureOutbox(ctx, s.resolved, s.manager, record, peerDID, func(method string, params map[string]any) (map[string]any, error) {
		return transport.rpcMapCall(ctx, method, params)
	})
}

func directInitSessionIDFromMessage(message map[string]any) string {
	body, err := mapFromAny(message["content"])
	if err != nil {
		return ""
	}
	return stringFromAny(body["session_id"])
}

func containsDirectE2EEMessages(messages []map[string]any) bool {
	for _, message := range messages {
		if isDirectE2EEWireContentType(stringFromAny(message["content_type"])) {
			return true
		}
	}
	return false
}

func directE2EENotificationFromMessageView(message map[string]any) (map[string]any, error) {
	body, err := mapFromAny(message["content"])
	if err != nil {
		return nil, fmt.Errorf("content is not a direct-e2ee object")
	}
	senderDID := stringFromAny(message["sender_did"])
	receiverDID := stringFromAny(message["receiver_did"])
	messageID := stringFromAny(message["id"])
	if senderDID == "" || receiverDID == "" || messageID == "" {
		return nil, fmt.Errorf("missing sender_did/receiver_did/id")
	}
	result := map[string]any{
		"meta": map[string]any{
			"sender_did":       senderDID,
			"target":           map[string]any{"kind": "agent", "did": receiverDID},
			"message_id":       messageID,
			"profile":          "anp.direct.e2ee.v1",
			"security_profile": "direct-e2ee",
			"content_type":     stringFromAny(message["content_type"]),
		},
		"body": body,
	}
	if serverSeq := int64Value(message["server_seq"]); serverSeq != 0 {
		result["server_seq"] = serverSeq
	}
	return result, nil
}

func applyDirectE2EEProcessingResult(message map[string]any, result map[string]any) {
	message["secure"] = true
	state := stringFromAny(result["state"])
	if state == "" {
		return
	}
	message["decryption_state"] = state
	if state != "decrypted" {
		return
	}
	plaintext, err := mapFromAny(result["plaintext"])
	if err != nil {
		return
	}
	if IsSecureAckPlaintext(plaintext) || IsSecureInitPlaintext(plaintext) {
		message["secure_control"] = true
		message["type"] = "secure_control"
		message["content"] = ""
		return
	}
	contentType := stringFromAny(plaintext["application_content_type"])
	if contentType != "" {
		message["content_type"] = contentType
	}
	switch {
	case stringFromAny(plaintext["text"]) != "":
		message["content"] = stringFromAny(plaintext["text"])
		message["type"] = "text"
	case plaintext["payload"] != nil:
		message["content"] = plaintext["payload"]
		if contentType == attachmentManifestContentType {
			message["type"] = "attachment_manifest"
		} else {
			message["type"] = "json"
		}
	case stringFromAny(plaintext["payload_b64u"]) != "":
		message["content"] = stringFromAny(plaintext["payload_b64u"])
		message["type"] = "binary"
	}
}

func isDirectE2EEControlOrUndisplayable(message map[string]any) bool {
	if boolFromAny(message["secure_control"]) {
		return true
	}
	if !isDirectE2EEWireContentType(stringFromAny(message["content_type"])) {
		return false
	}
	state := stringFromAny(message["decryption_state"])
	return state == "undecryptable" || state == "failed" || state == ""
}

func isDirectE2EEWireControlMessage(message map[string]any) bool {
	switch stringFromAny(message["content_type"]) {
	case "application/anp-direct-init+json":
		return true
	}
	id := stringFromAny(message["id"])
	if id == "" {
		id = stringFromAny(message["msg_id"])
	}
	return strings.HasPrefix(id, "secure-init-") || strings.HasPrefix(id, "ack-")
}

func int64Value(value any) int64 {
	if parsed := int64PtrFromAny(value); parsed != nil {
		return *parsed
	}
	return 0
}
