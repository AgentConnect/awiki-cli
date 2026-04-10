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

func boolPtr(value bool) *bool {
	return &value
}
