package message

import (
	"fmt"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/identity"
)

func BuildGroupCreateRPCParams(record *identity.StoredIdentity, manager *identity.Manager, serviceDID string, request GroupCreateRequest) (map[string]any, error) {
	if strings.TrimSpace(serviceDID) == "" {
		return nil, fmt.Errorf("message service did is required")
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	profile := buildGroupProfilePatch(request.Name, request.Description, request.Discoverability, request.Slug, request.Goal, request.Rules, request.MessagePrompt, request.DocURL)
	policy := buildGroupPolicyPatch(request.AdmissionMode, request.AttachmentsAllowed, request.MaxMembers, request.MemberMaxMessages, request.MemberMaxTotalChars)
	if _, ok := profile["display_name"]; !ok {
		return nil, fmt.Errorf("group display name is required")
	}
	if len(policy) == 0 {
		policy = buildGroupPolicyPatch("open-join", boolPtr(true), "500", nil, nil)
	}
	if securityProfile := normalizedGroupSecurityProfile(request); securityProfile != "" {
		policy["message_security_profile"] = securityProfile
		policy["bootstrap_security_profile"] = securityProfile
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          "anp.group.base.v1",
		"security_profile": "transport-protected",
		"sender_did":       record.DID,
		"target": map[string]any{
			"kind": "service",
			"did":  serviceDID,
		},
		"operation_id": "op-" + generateOperationID(),
		"created_at":   nowRFC3339(),
		"content_type": "application/json",
	}
	body := map[string]any{
		"group_profile": profile,
		"group_policy":  policy,
	}
	payload := signedPayload{Method: "group.create", Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

func BuildGroupGetInfoRPCParams(record *identity.StoredIdentity, request GroupInfoRequest) (map[string]any, error) {
	groupDID := strings.TrimSpace(request.Group)
	if groupDID == "" {
		return nil, ErrGroupRequired
	}
	body := map[string]any{}
	if request.IncludePolicy {
		body["include_policy"] = true
	}
	if request.IncludeMemberList {
		body["include_member_list"] = true
	}
	return map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.group.base.v1",
			"security_profile": "transport-protected",
			"sender_did":       record.DID,
			"target": map[string]any{
				"kind": "group",
				"did":  groupDID,
			},
		},
		"body": body,
	}, nil
}

func BuildGroupJoinRPCParams(record *identity.StoredIdentity, manager *identity.Manager, request GroupJoinRequest) (map[string]any, error) {
	body := map[string]any{}
	if reason := strings.TrimSpace(request.ReasonText); reason != "" {
		body["reason_text"] = reason
	}
	return buildGroupMutationRPCParams(record, manager, request.Group, "group.join", body)
}

func BuildGroupAddRPCParams(record *identity.StoredIdentity, manager *identity.Manager, request GroupMemberRequest) (map[string]any, error) {
	memberDID := strings.TrimSpace(request.Member)
	if memberDID == "" {
		return nil, ErrMemberRequired
	}
	body := map[string]any{"member_did": memberDID}
	if role := strings.TrimSpace(request.Role); role != "" {
		body["role"] = role
	}
	if reason := strings.TrimSpace(request.ReasonText); reason != "" {
		body["reason_text"] = reason
	}
	return buildGroupMutationRPCParams(record, manager, request.Group, "group.add", body)
}

func BuildGroupRemoveRPCParams(record *identity.StoredIdentity, manager *identity.Manager, request GroupMemberRequest) (map[string]any, error) {
	memberDID := strings.TrimSpace(request.Member)
	if memberDID == "" {
		return nil, ErrMemberRequired
	}
	body := map[string]any{"member_did": memberDID}
	if reason := strings.TrimSpace(request.ReasonText); reason != "" {
		body["reason_text"] = reason
	}
	return buildGroupMutationRPCParams(record, manager, request.Group, "group.remove", body)
}

func BuildGroupLeaveRPCParams(record *identity.StoredIdentity, manager *identity.Manager, request GroupLeaveRequest) (map[string]any, error) {
	return buildGroupMutationRPCParams(record, manager, request.Group, "group.leave", map[string]any{})
}

func BuildGroupUpdateProfileRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, patch map[string]any) (map[string]any, error) {
	if len(patch) == 0 {
		return nil, fmt.Errorf("group profile patch is required")
	}
	return buildGroupMutationRPCParams(record, manager, groupDID, "group.update_profile", map[string]any{"group_profile_patch": patch})
}

func BuildGroupUpdatePolicyRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, patch map[string]any) (map[string]any, error) {
	if len(patch) == 0 {
		return nil, fmt.Errorf("group policy patch is required")
	}
	return buildGroupMutationRPCParams(record, manager, groupDID, "group.update_policy", map[string]any{"group_policy_patch": patch})
}

func BuildGroupSendRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, text string, messageType string) (map[string]any, error) {
	if strings.TrimSpace(groupDID) == "" {
		return nil, ErrGroupRequired
	}
	if strings.TrimSpace(text) == "" {
		return nil, ErrTextRequired
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	contentType := contentTypeForMessageType(messageType)
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          "anp.group.base.v1",
		"security_profile": "transport-protected",
		"sender_did":       record.DID,
		"target": map[string]any{
			"kind": "group",
			"did":  groupDID,
		},
		"operation_id": "op-" + generateOperationID(),
		"message_id":   "msg-" + generateOperationID(),
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"content_type": contentType,
	}
	body := map[string]any{"text": text}
	payload := signedPayload{Method: "group.send", Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

func BuildGroupE2EECreateRPCParams(record *identity.StoredIdentity, manager *identity.Manager, serviceDID string, groupDID string, mlsHead map[string]any) (map[string]any, error) {
	return buildGroupE2EERPCParams(record, manager, "service", serviceDID, "group.e2ee.create", e2eeHeadBody(groupDID, "", mlsHead), "", "", "", GroupE2EESecurityProfile)
}

func BuildGroupE2EEAddRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, memberDID string, mlsHead map[string]any) (map[string]any, error) {
	body := e2eeHeadBody(groupDID, memberDID, mlsHead)
	if value, ok := mlsHead["welcome_b64u"]; ok {
		body["welcome_b64u"] = value
	}
	if value, ok := mlsHead["commit_b64u"]; ok {
		body["commit_b64u"] = value
	}
	if value, ok := mlsHead["ratchet_tree_b64u"]; ok {
		body["ratchet_tree_b64u"] = value
	}
	if value, ok := mlsHead["key_package_id"]; ok {
		body["key_package_id"] = value
		body["subject_key_package_id"] = value
	}
	if value, ok := mlsHead["group_key_package"]; ok {
		body["group_key_package"] = value
	}
	return buildGroupE2EERPCParams(record, manager, "group", groupDID, "group.e2ee.add", body, "", "", "", GroupE2EESecurityProfile)
}

func BuildGroupE2EERemoveRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, memberDID string, preparedCommit map[string]any, reasonText string, leaveRequestID string) (map[string]any, error) {
	body := e2eeMembershipCommitBody(groupDID, memberDID, "removed", preparedCommit)
	if reason := strings.TrimSpace(reasonText); reason != "" {
		body["reason_text"] = reason
	}
	if requestID := strings.TrimSpace(leaveRequestID); requestID != "" {
		body["leave_request_id"] = requestID
	}
	return buildGroupE2EERPCParams(record, manager, "group", groupDID, "group.e2ee.remove", body, "", stringFromAny(preparedCommit["operation_id"]), "", GroupE2EESecurityProfile)
}

func BuildGroupE2EELeaveRequestRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, reasonText string) (map[string]any, error) {
	body := map[string]any{
		"group_did":       strings.TrimSpace(groupDID),
		"subject_did":     record.DID,
		"member_did":      record.DID,
		"subject_status":  "leave_requested",
		"group_state_ref": map[string]any{"group_did": strings.TrimSpace(groupDID)},
	}
	if reason := strings.TrimSpace(reasonText); reason != "" {
		body["reason_text"] = reason
	}
	return buildGroupE2EERPCParams(record, manager, "group", groupDID, "group.e2ee.leave_request", body, "", "", "", GroupE2EETransportProfile)
}

func BuildGroupE2EELeaveRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, preparedCommit map[string]any) (map[string]any, error) {
	body := e2eeMembershipCommitBody(groupDID, record.DID, "left", preparedCommit)
	return buildGroupE2EERPCParams(record, manager, "group", groupDID, "group.e2ee.leave", body, "", stringFromAny(preparedCommit["operation_id"]), "", GroupE2EESecurityProfile)
}

func BuildGroupE2EESendRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, cipher map[string]any, operationID string, messageID string) (map[string]any, error) {
	return buildGroupE2EERPCParams(record, manager, "group", groupDID, "group.e2ee.send", map[string]any{"group_cipher_object": sanitizeGroupCipherObjectForService(cipher)}, "application/anp-group-cipher+json", operationID, messageID, GroupE2EESecurityProfile)
}

func sanitizeGroupCipherObjectForService(cipher map[string]any) map[string]any {
	sanitized := make(map[string]any)
	for _, key := range []string{
		"crypto_group_id_b64u",
		"epoch",
		"private_message_b64u",
		"group_state_ref",
		"epoch_authenticator",
		"non_cryptographic",
		"artifact_mode",
	} {
		if value, ok := cipher[key]; ok {
			sanitized[key] = value
		}
	}
	return sanitized
}

func BuildGroupE2EEPublishKeyPackageRPCParams(record *identity.StoredIdentity, manager *identity.Manager, serviceDID string, packageResult map[string]any) (map[string]any, error) {
	serviceDID = strings.TrimSpace(serviceDID)
	if serviceDID == "" {
		return nil, fmt.Errorf("message service did is required")
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	groupKeyPackage, _ := packageResult["group_key_package"].(map[string]any)
	if len(groupKeyPackage) == 0 {
		return nil, fmt.Errorf("group_key_package is required")
	}
	groupKeyPackage = sanitizeGroupKeyPackageForService(groupKeyPackage)
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          GroupE2EEProfile,
		"security_profile": GroupE2EETransportProfile,
		"sender_did":       record.DID,
		"target":           map[string]any{"kind": "service", "did": serviceDID},
		"operation_id":     "op-" + generateOperationID(),
		"created_at":       nowRFC3339(),
		"content_type":     "application/json",
	}
	body := map[string]any{"group_key_package": groupKeyPackage}
	payload := signedPayload{Method: "group.e2ee.publish_key_package", Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

func BuildGroupE2EEGetKeyPackageRPCParams(record *identity.StoredIdentity, manager *identity.Manager, serviceDID string, targetDID string) (map[string]any, error) {
	return buildGroupE2EEGetKeyPackageRPCParams(record, manager, serviceDID, map[string]any{"target_did": strings.TrimSpace(targetDID)})
}

func BuildGroupE2EEGetRecoveryKeyPackageRPCParams(record *identity.StoredIdentity, manager *identity.Manager, serviceDID string, groupDID string, targetDID string, deviceID string) (map[string]any, error) {
	body := map[string]any{
		"target_did": targetDID,
		"purpose":    "recovery",
		"group_did":  strings.TrimSpace(groupDID),
		"device_id":  defaultString(strings.TrimSpace(deviceID), "default"),
	}
	return buildGroupE2EEGetKeyPackageRPCParams(record, manager, serviceDID, body)
}

func buildGroupE2EEGetKeyPackageRPCParams(record *identity.StoredIdentity, manager *identity.Manager, serviceDID string, body map[string]any) (map[string]any, error) {
	serviceDID = strings.TrimSpace(serviceDID)
	targetDID := strings.TrimSpace(stringFromAny(body["target_did"]))
	if serviceDID == "" {
		return nil, fmt.Errorf("message service did is required")
	}
	if targetDID == "" {
		return nil, ErrMemberRequired
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          GroupE2EEProfile,
		"security_profile": GroupE2EETransportProfile,
		"sender_did":       record.DID,
		"target":           map[string]any{"kind": "service", "did": serviceDID},
		"operation_id":     "op-" + generateOperationID(),
		"created_at":       nowRFC3339(),
		"content_type":     "application/json",
	}
	payload := signedPayload{Method: "group.e2ee.get_key_package", Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

func BuildGroupE2EERecoverMemberRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, memberDID string, deviceID string, prepared map[string]any, leasedPackage map[string]any) (map[string]any, error) {
	body := e2eeRecoveryCommitBody(groupDID, memberDID, deviceID, prepared, leasedPackage)
	return buildGroupE2EERPCParams(record, manager, "group", groupDID, "group.e2ee.recover_member", body, "", stringFromAny(prepared["operation_id"]), "", GroupE2EESecurityProfile)
}

func BuildGroupE2EENoticeRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, limit int, markDelivered bool, noticeIDs []string) (map[string]any, error) {
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          GroupE2EEProfile,
		"security_profile": GroupE2EETransportProfile,
		"sender_did":       record.DID,
		"target":           map[string]any{"kind": "agent", "did": record.DID},
		"operation_id":     "op-" + generateOperationID(),
		"created_at":       nowRFC3339(),
		"content_type":     "application/json",
	}
	body := map[string]any{"limit": limit}
	if strings.TrimSpace(groupDID) != "" {
		body["group_did"] = strings.TrimSpace(groupDID)
	}
	if markDelivered {
		body["mark_delivered"] = true
	}
	if len(noticeIDs) > 0 {
		ids := make([]string, 0, len(noticeIDs))
		for _, noticeID := range noticeIDs {
			if trimmed := strings.TrimSpace(noticeID); trimmed != "" {
				ids = append(ids, trimmed)
			}
		}
		if len(ids) > 0 {
			body["notice_ids"] = ids
		}
	}
	payload := signedPayload{Method: "group.e2ee.notice", Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

func BuildGroupE2EEHeadRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string) (map[string]any, error) {
	groupDID = strings.TrimSpace(groupDID)
	if groupDID == "" {
		return nil, ErrGroupRequired
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          GroupE2EEProfile,
		"security_profile": GroupE2EETransportProfile,
		"sender_did":       record.DID,
		"target":           map[string]any{"kind": "group", "did": groupDID},
		"operation_id":     "op-" + generateOperationID(),
		"created_at":       nowRFC3339(),
		"content_type":     "application/json",
	}
	body := map[string]any{
		"group_did":       groupDID,
		"group_state_ref": map[string]any{"group_did": groupDID},
	}
	payload := signedPayload{Method: "group.e2ee.head", Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

func sanitizeGroupKeyPackageForService(input map[string]any) map[string]any {
	allowed := map[string]struct{}{
		"owner_did":            {},
		"device_id":            {},
		"key_package_id":       {},
		"suite":                {},
		"mls_key_package_b64u": {},
		"did_wba_binding":      {},
		"expires_at":           {},
		"purpose":              {},
		"group_did":            {},
		"non_cryptographic":    {},
		"artifact_mode":        {},
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		if _, ok := allowed[key]; ok {
			if (key == "group_did" || key == "purpose") && strings.TrimSpace(stringFromAny(value)) == "" {
				continue
			}
			output[key] = value
		}
	}
	return output
}

func BuildGroupGetRPCParams(record *identity.StoredIdentity, request GroupGetRequest) (map[string]any, error) {
	groupDID := strings.TrimSpace(request.Group)
	if groupDID == "" {
		return nil, ErrGroupRequired
	}
	return map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.group.local.v1",
			"security_profile": "transport-protected",
			"sender_did":       record.DID,
			"target": map[string]any{
				"kind": "group",
				"did":  groupDID,
			},
		},
		"body": map[string]any{"group_did": groupDID},
	}, nil
}

func BuildGroupMembersRPCParams(record *identity.StoredIdentity, request GroupMembersRequest) (map[string]any, error) {
	groupDID := strings.TrimSpace(request.Group)
	if groupDID == "" {
		return nil, ErrGroupRequired
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 100
	}
	return map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.group.local.v1",
			"security_profile": "transport-protected",
			"sender_did":       record.DID,
			"target": map[string]any{
				"kind": "group",
				"did":  groupDID,
			},
		},
		"body": map[string]any{"group_did": groupDID, "limit": limit},
	}, nil
}

func BuildGroupMessagesRPCParams(record *identity.StoredIdentity, request GroupMessagesRequest) (map[string]any, error) {
	groupDID := strings.TrimSpace(request.Group)
	if groupDID == "" {
		return nil, ErrGroupRequired
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 50
	}
	body := map[string]any{"group_did": groupDID, "limit": limit}
	if cursor := strings.TrimSpace(request.Cursor); cursor != "" {
		body["since_seq"] = cursor
	}
	if request.Skip > 0 {
		body["skip"] = request.Skip
	}
	return map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.group.local.v1",
			"security_profile": "transport-protected",
			"sender_did":       record.DID,
			"target": map[string]any{
				"kind": "group",
				"did":  groupDID,
			},
		},
		"body": body,
	}, nil
}

func buildGroupMutationRPCParams(record *identity.StoredIdentity, manager *identity.Manager, groupDID string, method string, body map[string]any) (map[string]any, error) {
	groupDID = strings.TrimSpace(groupDID)
	if groupDID == "" {
		return nil, ErrGroupRequired
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          "anp.group.base.v1",
		"security_profile": "transport-protected",
		"sender_did":       record.DID,
		"target": map[string]any{
			"kind": "group",
			"did":  groupDID,
		},
		"operation_id": "op-" + generateOperationID(),
		"created_at":   nowRFC3339(),
		"content_type": "application/json",
	}
	payload := signedPayload{Method: method, Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

func buildGroupProfilePatch(name string, description string, discoverability string, slug string, goal string, rules string, messagePrompt string, docURL string) map[string]any {
	patch := map[string]any{}
	if value := strings.TrimSpace(name); value != "" {
		patch["display_name"] = value
	}
	if value := strings.TrimSpace(description); value != "" {
		patch["description"] = value
	}
	if value := strings.TrimSpace(discoverability); value != "" {
		patch["discoverability"] = value
	}
	if value := strings.TrimSpace(slug); value != "" {
		patch["slug"] = value
	}
	if value := strings.TrimSpace(goal); value != "" {
		patch["goal"] = value
	}
	if value := strings.TrimSpace(rules); value != "" {
		patch["rules"] = value
	}
	if value := strings.TrimSpace(messagePrompt); value != "" {
		patch["message_prompt"] = value
	}
	if value := strings.TrimSpace(docURL); value != "" {
		patch["doc_url"] = value
	}
	return patch
}

func buildGroupPolicyPatch(admissionMode string, attachmentsAllowed *bool, maxMembers string, memberMaxMessages *int64, memberMaxTotalChars *int64) map[string]any {
	patch := map[string]any{}
	if value := strings.TrimSpace(admissionMode); value != "" {
		patch["admission_mode"] = value
	}
	if attachmentsAllowed != nil {
		patch["attachments_allowed"] = *attachmentsAllowed
	}
	if value := strings.TrimSpace(maxMembers); value != "" {
		patch["max_members"] = value
	}
	if memberMaxMessages != nil {
		patch["member_max_messages"] = *memberMaxMessages
	}
	if memberMaxTotalChars != nil {
		patch["member_max_total_chars"] = *memberMaxTotalChars
	}
	if len(patch) == 0 {
		return patch
	}
	if _, ok := patch["message_security_profile"]; !ok {
		patch["message_security_profile"] = "transport-protected"
	}
	if _, ok := patch["bootstrap_security_profile"]; !ok {
		patch["bootstrap_security_profile"] = "transport-protected"
	}
	if _, ok := patch["permissions"]; !ok {
		patch["permissions"] = map[string]any{
			"send":           "member",
			"add":            "admin",
			"remove":         "admin",
			"update_profile": "admin",
			"update_policy":  "owner",
		}
	}
	return patch
}

func normalizedGroupSecurityProfile(request GroupCreateRequest) string {
	if request.E2EE {
		return GroupE2EESecurityProfile
	}
	switch strings.TrimSpace(request.MessageSecurityProfile) {
	case "", "transport-protected":
		return ""
	case GroupE2EESecurityProfile:
		return GroupE2EESecurityProfile
	default:
		return strings.TrimSpace(request.MessageSecurityProfile)
	}
}

func buildGroupE2EERPCParams(record *identity.StoredIdentity, manager *identity.Manager, targetKind string, targetDID string, method string, body map[string]any, contentType string, operationID string, messageID string, securityProfile string) (map[string]any, error) {
	targetKind = strings.TrimSpace(targetKind)
	targetDID = strings.TrimSpace(targetDID)
	if targetKind == "" {
		targetKind = "group"
	}
	if targetDID == "" {
		return nil, ErrGroupRequired
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	if contentType == "" {
		contentType = "application/json"
	}
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		operationID = "op-" + generateOperationID()
	}
	securityProfile = strings.TrimSpace(securityProfile)
	if securityProfile == "" {
		securityProfile = GroupE2EESecurityProfile
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          GroupE2EEProfile,
		"security_profile": securityProfile,
		"sender_did":       record.DID,
		"target":           map[string]any{"kind": targetKind, "did": targetDID},
		"operation_id":     operationID,
		"created_at":       nowRFC3339(),
		"content_type":     contentType,
	}
	if method == "group.e2ee.send" {
		messageID = strings.TrimSpace(messageID)
		if messageID == "" {
			messageID = "msg-" + generateOperationID()
		}
		meta["message_id"] = messageID
	}
	payload := signedPayload{Method: method, Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

func e2eeHeadBody(groupDID string, memberDID string, mlsHead map[string]any) map[string]any {
	body := map[string]any{
		"group_did": groupDID,
		"group_state_ref": map[string]any{
			"group_did": groupDID,
		},
	}
	for _, key := range []string{"crypto_group_id_b64u", "epoch", "epoch_authenticator", "epoch_authenticator_b64u", "suite", "last_handshake_digest"} {
		if value, ok := mlsHead[key]; ok {
			body[key] = value
		}
	}
	if value, ok := body["epoch_authenticator_b64u"]; ok {
		body["epoch_authenticator"] = value
	}
	if memberDID != "" {
		body["member_did"] = memberDID
		body["subject_did"] = memberDID
	}
	return body
}

func e2eeMembershipCommitBody(groupDID string, subjectDID string, defaultSubjectStatus string, preparedCommit map[string]any) map[string]any {
	body := e2eeHeadBody(groupDID, subjectDID, preparedCommit)
	for _, key := range []string{
		"pending_commit_id",
		"operation_id",
		"commit_b64u",
		"ratchet_tree_b64u",
		"group_info_b64u",
		"from_epoch",
		"to_epoch",
		"actor_did",
		"subject_status",
	} {
		if value, ok := preparedCommit[key]; ok {
			body[key] = value
		}
	}
	if _, ok := body["epoch"]; !ok {
		if value, ok := preparedCommit["to_epoch"]; ok {
			body["epoch"] = value
		}
	}
	if _, ok := body["epoch_authenticator"]; !ok {
		if value, ok := preparedCommit["epoch_authenticator_b64u"]; ok {
			body["epoch_authenticator"] = value
		}
	}
	if _, ok := body["subject_status"]; !ok && defaultSubjectStatus != "" {
		body["subject_status"] = defaultSubjectStatus
	}
	groupStateRef, _ := body["group_state_ref"].(map[string]any)
	if len(groupStateRef) == 0 {
		groupStateRef = map[string]any{"group_did": groupDID}
		body["group_state_ref"] = groupStateRef
	}
	if cryptoGroupID := stringFromAny(body["crypto_group_id_b64u"]); cryptoGroupID != "" {
		groupStateRef["crypto_group_id_b64u"] = cryptoGroupID
	}
	if fromEpoch := stringFromAny(body["from_epoch"]); fromEpoch != "" {
		groupStateRef["epoch"] = fromEpoch
	}
	return body
}

func e2eeRecoveryCommitBody(groupDID string, memberDID string, deviceID string, prepared map[string]any, leasedPackage map[string]any) map[string]any {
	body := map[string]any{
		"group_did": groupDID,
		"group_state_ref": map[string]any{
			"group_did": groupDID,
		},
		"target": map[string]any{
			"agent_did": memberDID,
			"device_id": defaultString(strings.TrimSpace(deviceID), "default"),
		},
	}
	for _, key := range []string{
		"crypto_group_id_b64u",
		"epoch",
		"epoch_authenticator",
		"epoch_authenticator_b64u",
		"suite",
		"last_handshake_digest",
		"pending_commit_id",
		"operation_id",
		"commit_b64u",
		"welcome_b64u",
		"ratchet_tree_b64u",
		"group_info_b64u",
		"from_epoch",
		"to_epoch",
		"old_generation_id",
		"new_generation_id",
	} {
		if value, ok := prepared[key]; ok {
			body[key] = value
		}
	}
	if _, ok := body["epoch"]; !ok {
		if value, ok := prepared["to_epoch"]; ok {
			body["epoch"] = value
		}
	}
	if _, ok := body["epoch_authenticator"]; !ok {
		if value, ok := prepared["epoch_authenticator_b64u"]; ok {
			body["epoch_authenticator"] = value
		}
	}
	keyPackageID := firstNonEmptyString(
		stringFromAny(prepared["recovery_key_package_id"]),
		stringFromAny(prepared["key_package_id"]),
		stringFromAny(leasedPackage["key_package_id"]),
	)
	if keyPackageID != "" {
		body["recovery_key_package_id"] = keyPackageID
	}
	if groupKeyPackage, ok := leasedPackage["group_key_package"].(map[string]any); ok && len(groupKeyPackage) > 0 {
		body["group_key_package"] = sanitizeGroupKeyPackageForService(groupKeyPackage)
	}
	groupStateRef, _ := body["group_state_ref"].(map[string]any)
	if len(groupStateRef) == 0 {
		groupStateRef = map[string]any{"group_did": groupDID}
		body["group_state_ref"] = groupStateRef
	}
	if cryptoGroupID := stringFromAny(body["crypto_group_id_b64u"]); cryptoGroupID != "" {
		groupStateRef["crypto_group_id_b64u"] = cryptoGroupID
	}
	if fromEpoch := stringFromAny(body["from_epoch"]); fromEpoch != "" {
		groupStateRef["epoch"] = fromEpoch
	}
	return body
}

func boolPtr(value bool) *bool {
	return &value
}
