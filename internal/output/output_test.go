package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderSuccessTableUsesCommandDataRows(t *testing.T) {
	t.Parallel()

	envelope := SuccessEnvelope{
		OK:      true,
		Command: "awiki-cli msg inbox",
		Data: map[string]any{
			"messages": []map[string]any{
				{
					"id":   "msg-1",
					"text": "hello",
				},
			},
			"total":  1,
			"source": "http",
		},
		Summary: "Loaded inbox",
		Meta: Meta{
			Version: "test",
			Format:  string(FormatTable),
		},
	}

	var buffer bytes.Buffer
	if err := RenderSuccess(&buffer, FormatTable, "", envelope); err != nil {
		t.Fatalf("RenderSuccess() error = %v", err)
	}

	rendered := buffer.String()
	for _, forbidden := range []string{"command", "meta", "data", "ok", "awiki-cli msg inbox"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("table output %q unexpectedly contains envelope field %q", rendered, forbidden)
		}
	}
	for _, want := range []string{"msg-1", "hello"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("table output %q does not contain row value %q", rendered, want)
		}
	}
}
