package message

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func (s *Service) sendSecureDirect(ctx context.Context, request SendRequest) (*CommandResult, error) {
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	if record.E2EEAgreementPrivatePEM == "" || record.Key1PrivatePEM == "" {
		return nil, fmt.Errorf("secure direct messaging requires DID signing and X25519 E2EE private keys")
	}
	targetDID, targetHandle, err := s.resolveTarget(ctx, request.Target)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, s.maybePublishSecurePrekeys(ctx, record)...)
	client, err := s.secureE2EEClient(ctx, record, transport)
	if err != nil {
		return nil, err
	}
	messageID := "msg-" + generateOperationID()
	resultMap, err := client.SendText(ctx, targetDID, request.Text, messageID, messageID)
	if err != nil {
		if isPendingConfirmationError(err) {
			outboxID, queueErr := queueSecureOutboxRecord(ctx, s.resolved, s.manager, record, targetDID, defaultString(request.MessageType, "text"), request.Text)
			if queueErr != nil {
				return nil, queueErr
			}
			return &CommandResult{
				Data: map[string]any{
					"action": "queue_secure_message",
					"target": map[string]any{
						"did":    targetDID,
						"handle": targetHandle,
						"kind":   "direct",
					},
					"message": map[string]any{
						"type":   defaultString(request.MessageType, "text"),
						"secure": true,
						"queued": true,
					},
					"delivery": map[string]any{
						"delivery_state": "queued",
						"outbox_id":      outboxID,
						"target_did":     targetDID,
					},
				},
				Summary:  "Queued secure direct message pending peer confirmation",
				Warnings: warnings,
			}, nil
		}
		return nil, err
	}
	result := directSendResult{
		Accepted:    boolFromAny(resultMap["accepted"]),
		MessageID:   stringFromAny(resultMap["message_id"]),
		OperationID: stringFromAny(resultMap["operation_id"]),
		TargetDID:   stringFromAny(resultMap["target_did"]),
	}
	if result.MessageID == "" {
		result.MessageID = messageID
	}
	if result.OperationID == "" {
		result.OperationID = messageID
	}
	if result.TargetDID == "" {
		result.TargetDID = targetDID
	}
	request.Target = targetDID
	request.SecureMode = "on"
	return s.persistSendResult(ctx, record, targetDID, targetHandle, request, &result, warnings)
}

func (s *Service) maybePublishSecurePrekeys(ctx context.Context, record *identity.StoredIdentity) []string {
	return PublishSecurePrekeys(ctx, s.resolved, s.manager, record)
}

func (s *Service) secureE2EEClient(ctx context.Context, record *identity.StoredIdentity, transport *HTTPTransport) (*anpsdk.MessageServiceE2EEClient, error) {
	return NewSecureE2EEClientForRecord(ctx, s.manager, record, func(method string, params map[string]any) (map[string]any, error) {
		return transport.rpcMapCall(ctx, method, params)
	})
}

// PublishSecurePrekeys ensures one identity has a published secure prekey bundle.
func PublishSecurePrekeys(ctx context.Context, resolved *appconfig.Resolved, manager *identity.Manager, record *identity.StoredIdentity) []string {
	if record == nil || record.E2EEAgreementPrivatePEM == "" || record.Key1PrivatePEM == "" {
		return nil
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return compactWarnings([]string{fmt.Sprintf("Failed to initialize secure prekey auth: %v", err)})
	}
	primeAuthSession(auth, resolved)
	transport := NewHTTPTransport(resolved, auth, nil)
	client, err := NewSecureE2EEClientForRecord(ctx, manager, record, func(method string, params map[string]any) (map[string]any, error) {
		return transport.rpcMapCall(ctx, method, params)
	})
	if err != nil {
		return compactWarnings([]string{fmt.Sprintf("Failed to initialize secure prekey publisher: %v", err)})
	}
	if _, err := client.PublishPrekeyBundle(); err != nil {
		return compactWarnings([]string{fmt.Sprintf("Failed to publish secure prekeys: %v", err)})
	}
	return nil
}

// NewSecureE2EEClientForRecord creates one direct-e2ee client for the given identity.
func NewSecureE2EEClientForRecord(ctx context.Context, manager *identity.Manager, record *identity.StoredIdentity, rpc func(string, map[string]any) (map[string]any, error)) (*anpsdk.MessageServiceE2EEClient, error) {
	_ = ctx
	if manager == nil {
		return nil, fmt.Errorf("identity manager is required")
	}
	if record == nil {
		return nil, fmt.Errorf("identity record is required")
	}
	paths, err := manager.PathsForIdentity(record.IdentityName)
	if err != nil {
		return nil, err
	}
	signingPrivate, err := anpsdk.PrivateKeyFromPEM(record.Key1PrivatePEM)
	if err != nil {
		return nil, fmt.Errorf("parse DID signing private key: %w", err)
	}
	agreementPrivate, err := anpsdk.PrivateKeyFromPEM(record.E2EEAgreementPrivatePEM)
	if err != nil {
		return nil, fmt.Errorf("parse E2EE agreement private key: %w", err)
	}
	sessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		return nil, err
	}
	signedPrekeyStore, err := anpsdk.NewFileSignedPrekeyStore(filepath.Join(paths.IdentityDir, "p5-signed-prekeys"))
	if err != nil {
		return nil, err
	}
	oneTimePrekeyStore, err := anpsdk.NewFileOneTimePrekeyStore(filepath.Join(paths.IdentityDir, "p5-one-time-prekeys"))
	if err != nil {
		return nil, err
	}
	resolver := func(ctx context.Context, did string) (map[string]any, error) {
		if did == record.DID && record.DIDDocument != nil {
			return record.DIDDocument, nil
		}
		if localDocument, ok := loadLocalDIDDocument(manager, did); ok {
			return localDocument, nil
		}
		return anpsdk.ResolveDidDocument(ctx, did, true)
	}
	return anpsdk.NewMessageServiceDirectE2eeClient(
		record.DID,
		signingPrivate,
		record.DID+"#key-1",
		agreementPrivate,
		record.DID+"#key-3",
		rpc,
		resolver,
		sessionStore,
		signedPrekeyStore,
		oneTimePrekeyStore,
	)
}

func loadLocalDIDDocument(manager *identity.Manager, did string) (map[string]any, bool) {
	if manager == nil || did == "" {
		return nil, false
	}
	summaries, err := manager.List()
	if err != nil {
		return nil, false
	}
	for _, summary := range summaries {
		if summary.DID != did {
			continue
		}
		record, err := manager.Load(summary.IdentityName)
		if err != nil || record == nil || record.DIDDocument == nil {
			return nil, false
		}
		return record.DIDDocument, true
	}
	return nil, false
}
