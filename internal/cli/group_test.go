package cli

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
	"github.com/agentconnect/awiki-cli/internal/output"
)

func TestGroupDryRunPlansRenderStableContracts(t *testing.T) {
	cases := []struct {
		name        string
		spec        string
		setFlags    map[string]string
		wantSummary string
		wantAction  string
		verifyPlan  func(t *testing.T, plan map[string]any)
	}{
		{
			name:        "group create includes slug in request plan",
			spec:        "group.create",
			setFlags:    map[string]string{"name": "Demo Group", "slug": "demo-group"},
			wantSummary: "Dry run: group create planned",
			wantAction:  "group.create",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request, ok := plan["request"].(map[string]any)
				if !ok {
					t.Fatalf("plan.request type = %T, want map[string]any", plan["request"])
				}
				if request["Slug"] != "demo-group" {
					t.Fatalf("request.Slug = %#v, want %q", request["Slug"], "demo-group")
				}
				if request["Name"] != "Demo Group" {
					t.Fatalf("request.Name = %#v, want %q", request["Name"], "Demo Group")
				}
			},
		},
		{
			name:        "group remove uses kick action and member",
			spec:        "group.remove",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group", "member": "bob"},
			wantSummary: "Dry run: group membership change planned",
			wantAction:  "group.kick",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request, ok := plan["request"].(map[string]any)
				if !ok {
					t.Fatalf("plan.request type = %T, want map[string]any", plan["request"])
				}
				if request["Member"] != "bob" {
					t.Fatalf("request.Member = %#v, want %q", request["Member"], "bob")
				}
				if plan["member_handle"] != "bob.awiki.ai" {
					t.Fatalf("plan.member_handle = %#v, want bob.awiki.ai", plan["member_handle"])
				}
			},
		},
		{
			name:        "group messages includes cursor and limit",
			spec:        "group.messages",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group", "limit": "25", "cursor": "42"},
			wantSummary: "Dry run: group messages planned",
			wantAction:  "group.list_messages",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request, ok := plan["request"].(map[string]any)
				if !ok {
					t.Fatalf("plan.request type = %T, want map[string]any", plan["request"])
				}
				if request["Cursor"] != "42" {
					t.Fatalf("request.Cursor = %#v, want %q", request["Cursor"], "42")
				}
				if request["Limit"] != float64(25) {
					t.Fatalf("request.Limit = %#v, want 25", request["Limit"])
				}
			},
		},
		{
			name:        "group list includes limit",
			spec:        "group.list",
			setFlags:    map[string]string{"limit": "25"},
			wantSummary: "Dry run: group list planned",
			wantAction:  "group.list",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request, ok := plan["request"].(map[string]any)
				if !ok {
					t.Fatalf("plan.request type = %T, want map[string]any", plan["request"])
				}
				if request["Limit"] != float64(25) {
					t.Fatalf("request.Limit = %#v, want 25", request["Limit"])
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
				case "group.create":
					return app.runGroupCreate(cmd, nil)
				case "group.remove":
					return app.runGroupKick(cmd, nil)
				case "group.messages":
					return app.runGroupMessages(cmd, nil)
				case "group.list":
					return app.runGroupList(cmd, nil)
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
			if envelope.Meta["dry_run"] != true {
				t.Fatalf("meta.dry_run = %#v, want true", envelope.Meta["dry_run"])
			}
			plan, ok := envelope.Data["plan"].(map[string]any)
			if !ok {
				t.Fatalf("data.plan type = %T, want map[string]any", envelope.Data["plan"])
			}
			if plan["action"] != tc.wantAction {
				t.Fatalf("plan.action = %#v, want %q", plan["action"], tc.wantAction)
			}
			if plan["identity"] != "alice" {
				t.Fatalf("plan.identity = %#v, want %q", plan["identity"], "alice")
			}
			if tc.verifyPlan != nil {
				tc.verifyPlan(t, plan)
			}
		})
	}
}
