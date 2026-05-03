package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
)

func (s *Service) publishGroupE2EEKeyPackage(ctx context.Context, record *identity.StoredIdentity, deviceID string, contractTest bool) (map[string]any, map[string]any, error) {
	provider := s.groupMLSProvider()
	if strings.TrimSpace(deviceID) == "" {
		deviceID = "default"
	}
	packageResult, err := provider.GenerateKeyPackage(ctx, MLSRequest{
		APIVersion:          "anp-mls/v1",
		RequestID:           "group-e2ee-key-package-" + generateOperationID(),
		AgentDID:            record.DID,
		DeviceID:            deviceID,
		ContractTestEnabled: contractTest,
		Params: map[string]any{
			"agent_did": record.DID,
			"device_id": deviceID,
			"owner_did": record.DID,
		},
	})
	if err != nil {
		return nil, nil, err
	}
	packageResult, err = signGroupKeyPackageDIDWBABinding(record, packageResult)
	if err != nil {
		return nil, nil, err
	}
	transport, _, err := s.httpTransport(record)
	if err != nil {
		return packageResult, nil, err
	}
	published, err := transport.PublishGroupE2EEKeyPackage(ctx, packageResult)
	if err != nil {
		return packageResult, nil, err
	}
	return packageResult, published, nil
}

func signGroupKeyPackageDIDWBABinding(record *identity.StoredIdentity, packageResult map[string]any) (map[string]any, error) {
	if record == nil {
		return nil, fmt.Errorf("identity record is required")
	}
	if len(packageResult) == 0 {
		return nil, fmt.Errorf("anp-mls key-package response is empty")
	}
	groupKeyPackage, ok := packageResult["group_key_package"].(map[string]any)
	if !ok || len(groupKeyPackage) == 0 {
		return nil, fmt.Errorf("anp-mls key-package response missing group_key_package")
	}
	binding, ok := groupKeyPackage["did_wba_binding"].(map[string]any)
	if !ok || len(binding) == 0 {
		return nil, fmt.Errorf("group_key_package.did_wba_binding is required")
	}
	ownerDID := stringFromAny(groupKeyPackage["owner_did"])
	if ownerDID == "" {
		ownerDID = record.DID
	}
	if ownerDID != record.DID {
		return nil, fmt.Errorf("group_key_package.owner_did must match active identity")
	}
	agentDID := stringFromAny(binding["agent_did"])
	if agentDID == "" {
		agentDID = record.DID
	}
	if agentDID != record.DID {
		return nil, fmt.Errorf("did_wba_binding.agent_did must match active identity")
	}
	verificationMethod := verificationMethodID(record.DIDDocument)
	if verificationMethod == "" {
		return nil, fmt.Errorf("active DID document does not expose a signing verification method")
	}
	leafSignatureKey := stringFromAny(binding["leaf_signature_key_b64u"])
	if leafSignatureKey == "" {
		return nil, fmt.Errorf("did_wba_binding.leaf_signature_key_b64u is required")
	}
	issuedAt := stringFromAny(binding["issued_at"])
	if issuedAt == "" {
		return nil, fmt.Errorf("did_wba_binding.issued_at is required")
	}
	expiresAt := stringFromAny(binding["expires_at"])
	if expiresAt == "" {
		return nil, fmt.Errorf("did_wba_binding.expires_at is required")
	}
	privateKey, err := loadPrivateKeyMaterial(record.Key1PrivatePEM)
	if err != nil {
		return nil, fmt.Errorf("load active identity signing key: %w", err)
	}
	signedBinding, err := anpsdk.GenerateDidWbaBinding(
		record.DID,
		verificationMethod,
		leafSignatureKey,
		privateKey,
		issuedAt,
		expiresAt,
		issuedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("sign did_wba_binding: %w", err)
	}
	signedPackageResult := cloneStringAnyMap(packageResult)
	signedGroupKeyPackage := cloneStringAnyMap(groupKeyPackage)
	signedGroupKeyPackage["owner_did"] = record.DID
	signedGroupKeyPackage["did_wba_binding"] = signedBinding
	signedPackageResult["group_key_package"] = signedGroupKeyPackage
	return signedPackageResult, nil
}

func cloneStringAnyMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func (s *Service) PublishGroupE2EEKeyPackage(ctx context.Context, identityName string, deviceID string, contractTest bool) (*CommandResult, error) {
	record, err := s.requireActiveIdentity(identityName)
	if err != nil {
		return nil, err
	}
	packageResult, published, err := s.publishGroupE2EEKeyPackage(ctx, record, deviceID, contractTest)
	if err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"mls":       packageResult,
			"published": published,
		},
		Summary: "Published group E2EE KeyPackage",
	}, nil
}

func (s *Service) createGroupE2EE(ctx context.Context, record *identity.StoredIdentity, groupDID string) (map[string]any, []string) {
	provider := s.groupMLSProvider()
	mlsHead, err := provider.CreateGroup(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-create-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   "default",
		Params: map[string]any{
			"agent_did": record.DID,
			"device_id": "default",
			"group_did": groupDID,
		},
	})
	if err != nil {
		return nil, []string{fmt.Sprintf("Group E2EE MLS create failed: %v", err)}
	}
	transport, _, err := s.httpTransport(record)
	if err != nil {
		return map[string]any{"mls": mlsHead}, []string{fmt.Sprintf("Group E2EE service transport unavailable: %v", err)}
	}
	delivery, err := transport.CreateGroupE2EE(ctx, groupDID, mlsHead)
	if err != nil {
		return map[string]any{"mls": mlsHead}, []string{fmt.Sprintf("Group E2EE create delivery failed: %v", err)}
	}
	warnings := s.persistGroupE2EESummary(ctx, record, groupDID, mlsHead, delivery)
	return map[string]any{"mls": mlsHead, "delivery": delivery}, warnings
}

func (s *Service) addGroupMemberE2EE(ctx context.Context, record *identity.StoredIdentity, groupDID string, memberDID string) (map[string]any, []string) {
	transport, _, err := s.httpTransport(record)
	if err != nil {
		return nil, []string{fmt.Sprintf("Group E2EE service transport unavailable: %v", err)}
	}
	leasedPackage, err := transport.GetGroupE2EEKeyPackage(ctx, memberDID)
	if err != nil {
		return nil, []string{fmt.Sprintf("Group E2EE member KeyPackage lookup failed: %v", err)}
	}
	provider := s.groupMLSProvider()
	mlsHead, err := provider.AddMember(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-add-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   "default",
		Params: map[string]any{
			"agent_did":          record.DID,
			"device_id":          "default",
			"group_did":          groupDID,
			"member_did":         memberDID,
			"group_key_package":  leasedPackage["group_key_package"],
			"key_package_id":     leasedPackage["key_package_id"],
			"target_key_package": leasedPackage,
		},
	})
	if err != nil {
		return map[string]any{"leased_key_package": redactedKeyPackageSummary(leasedPackage)}, []string{fmt.Sprintf("Group E2EE MLS add-member failed: %v", err)}
	}
	if keyPackageID := stringFromAny(leasedPackage["key_package_id"]); keyPackageID != "" {
		mlsHead["key_package_id"] = keyPackageID
	}
	if groupKeyPackage, ok := leasedPackage["group_key_package"]; ok {
		mlsHead["group_key_package"] = groupKeyPackage
	}
	delivery, err := transport.AddGroupE2EE(ctx, groupDID, memberDID, mlsHead)
	if err != nil {
		return map[string]any{"mls": mlsHead, "leased_key_package": redactedKeyPackageSummary(leasedPackage)}, []string{fmt.Sprintf("Group E2EE add delivery failed: %v", err)}
	}
	warnings := s.persistGroupE2EESummary(ctx, record, groupDID, mlsHead, delivery)
	localWelcome, localWelcomeWarnings := s.processLocalGroupWelcome(ctx, memberDID, groupDID, delivery, leasedPackage)
	warnings = append(warnings, localWelcomeWarnings...)
	result := map[string]any{"mls": mlsHead, "delivery": delivery, "leased_key_package": redactedKeyPackageSummary(leasedPackage)}
	if localWelcome != nil {
		result["local_welcome"] = localWelcome
	}
	return result, warnings
}

func (s *Service) removeGroupMemberE2EE(ctx context.Context, record *identity.StoredIdentity, request GroupMemberRequest) (map[string]any, []string, error) {
	operationID := "op-" + generateOperationID()
	provider := s.groupMLSProvider()
	prepared, err := provider.RemoveMember(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-remove-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   "default",
		Params: map[string]any{
			"agent_did":       record.DID,
			"actor_did":       record.DID,
			"device_id":       "default",
			"group_did":       request.Group,
			"member_did":      request.Member,
			"subject_did":     request.Member,
			"operation_id":    operationID,
			"group_state_ref": s.localGroupStateRef(ctx, record, request.Group),
		},
	})
	if err != nil {
		return nil, nil, err
	}
	return s.submitPreparedGroupE2EECommit(ctx, record, request.Group, request.Member, request.ReasonText, prepared, func(transport *HTTPTransport) (map[string]any, error) {
		return transport.RemoveGroupE2EE(ctx, request.Group, request.Member, prepared, request.ReasonText)
	})
}

func (s *Service) leaveGroupE2EE(ctx context.Context, record *identity.StoredIdentity, request GroupLeaveRequest) (map[string]any, []string, error) {
	operationID := "op-" + generateOperationID()
	provider := s.groupMLSProvider()
	prepared, err := provider.LeaveGroup(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-leave-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   "default",
		Params: map[string]any{
			"agent_did":       record.DID,
			"actor_did":       record.DID,
			"device_id":       "default",
			"group_did":       request.Group,
			"subject_did":     record.DID,
			"operation_id":    operationID,
			"group_state_ref": s.localGroupStateRef(ctx, record, request.Group),
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if reason := unsupportedGroupE2EESelfLeaveReason(prepared); reason != "" {
		data := map[string]any{"mls_prepare": prepared, "subject_did": record.DID}
		warnings := []string{"Group E2EE self-leave is unsupported in PR-A because anp-mls cannot produce an epoch-advancing remove commit for the leaving member."}
		if abortResult, abortErr := s.abortPreparedGroupE2EECommit(ctx, record, request.Group, prepared); abortErr != nil {
			warnings = append(warnings, fmt.Sprintf("Group E2EE local-terminal leave pending commit abort failed: %v", abortErr))
		} else {
			data["mls_abort"] = abortResult
			warnings = append(warnings, "Group E2EE local-terminal leave pending commit aborted before service submission.")
		}
		return data, warnings, fmt.Errorf("%w: %s; ask a group owner/admin to remove this member until an epoch-advancing leave-request flow is available", ErrGroupE2EESelfLeaveUnsupported, reason)
	}
	return s.submitPreparedGroupE2EECommit(ctx, record, request.Group, record.DID, "", prepared, func(transport *HTTPTransport) (map[string]any, error) {
		return transport.LeaveGroupE2EE(ctx, request.Group, prepared)
	})
}

func (s *Service) submitPreparedGroupE2EECommit(
	ctx context.Context,
	record *identity.StoredIdentity,
	groupDID string,
	subjectDID string,
	reasonText string,
	prepared map[string]any,
	submit func(*HTTPTransport) (map[string]any, error),
) (map[string]any, []string, error) {
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return map[string]any{"mls_prepare": prepared}, warnings, err
	}
	delivery, err := submit(transport)
	if err != nil {
		if shouldAbortGroupE2EEPendingCommit(err) {
			if abortResult, abortErr := s.abortPreparedGroupE2EECommit(ctx, record, groupDID, prepared); abortErr != nil {
				warnings = append(warnings, fmt.Sprintf("Group E2EE pending commit abort failed after service rejection: %v", abortErr))
			} else {
				warnings = append(warnings, "Group E2EE pending commit aborted after deterministic service rejection.")
				return map[string]any{"mls_prepare": prepared, "mls_abort": abortResult, "subject_did": subjectDID, "reason_text": reasonText}, warnings, fmt.Errorf("%w; local group E2EE pending commit aborted", err)
			}
		} else {
			warnings = append(warnings, "Group E2EE pending commit left intact after retryable or unknown service failure; retry with the same operation_id or inspect group e2ee status before finalize/abort.")
		}
		return map[string]any{"mls_prepare": prepared, "subject_did": subjectDID, "reason_text": reasonText}, warnings, fmt.Errorf("%w; local group E2EE pending commit retained for retry", err)
	}
	finalized, finalizeErr := s.finalizePreparedGroupE2EECommit(ctx, record, groupDID, prepared)
	if finalizeErr != nil {
		warnings = append(warnings, fmt.Sprintf("Group E2EE service accepted commit but local finalize failed: %v", finalizeErr))
	}
	summarySource := prepared
	if finalized != nil {
		summarySource = finalized
	}
	warnings = append(warnings, s.persistGroupE2EESummary(ctx, record, groupDID, summarySource, delivery)...)
	return map[string]any{
		"mls_prepare":  prepared,
		"mls_finalize": finalized,
		"delivery":     delivery,
		"subject_did":  subjectDID,
		"reason_text":  reasonText,
	}, warnings, nil
}

func (s *Service) finalizePreparedGroupE2EECommit(ctx context.Context, record *identity.StoredIdentity, groupDID string, prepared map[string]any) (map[string]any, error) {
	provider := s.groupMLSProvider()
	return provider.CommitFinalize(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-commit-finalize-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   "default",
		Params:     pendingCommitParams(record, groupDID, prepared),
	})
}

func (s *Service) abortPreparedGroupE2EECommit(ctx context.Context, record *identity.StoredIdentity, groupDID string, prepared map[string]any) (map[string]any, error) {
	provider := s.groupMLSProvider()
	return provider.CommitAbort(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-commit-abort-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   "default",
		Params:     pendingCommitParams(record, groupDID, prepared),
	})
}

func unsupportedGroupE2EESelfLeaveReason(prepared map[string]any) string {
	if len(prepared) == 0 {
		return ""
	}
	if stringFromAny(prepared["artifact_type"]) == "local-terminal-leave" {
		return "anp-mls returned a local-terminal leave artifact instead of an MLS epoch-advancing commit"
	}
	fromEpoch, hasFromEpoch := int64FromAny(prepared["from_epoch"])
	toEpoch, hasToEpoch := int64FromAny(prepared["to_epoch"])
	if !hasToEpoch {
		toEpoch, hasToEpoch = int64FromAny(prepared["epoch"])
	}
	if hasFromEpoch && hasToEpoch && toEpoch <= fromEpoch {
		return fmt.Sprintf("anp-mls returned non-advancing leave epochs from_epoch=%d to_epoch=%d", fromEpoch, toEpoch)
	}
	return ""
}

func int64FromAny(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(typed), true
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func pendingCommitParams(record *identity.StoredIdentity, groupDID string, prepared map[string]any) map[string]any {
	params := map[string]any{
		"agent_did":   record.DID,
		"actor_did":   record.DID,
		"device_id":   "default",
		"group_did":   groupDID,
		"commit_b64u": prepared["commit_b64u"],
	}
	for _, key := range []string{"pending_commit_id", "operation_id", "subject_did", "subject_status", "from_epoch", "to_epoch"} {
		if value, ok := prepared[key]; ok {
			params[key] = value
		}
	}
	return params
}

func shouldAbortGroupE2EEPendingCommit(err error) bool {
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		return false
	}
	if serviceErr.StatusCode >= http.StatusInternalServerError {
		return false
	}
	if serviceErr.StatusCode >= http.StatusBadRequest {
		return true
	}
	return serviceErr.RPCCode >= 2000
}

func (s *Service) sendGroupE2EE(ctx context.Context, record *identity.StoredIdentity, request SendRequest) (*CommandResult, error) {
	provider := s.groupMLSProvider()
	warnings := s.syncGroupState(ctx, record, request.Group, false)
	groupStateRef := s.localGroupStateRef(ctx, record, request.Group)
	operationID := "op-" + generateOperationID()
	messageID := "msg-" + generateOperationID()
	contentType := "application/anp-group-cipher+json"
	encryptResult, err := provider.Encrypt(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-encrypt-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   "default",
		Params: map[string]any{
			"agent_did":        record.DID,
			"device_id":        "default",
			"group_did":        request.Group,
			"group_state_ref":  groupStateRef,
			"sender_did":       record.DID,
			"content_type":     contentType,
			"security_profile": GroupE2EESecurityProfile,
			"message_id":       messageID,
			"operation_id":     operationID,
			"message_type":     request.MessageType,
			"application_plaintext": map[string]any{
				"application_content_type": contentTypeForMessageType(request.MessageType),
				"text":                     request.Text,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	cipher, _ := encryptResult["group_cipher_object"].(map[string]any)
	if len(cipher) == 0 {
		return nil, fmt.Errorf("anp-mls encrypt response missing group_cipher_object")
	}
	transport, transportWarnings, err := s.httpTransport(record)
	warnings = append(warnings, transportWarnings...)
	if err != nil {
		return nil, err
	}
	delivery, err := transport.SendGroupE2EE(ctx, request.Group, cipher, operationID, messageID)
	if err != nil {
		return nil, err
	}
	return s.persistGroupE2EESendResult(ctx, record, request, delivery, encryptResult, warnings)
}

func (s *Service) maybeDecryptGroupMessages(ctx context.Context, record *identity.StoredIdentity, groupDID string, raw map[string]any) ([]string, map[string]any) {
	messages := messagesFromResult(raw["messages"])
	if len(messages) == 0 {
		return nil, raw
	}
	provider := s.groupMLSProvider()
	deviceIDs := provider.candidateDeviceIDs(record.DID)
	warnings := make([]string, 0)
	for _, item := range messages {
		cipher := groupCipherObjectFromMessage(item)
		if len(cipher) == 0 {
			continue
		}
		aad := groupE2EEAADParamsFromMessage(groupDID, item, cipher)
		plain, err := decryptGroupCipherWithDevices(ctx, provider, record.DID, groupDID, cipher, aad, deviceIDs)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Group E2EE decrypt failed for message %s: %v", stringFromAny(item["id"]), err))
			continue
		}
		if appPlaintext, ok := plain["application_plaintext"].(map[string]any); ok {
			item["content"] = defaultString(stringFromAny(appPlaintext["text"]), metadataString(appPlaintext))
			item["content_type"] = defaultString(stringFromAny(appPlaintext["application_content_type"]), "text/plain")
			item["decrypted"] = true
		}
	}
	raw["messages"] = messages
	return compactWarnings(warnings), raw
}

func decryptGroupCipherWithDevices(ctx context.Context, provider MLSExecProvider, agentDID string, groupDID string, cipher map[string]any, aad map[string]any, deviceIDs []string) (map[string]any, error) {
	if len(deviceIDs) == 0 {
		deviceIDs = []string{"default"}
	}
	var lastErr error
	for _, deviceID := range deviceIDs {
		deviceID = defaultString(strings.TrimSpace(deviceID), "default")
		params := map[string]any{
			"agent_did":            agentDID,
			"recipient_did":        agentDID,
			"device_id":            deviceID,
			"group_did":            groupDID,
			"group_cipher_object":  cipher,
			"private_message_b64u": cipher["private_message_b64u"],
		}
		for key, value := range aad {
			if value != nil {
				params[key] = value
			}
		}
		plain, err := provider.Decrypt(ctx, MLSRequest{
			APIVersion: "anp-mls/v1",
			RequestID:  "group-e2ee-decrypt-" + generateOperationID(),
			AgentDID:   agentDID,
			DeviceID:   deviceID,
			Params:     params,
		})
		if err == nil {
			return plain, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no candidate MLS device state found")
}

func (s *Service) persistGroupE2EESummary(ctx context.Context, record *identity.StoredIdentity, groupDID string, mls map[string]any, delivery map[string]any) []string {
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return []string{fmt.Sprintf("Failed to open local store for group E2EE summary: %v", err)}
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return []string{fmt.Sprintf("Failed to ensure local schema for group E2EE summary: %v", err)}
	}
	metadata := map[string]any{
		"message_security_profile": GroupE2EESecurityProfile,
		"group_e2ee": map[string]any{
			"crypto_group_id_b64u": stringFromAny(firstNonNil(mls["crypto_group_id_b64u"], delivery["crypto_group_id_b64u"])),
			"epoch":                stringFromAny(firstNonNil(mls["epoch"], delivery["epoch"])),
			"epoch_authenticator":  stringFromAny(firstNonNil(mls["epoch_authenticator"], mls["epoch_authenticator_b64u"], delivery["epoch_authenticator"])),
			"suite":                stringFromAny(firstNonNil(mls["suite"], delivery["suite"])),
			"updated_at":           stringFromAny(delivery["updated_at"]),
			"operation_id":         stringFromAny(delivery["operation_id"]),
		},
	}
	if err := store.UpsertGroup(ctx, db, store.GroupRecord{
		OwnerDID:         record.DID,
		GroupID:          groupStorageKey(groupDID),
		GroupDID:         groupDID,
		MembershipStatus: "active",
		Metadata:         metadataString(metadata),
		CredentialName:   record.IdentityName,
	}); err != nil {
		return []string{fmt.Sprintf("Failed to persist group E2EE summary: %v", err)}
	}
	return nil
}

func (s *Service) persistGroupE2EESendResult(ctx context.Context, record *identity.StoredIdentity, request SendRequest, result *groupSendResult, encryptResult map[string]any, warnings []string) (*CommandResult, error) {
	commandResult, err := s.persistGroupSendResult(ctx, record, request, result, warnings, "http")
	if err != nil {
		return nil, err
	}
	if message, ok := commandResult.Data["message"].(map[string]any); ok {
		message["secure"] = true
		message["security_profile"] = GroupE2EESecurityProfile
	}
	commandResult.Data["e2ee"] = map[string]any{
		"encrypted":          true,
		"group_state_ref":    groupStateRefFromCipher(encryptResult),
		"cipher_object_sent": true,
	}
	commandResult.Summary = fmt.Sprintf("Sent a group %s message with group E2EE", request.MessageType)
	return commandResult, nil
}

func (s *Service) processLocalGroupWelcome(ctx context.Context, memberDID string, groupDID string, delivery map[string]any, leasedPackage map[string]any) (map[string]any, []string) {
	notice := e2eeNoticeObject(delivery)
	welcomeB64U := stringFromAny(notice["welcome_b64u"])
	if welcomeB64U == "" {
		return nil, nil
	}
	ratchetTreeB64U := stringFromAny(notice["ratchet_tree_b64u"])
	if ratchetTreeB64U == "" {
		return nil, []string{"Group E2EE local welcome processing skipped: notice missing ratchet_tree_b64u"}
	}
	memberRecord, err := s.localIdentityByDID(memberDID)
	if err != nil {
		return nil, nil
	}
	deviceID := groupE2EEWelcomeDeviceID(leasedPackage)
	provider := s.groupMLSProvider()
	welcomeResult, err := provider.ProcessWelcome(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-welcome-" + generateOperationID(),
		AgentDID:   memberRecord.DID,
		DeviceID:   deviceID,
		Params: map[string]any{
			"agent_did":         memberRecord.DID,
			"device_id":         deviceID,
			"group_did":         groupDID,
			"welcome_b64u":      welcomeB64U,
			"ratchet_tree_b64u": ratchetTreeB64U,
			"group_state_ref":   map[string]any{"group_did": groupDID},
		},
	})
	if err != nil {
		return nil, []string{fmt.Sprintf("Group E2EE local welcome processing failed for member %s: %v", memberDID, err)}
	}
	warnings := s.persistGroupE2EESummary(ctx, memberRecord, groupDID, welcomeResult, delivery)
	return map[string]any{
		"processed":  true,
		"group_did":  groupDID,
		"member_did": memberRecord.DID,
		"device_id":  deviceID,
		"epoch":      welcomeResult["epoch"],
	}, warnings
}

func (s *Service) PullGroupE2EENotices(ctx context.Context, identityName string, groupDID string, limit int) (*CommandResult, error) {
	record, err := s.requireActiveIdentity(identityName)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return nil, err
	}
	result, err := transport.PullGroupE2EENotices(ctx, groupDID, limit, false)
	if err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"notices":       noticesFromResult(result["notices"]),
			"pending_count": result["pending_count"],
			"group":         groupDID,
		},
		Summary:  "Pulled group E2EE pending notices",
		Warnings: warnings,
	}, nil
}

func (s *Service) RepairGroupE2EENotices(ctx context.Context, identityName string, groupDID string, limit int) (*CommandResult, error) {
	record, err := s.requireActiveIdentity(identityName)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return nil, err
	}
	pending, err := transport.PullGroupE2EENotices(ctx, groupDID, limit, false)
	if err != nil {
		return nil, err
	}
	processed := make([]map[string]any, 0)
	noticeIDs := make([]string, 0)
	provider := s.groupMLSProvider()
	for _, notice := range noticesFromResult(pending["notices"]) {
		noticeType := stringFromAny(notice["notice_type"])
		if noticeType != "welcome-delivery" && noticeType != "commit-delivery" {
			continue
		}
		targetGroupDID := defaultString(stringFromAny(notice["group_did"]), groupDID)
		if targetGroupDID == "" {
			warnings = append(warnings, fmt.Sprintf("Group E2EE repair skipped %s notice without group_did", noticeType))
			continue
		}
		recipient := firstNonEmptyString(notice["recipient_did"], notice["member_did"])
		if noticeType == "welcome-delivery" && recipient == "" {
			recipient = stringFromAny(notice["subject_did"])
		}
		if recipient != "" && recipient != record.DID {
			warnings = append(warnings, fmt.Sprintf("Group E2EE repair skipped notice for different recipient %s", recipient))
			continue
		}
		if noticeType == "commit-delivery" {
			commit, noticeWarnings := s.processGroupCommitNotice(ctx, record, targetGroupDID, notice)
			if commit != nil {
				processed = append(processed, commit)
				if noticeID := stringFromAny(notice["notice_id"]); noticeID != "" {
					noticeIDs = append(noticeIDs, noticeID)
				}
				continue
			}
			warnings = append(warnings, noticeWarnings...)
			continue
		}
		welcome, noticeWarnings := s.processGroupWelcomeNotice(ctx, record, targetGroupDID, notice)
		if welcome != nil {
			processed = append(processed, welcome)
			if noticeID := stringFromAny(notice["notice_id"]); noticeID != "" {
				noticeIDs = append(noticeIDs, noticeID)
			}
			continue
		}
		if s.groupWelcomeAlreadyAvailable(ctx, provider, record, targetGroupDID, notice) {
			processed = append(processed, map[string]any{
				"processed":        true,
				"already_restored": true,
				"notice_id":        notice["notice_id"],
				"group_did":        targetGroupDID,
				"member_did":       record.DID,
				"device_id":        defaultString(stringFromAny(notice["device_id"]), "default"),
			})
			if noticeID := stringFromAny(notice["notice_id"]); noticeID != "" {
				noticeIDs = append(noticeIDs, noticeID)
			}
			continue
		}
		warnings = append(warnings, noticeWarnings...)
	}
	delivered := map[string]any(nil)
	if len(noticeIDs) > 0 {
		delivered, err = transport.MarkGroupE2EENoticesDelivered(ctx, groupDID, noticeIDs)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Group E2EE repair processed notices but failed to mark delivered: %v", err))
		}
	}
	return &CommandResult{
		Data: map[string]any{
			"processed":        processed,
			"processed_count":  len(processed),
			"pending_count":    pending["pending_count"],
			"delivered_result": delivered,
			"group":            groupDID,
		},
		Summary:  "Replayed group E2EE pending notices",
		Warnings: compactWarnings(warnings),
	}, nil
}

func (s *Service) processGroupCommitNotice(ctx context.Context, record *identity.StoredIdentity, groupDID string, notice map[string]any) (map[string]any, []string) {
	commitB64U := stringFromAny(notice["commit_b64u"])
	if commitB64U == "" {
		return nil, []string{"Group E2EE repair skipped commit notice missing commit_b64u"}
	}
	deviceID := defaultString(stringFromAny(notice["device_id"]), "default")
	groupStateRef, _ := notice["group_state_ref"].(map[string]any)
	if len(groupStateRef) == 0 {
		groupStateRef = map[string]any{
			"group_did": groupDID,
		}
		if cryptoGroupID := stringFromAny(notice["crypto_group_id_b64u"]); cryptoGroupID != "" {
			groupStateRef["crypto_group_id_b64u"] = cryptoGroupID
		}
		if fromEpoch := stringFromAny(notice["from_epoch"]); fromEpoch != "" {
			groupStateRef["epoch"] = fromEpoch
		}
	}
	params := map[string]any{
		"agent_did":            record.DID,
		"device_id":            deviceID,
		"group_did":            groupDID,
		"group_state_ref":      groupStateRef,
		"commit_b64u":          commitB64U,
		"ratchet_tree_b64u":    notice["ratchet_tree_b64u"],
		"group_info_b64u":      notice["group_info_b64u"],
		"operation_id":         notice["operation_id"],
		"notice_id":            notice["notice_id"],
		"actor_did":            notice["actor_did"],
		"subject_did":          notice["subject_did"],
		"subject_status":       notice["subject_status"],
		"from_epoch":           notice["from_epoch"],
		"to_epoch":             notice["to_epoch"],
		"crypto_group_id_b64u": notice["crypto_group_id_b64u"],
		"epoch_authenticator":  firstNonNil(notice["epoch_authenticator"], notice["epoch_authenticator_b64u"]),
	}
	provider := s.groupMLSProvider()
	commitResult, err := provider.ProcessCommit(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-commit-repair-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   deviceID,
		Params:     params,
	})
	if err != nil {
		return nil, []string{fmt.Sprintf("Group E2EE repair commit processing failed: %v", err)}
	}
	warnings := []string(nil)
	subjectDID := stringFromAny(notice["subject_did"])
	subjectStatus := stringFromAny(notice["subject_status"])
	if subjectDID == record.DID && (subjectStatus == "removed" || subjectStatus == "left") {
		warnings = append(warnings, s.markCachedGroupLeft(ctx, record, groupDID)...)
	} else {
		warnings = append(warnings, s.persistGroupE2EESummary(ctx, record, groupDID, commitResult, notice)...)
	}
	return map[string]any{
		"processed":      true,
		"notice_type":    "commit-delivery",
		"notice_id":      notice["notice_id"],
		"group_did":      groupDID,
		"member_did":     record.DID,
		"device_id":      deviceID,
		"epoch":          firstNonNil(commitResult["epoch"], notice["to_epoch"]),
		"subject_did":    subjectDID,
		"subject_status": subjectStatus,
	}, warnings
}

func (s *Service) groupWelcomeAlreadyAvailable(ctx context.Context, provider MLSExecProvider, record *identity.StoredIdentity, groupDID string, notice map[string]any) bool {
	deviceID := defaultString(stringFromAny(notice["device_id"]), "default")
	resp, err := provider.Call(ctx, "group", "status", MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-welcome-status-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   deviceID,
		Params: map[string]any{
			"agent_did": record.DID,
			"device_id": deviceID,
			"group_did": groupDID,
		},
	})
	if err != nil || resp == nil {
		return false
	}
	return stringFromAny(resp.Result["status"]) == "active"
}

func (s *Service) processGroupWelcomeNotice(ctx context.Context, record *identity.StoredIdentity, groupDID string, notice map[string]any) (map[string]any, []string) {
	welcomeB64U := stringFromAny(notice["welcome_b64u"])
	if welcomeB64U == "" {
		return nil, []string{"Group E2EE repair skipped welcome notice missing welcome_b64u"}
	}
	ratchetTreeB64U := stringFromAny(notice["ratchet_tree_b64u"])
	if ratchetTreeB64U == "" {
		return nil, []string{"Group E2EE repair skipped welcome notice missing ratchet_tree_b64u"}
	}
	deviceID := defaultString(stringFromAny(notice["device_id"]), "default")
	provider := s.groupMLSProvider()
	welcomeResult, err := provider.ProcessWelcome(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-welcome-repair-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   deviceID,
		Params: map[string]any{
			"agent_did":         record.DID,
			"device_id":         deviceID,
			"group_did":         groupDID,
			"welcome_b64u":      welcomeB64U,
			"ratchet_tree_b64u": ratchetTreeB64U,
			"group_state_ref":   firstNonNil(notice["group_state_ref"], map[string]any{"group_did": groupDID}),
		},
	})
	if err != nil {
		return nil, []string{fmt.Sprintf("Group E2EE repair welcome processing failed: %v", err)}
	}
	warnings := s.persistGroupE2EESummary(ctx, record, groupDID, welcomeResult, notice)
	return map[string]any{
		"processed":  true,
		"notice_id":  notice["notice_id"],
		"group_did":  groupDID,
		"member_did": record.DID,
		"device_id":  deviceID,
		"epoch":      welcomeResult["epoch"],
	}, warnings
}

func (s *Service) localIdentityByDID(did string) (*identity.StoredIdentity, error) {
	if s == nil || s.manager == nil {
		return nil, identity.ErrIdentityNotFound
	}
	summaries, err := s.manager.List()
	if err != nil {
		return nil, err
	}
	for _, summary := range summaries {
		if summary.DID == did {
			return s.manager.Load(summary.IdentityName)
		}
	}
	return nil, identity.ErrIdentityNotFound
}

func e2eeNoticeObject(delivery map[string]any) map[string]any {
	if notice, ok := delivery["e2ee_notice"].(map[string]any); ok {
		return notice
	}
	return nil
}

func groupE2EEWelcomeDeviceID(leasedPackage map[string]any) string {
	if groupKeyPackage, ok := leasedPackage["group_key_package"].(map[string]any); ok {
		if deviceID := stringFromAny(groupKeyPackage["device_id"]); deviceID != "" {
			return deviceID
		}
	}
	if deviceID := stringFromAny(leasedPackage["device_id"]); deviceID != "" {
		return deviceID
	}
	return "default"
}

func (s *Service) localGroupStateRef(ctx context.Context, record *identity.StoredIdentity, groupDID string) map[string]any {
	snapshot, err := s.readCachedGroupSnapshot(ctx, record, groupDID)
	if err != nil {
		return map[string]any{"group_did": groupDID}
	}
	return groupStateRefFromSnapshot(groupDID, snapshot)
}

func groupStateRefFromSnapshot(groupDID string, snapshot map[string]any) map[string]any {
	ref := map[string]any{"group_did": groupDID}
	metadata := decodeMetadataMap(snapshot["metadata"])
	if version := firstNonEmptyString(snapshot["group_state_version"], metadata["group_state_version"]); version != "" {
		ref["group_state_version"] = version
	}
	if e2ee, ok := metadata["group_e2ee"].(map[string]any); ok {
		if version := stringFromAny(e2ee["group_state_version"]); version != "" {
			ref["group_state_version"] = version
		}
		if cryptoGroupID := stringFromAny(e2ee["crypto_group_id_b64u"]); cryptoGroupID != "" {
			ref["crypto_group_id_b64u"] = cryptoGroupID
		}
	}
	return ref
}

func groupRequestUsesE2EE(request GroupCreateRequest) bool {
	return request.E2EE || strings.TrimSpace(request.MessageSecurityProfile) == GroupE2EESecurityProfile
}

func groupMemberMutationUsesE2EE(request GroupMemberRequest, preMutationSnapshot map[string]any, postMutationSnapshot map[string]any) bool {
	return request.E2EE || groupSnapshotUsesE2EE(preMutationSnapshot) || groupSnapshotUsesE2EE(postMutationSnapshot)
}

func groupSnapshotUsesE2EE(snapshot map[string]any) bool {
	if len(snapshot) == 0 {
		return false
	}
	if stringFromAny(snapshot["message_security_profile"]) == GroupE2EESecurityProfile {
		return true
	}
	if policy, ok := snapshot["group_policy"].(map[string]any); ok {
		if stringFromAny(policy["message_security_profile"]) == GroupE2EESecurityProfile {
			return true
		}
	}
	metadata := decodeMetadataMap(snapshot["metadata"])
	if stringFromAny(metadata["message_security_profile"]) == GroupE2EESecurityProfile {
		return true
	}
	return false
}

func decodeMetadataMap(value any) map[string]any {
	text := stringFromAny(value)
	if text == "" {
		if typed, ok := value.(map[string]any); ok {
			return typed
		}
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil
	}
	return result
}

func groupCipherObjectFromMessage(item map[string]any) map[string]any {
	for _, key := range []string{"group_cipher_object", "content"} {
		if cipher, ok := item[key].(map[string]any); ok {
			if nested, ok := cipher["group_cipher_object"].(map[string]any); ok {
				return nested
			}
			if _, ok := cipher["private_message_b64u"]; ok {
				return cipher
			}
		}
	}
	body, _ := item["body"].(map[string]any)
	if cipher, ok := body["group_cipher_object"].(map[string]any); ok {
		return cipher
	}
	return nil
}

func groupE2EEAADParamsFromMessage(groupDID string, item map[string]any, cipher map[string]any) map[string]any {
	receipt, _ := item["receipt"].(map[string]any)
	groupStateRef, _ := cipher["group_state_ref"].(map[string]any)
	if len(groupStateRef) == 0 {
		groupStateRef = map[string]any{"group_did": groupDID}
	}
	params := map[string]any{
		"group_state_ref":  groupStateRef,
		"sender_did":       stringFromAny(item["sender_did"]),
		"content_type":     "application/anp-group-cipher+json",
		"security_profile": GroupE2EESecurityProfile,
		"message_id":       firstNonEmptyString(item["message_id"], item["id"]),
		"operation_id":     firstNonEmptyString(item["operation_id"], receipt["operation_id"]),
	}
	return params
}

func groupStateRefFromCipher(encryptResult map[string]any) map[string]any {
	cipher, _ := encryptResult["group_cipher_object"].(map[string]any)
	ref, _ := cipher["group_state_ref"].(map[string]any)
	return ref
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text := stringFromAny(value); text != "" {
			return text
		}
	}
	return ""
}

func noticesFromResult(value any) []map[string]any {
	return messagesFromResult(value)
}

func redactedKeyPackageSummary(raw map[string]any) map[string]any {
	return map[string]any{
		"target_did":       raw["target_did"],
		"key_package_id":   raw["key_package_id"],
		"leased":           true,
		"private_material": false,
	}
}
