package message

import "testing"

func TestSelectAttachmentRPCServiceFromDocumentUsesAttachmentCapableService(t *testing.T) {
	t.Parallel()

	service, err := selectAttachmentRPCServiceFromDocument("did:wba:example.com:user:alice:e1", map[string]any{
		"service": []any{
			map[string]any{
				"id":               "#direct",
				"type":             "ANPMessageService",
				"serviceEndpoint":  "https://example.com/direct/rpc",
				"serviceDid":       "did:wba:example.com",
				"profiles":         []any{"anp.direct.base.v1"},
				"securityProfiles": []any{"transport-protected"},
				"priority":         1,
			},
			map[string]any{
				"id":               "#attachment",
				"type":             "ANPMessageService",
				"serviceEndpoint":  "https://example.com/attachment/rpc",
				"serviceDid":       "did:wba:example.com",
				"profiles":         []any{"anp.attachment.v1"},
				"securityProfiles": []any{"transport-protected"},
				"priority":         7,
			},
		},
	})
	if err != nil {
		t.Fatalf("selectAttachmentRPCServiceFromDocument() error = %v", err)
	}
	if service.RPCEndpoint != "https://example.com/attachment/rpc" {
		t.Fatalf("RPCEndpoint = %q, want attachment endpoint", service.RPCEndpoint)
	}
	if service.ServiceDID != "did:wba:example.com" {
		t.Fatalf("ServiceDID = %q, want service DID", service.ServiceDID)
	}
}

func TestSelectAttachmentRPCServiceFromDocumentUsesLowestPriority(t *testing.T) {
	t.Parallel()

	service, err := selectAttachmentRPCServiceFromDocument("did:wba:example.com:user:alice:e1", map[string]any{
		"service": []any{
			map[string]any{
				"id":               "#secondary",
				"type":             "ANPMessageService",
				"serviceEndpoint":  "https://example.com/secondary/rpc",
				"serviceDid":       "did:wba:example.com",
				"profiles":         []any{"anp.attachment.v1"},
				"securityProfiles": []any{"transport-protected"},
				"priority":         9,
			},
			map[string]any{
				"id":               "#primary",
				"type":             "ANPMessageService",
				"serviceEndpoint":  "https://example.com/primary/rpc",
				"serviceDid":       "did:wba:example.com",
				"profiles":         []any{"anp.attachment.v1"},
				"securityProfiles": []any{"transport-protected"},
				"priority":         2,
			},
		},
	})
	if err != nil {
		t.Fatalf("selectAttachmentRPCServiceFromDocument() error = %v", err)
	}
	if service.RPCEndpoint != "https://example.com/primary/rpc" {
		t.Fatalf("RPCEndpoint = %q, want lowest-priority endpoint", service.RPCEndpoint)
	}
}
