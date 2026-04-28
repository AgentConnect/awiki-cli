package cli

import (
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
	"github.com/agentconnect/awiki-cli/internal/output"
)

func TestMsgDryRunPlansRenderStableContracts(t *testing.T) {
	cases := []struct {
		name        string
		spec        string
		args        []string
		setFlags    map[string]string
		wantSummary string
		wantAction  string
		verifyPlan  func(t *testing.T, plan map[string]any)
	}{
		{
			name:        "direct send reports transport and local writes",
			spec:        "msg.send",
			setFlags:    map[string]string{"to": "bob", "text": "hello"},
			wantSummary: "Dry run: message send planned",
			wantAction:  "direct.send",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				target := mustMap(t, plan["target"], "plan.target")
				if target["kind"] != "direct" || target["did"] != "bob" {
					t.Fatalf("target = %#v, want direct bob", target)
				}
				if target["handle"] != "bob.awiki.ai" {
					t.Fatalf("target.handle = %#v, want bob.awiki.ai", target["handle"])
				}
				if plan["message_type"] != "text" {
					t.Fatalf("plan.message_type = %#v, want text", plan["message_type"])
				}
			},
		},
		{
			name:        "attachment send forces http transport",
			spec:        "msg.send",
			setFlags:    map[string]string{"to": "bob", "text": "caption", "file": "/tmp/demo.txt", "mime-type": "text/plain"},
			wantSummary: "Dry run: message send planned",
			wantAction:  "attachment.send",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["transport"] != "http" {
					t.Fatalf("plan.transport = %#v, want http", plan["transport"])
				}
				attachment := mustMap(t, plan["attachment"], "plan.attachment")
				if attachment["caption"] != "caption" || attachment["mime_type"] != "text/plain" {
					t.Fatalf("attachment = %#v, want caption and mime_type", attachment)
				}
			},
		},
		{
			name:        "history carries cursor and limit",
			spec:        "msg.history",
			setFlags:    map[string]string{"with": "bob", "limit": "15", "cursor": "seq-2"},
			wantSummary: "Dry run: direct history read planned",
			wantAction:  "direct.get_history",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["with"] != "bob" || plan["cursor"] != "seq-2" || plan["limit"] != float64(15) {
					t.Fatalf("plan = %#v, want with/cursor/limit", plan)
				}
				if plan["with_handle"] != "bob.awiki.ai" {
					t.Fatalf("plan.with_handle = %#v, want bob.awiki.ai", plan["with_handle"])
				}
			},
		},
		{
			name:        "mark-read keeps message ids",
			spec:        "msg.mark-read",
			args:        []string{"msg-1", "msg-2"},
			wantSummary: "Dry run: mark-read planned",
			wantAction:  "inbox.mark_read",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				ids, ok := plan["message_ids"].([]any)
				if !ok || len(ids) != 2 {
					t.Fatalf("plan.message_ids = %#v, want 2 ids", plan["message_ids"])
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", workspace)

			catalog := cmdmeta.NewCatalog()
			app := &App{catalog: catalog, globals: GlobalOptions{DryRun: true, Format: string(output.FormatJSON), Identity: "alice"}}
			cmd := app.commandFromSpec(catalog.MustLookup(tc.spec))
			for key, value := range tc.setFlags {
				if err := cmd.Flags().Set(key, value); err != nil {
					t.Fatalf("Flags().Set(%s) error = %v", key, err)
				}
			}

			rendered, err := captureStdout(func() error {
				switch tc.spec {
				case "msg.send":
					return app.runMsgSend(cmd, nil)
				case "msg.history":
					return app.runMsgHistory(cmd, nil)
				case "msg.mark-read":
					return app.runMsgMarkRead(cmd, tc.args)
				default:
					t.Fatalf("unsupported spec %q", tc.spec)
					return nil
				}
			})
			if err != nil {
				t.Fatalf("run dry-run command error = %v", err)
			}

			envelope := decodeSuccessEnvelope(t, rendered)
			if envelope.Summary != tc.wantSummary {
				t.Fatalf("summary = %q, want %q", envelope.Summary, tc.wantSummary)
			}
			plan := mustMap(t, envelope.Data["plan"], "data.plan")
			if plan["action"] != tc.wantAction {
				t.Fatalf("plan.action = %#v, want %q", plan["action"], tc.wantAction)
			}
			if tc.verifyPlan != nil {
				tc.verifyPlan(t, plan)
			}
		})
	}
}

func TestRunMsgSendRejectsInvalidFlagCombinationsBeforeService(t *testing.T) {
	t.Parallel()

	catalog := cmdmeta.NewCatalog()
	app := &App{catalog: catalog, globals: GlobalOptions{Format: string(output.FormatJSON)}}

	cmd := app.commandFromSpec(catalog.MustLookup("msg.send"))
	if err := cmd.Flags().Set("to", "bob"); err != nil {
		t.Fatalf("Flags().Set(to) error = %v", err)
	}
	if err := cmd.Flags().Set("group", "did:group"); err != nil {
		t.Fatalf("Flags().Set(group) error = %v", err)
	}
	if err := cmd.Flags().Set("text", "hello"); err != nil {
		t.Fatalf("Flags().Set(text) error = %v", err)
	}
	if err := app.runMsgSend(cmd, nil); err == nil || !strings.Contains(err.Error(), "either --to or --group") {
		t.Fatalf("runMsgSend() error = %v, want mutually exclusive target error", err)
	}

	cmd = app.commandFromSpec(catalog.MustLookup("msg.send"))
	if err := cmd.Flags().Set("to", "bob"); err != nil {
		t.Fatalf("Flags().Set(to) error = %v", err)
	}
	if err := cmd.Flags().Set("mime-type", "text/plain"); err != nil {
		t.Fatalf("Flags().Set(mime-type) error = %v", err)
	}
	if err := app.runMsgSend(cmd, nil); err == nil || !strings.Contains(err.Error(), "attachment file") {
		t.Fatalf("runMsgSend() error = %v, want attachment file validation", err)
	}
}
