package mail

import (
	"context"
	"errors"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestNewClientRequiresMailServiceURL(t *testing.T) {
	t.Parallel()

	resolved := &appconfig.Resolved{}
	if _, err := NewClient(resolved); err == nil {
		t.Fatal("NewClient() error = nil, want error when mail service url is empty")
	}
}

func TestServiceSendValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	ctx := context.Background()

	// Missing recipient
	if _, err := svc.Send(ctx, SendRequest{}); !errors.Is(err, ErrRecipientRequired) {
		t.Fatalf("Send() error = %v, want %v", err, ErrRecipientRequired)
	}

	// Missing subject
	if _, err := svc.Send(ctx, SendRequest{
		To: []string{"alice@example.com"},
	}); !errors.Is(err, ErrSubjectRequired) {
		t.Fatalf("Send() error = %v, want %v", err, ErrSubjectRequired)
	}

	// Missing body
	if _, err := svc.Send(ctx, SendRequest{
		To:      []string{"alice@example.com"},
		Subject: "Hello",
	}); !errors.Is(err, ErrBodyRequired) {
		t.Fatalf("Send() error = %v, want %v", err, ErrBodyRequired)
	}
}

func TestServiceAttachmentValidatesIndex(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	ctx := context.Background()

	// Negative index should be rejected before any identity or network access.
	if _, err := svc.Attachment(ctx, AttachmentRequest{
		IdentityName:    "",
		MessageID:       "msg-1",
		AttachmentIndex: -1,
	}); !errors.Is(err, ErrAttachmentIndexZero) {
		t.Fatalf("Attachment() error = %v, want %v", err, ErrAttachmentIndexZero)
	}
}

