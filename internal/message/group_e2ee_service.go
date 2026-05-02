package message

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	delivery, err := transport.AddGroupE2EE(ctx, groupDID, memberDID, mlsHead)
	if err != nil {
		return map[string]any{"mls": mlsHead, "leased_key_package": redactedKeyPackageSummary(leasedPackage)}, []string{fmt.Sprintf("Group E2EE add delivery failed: %v", err)}
	}
	warnings := s.persistGroupE2EESummary(ctx, record, groupDID, mlsHead, delivery)
	return map[string]any{"mls": mlsHead, "delivery": delivery, "leased_key_package": redactedKeyPackageSummary(leasedPackage)}, warnings
}

func (s *Service) sendGroupE2EE(ctx context.Context, record *identity.StoredIdentity, request SendRequest) (*CommandResult, error) {
	provider := s.groupMLSProvider()
	groupStateRef := s.localGroupStateRef(ctx, record, request.Group)
	encryptResult, err := provider.Encrypt(ctx, MLSRequest{
		APIVersion: "anp-mls/v1",
		RequestID:  "group-e2ee-encrypt-" + generateOperationID(),
		AgentDID:   record.DID,
		DeviceID:   "default",
		Params: map[string]any{
			"agent_did":       record.DID,
			"device_id":       "default",
			"group_did":       request.Group,
			"group_state_ref": groupStateRef,
			"message_type":    request.MessageType,
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
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return nil, err
	}
	delivery, err := transport.SendGroupE2EE(ctx, request.Group, cipher)
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
	warnings := make([]string, 0)
	for _, item := range messages {
		cipher := groupCipherObjectFromMessage(item)
		if len(cipher) == 0 {
			continue
		}
		plain, err := provider.Decrypt(ctx, MLSRequest{
			APIVersion: "anp-mls/v1",
			RequestID:  "group-e2ee-decrypt-" + generateOperationID(),
			AgentDID:   record.DID,
			DeviceID:   "default",
			Params: map[string]any{
				"agent_did":            record.DID,
				"device_id":            "default",
				"group_did":            groupDID,
				"group_cipher_object":  cipher,
				"private_message_b64u": cipher["private_message_b64u"],
			},
		})
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

func (s *Service) localGroupStateRef(ctx context.Context, record *identity.StoredIdentity, groupDID string) map[string]any {
	ref := map[string]any{"group_did": groupDID}
	snapshot, err := s.readCachedGroupSnapshot(ctx, record, groupDID)
	if err != nil {
		return ref
	}
	if version := stringFromAny(snapshot["group_state_version"]); version != "" {
		ref["group_state_version"] = version
	}
	metadata := decodeMetadataMap(snapshot["metadata"])
	if e2ee, ok := metadata["group_e2ee"].(map[string]any); ok {
		if epoch := stringFromAny(e2ee["epoch"]); epoch != "" {
			ref["epoch"] = epoch
		}
	}
	return ref
}

func groupRequestUsesE2EE(request GroupCreateRequest) bool {
	return request.E2EE || strings.TrimSpace(request.MessageSecurityProfile) == GroupE2EESecurityProfile
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

func groupStateRefFromCipher(encryptResult map[string]any) map[string]any {
	cipher, _ := encryptResult["group_cipher_object"].(map[string]any)
	ref, _ := cipher["group_state_ref"].(map[string]any)
	return ref
}

func redactedKeyPackageSummary(raw map[string]any) map[string]any {
	return map[string]any{
		"target_did":       raw["target_did"],
		"key_package_id":   raw["key_package_id"],
		"leased":           true,
		"private_material": false,
	}
}
