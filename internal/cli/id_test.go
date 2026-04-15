package cli

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestIdentityMetaFromDataSkipsTypedNilIdentitySummary(t *testing.T) {
	t.Parallel()

	var summary *identity.IdentitySummary
	meta := identityMetaFromData(map[string]any{
		"active_identity": summary,
	})
	if meta != nil {
		t.Fatalf("identityMetaFromData() = %#v, want nil", meta)
	}
}
