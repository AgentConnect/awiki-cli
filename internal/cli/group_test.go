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
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group", "member": "bob", "e2ee": "true"},
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
				if request["E2EE"] != true {
					t.Fatalf("request.E2EE = %#v, want true", request["E2EE"])
				}
				if plan["member_handle"] != "bob.awiki.ai" {
					t.Fatalf("plan.member_handle = %#v, want bob.awiki.ai", plan["member_handle"])
				}
			},
		},

		{
			name:        "group leave e2ee plans hidden leave request",
			spec:        "group.leave",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group", "e2ee": "true", "reason": "done"},
			wantSummary: "Dry run: group leave planned",
			wantAction:  "group.leave",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request, ok := plan["request"].(map[string]any)
				if !ok {
					t.Fatalf("plan.request type = %T, want map[string]any", plan["request"])
				}
				if request["E2EE"] != true || request["ReasonText"] != "done" {
					t.Fatalf("request = %#v, want E2EE leave request plan", request)
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
			name:        "group e2ee status exposes exec provider data dir without advertising discovery",
			spec:        "group.e2ee.status",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group"},
			wantSummary: "Dry run: group e2ee status planned",
			wantAction:  "group.e2ee.status",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["provider"] != "exec" {
					t.Fatalf("plan.provider = %#v, want exec", plan["provider"])
				}
				if plan["discovery_advertised"] != false {
					t.Fatalf("plan.discovery_advertised = %#v, want false", plan["discovery_advertised"])
				}
				if _, ok := plan["artifact_mode"]; ok {
					t.Fatalf("plan.artifact_mode should not be exposed by real MLS status diagnostics: %#v", plan)
				}
				if plan["mls_data_dir"] == "" {
					t.Fatal("plan.mls_data_dir should be populated")
				}
			},
		},
		{
			name:        "group e2ee pending plans P6 notice pull",
			spec:        "group.e2ee.pending",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group"},
			wantSummary: "Dry run: group e2ee pending planned",
			wantAction:  "group.e2ee.pending",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["provider"] != "exec" {
					t.Fatalf("plan.provider = %#v, want exec", plan["provider"])
				}
				if plan["group"] != "did:wba:example.com:groups:demo:e1_group" {
					t.Fatalf("plan.group = %#v, want group DID", plan["group"])
				}
			},
		},
		{
			name:        "group e2ee repair plans P6 notice replay",
			spec:        "group.e2ee.repair",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group"},
			wantSummary: "Dry run: group e2ee repair planned",
			wantAction:  "group.e2ee.repair",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["provider"] != "exec" {
					t.Fatalf("plan.provider = %#v, want exec", plan["provider"])
				}
				if plan["scope"] == "" {
					t.Fatalf("plan.scope should describe durable notice replay: %#v", plan)
				}
			},
		},

		{
			name:        "group e2ee publish update key package plans purpose update",
			spec:        "group.e2ee.publish-key-package",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group", "purpose": "update", "device": "bob-main"},
			wantSummary: "Dry run: group e2ee key package publish planned",
			wantAction:  "group.e2ee.publish_key_package",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["purpose"] != "update" {
					t.Fatalf("plan.purpose = %#v, want update", plan["purpose"])
				}
				if plan["recovery"] != false {
					t.Fatalf("plan.recovery = %#v, want false", plan["recovery"])
				}
			},
		},
		{
			name:        "group e2ee update key plans hidden owner controlled update",
			spec:        "group.e2ee.update-key",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group", "member": "bob", "device": "bob-main"},
			wantSummary: "Dry run: group e2ee update-key planned",
			wantAction:  "group.e2ee.update_key",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["key_package_purpose"] != "update" || plan["p4_membership_mutate"] != false {
					t.Fatalf("plan = %#v, want purpose=update without P4 mutation", plan)
				}
				if plan["hidden_awiki_extension"] != true {
					t.Fatalf("plan.hidden_awiki_extension = %#v, want true", plan["hidden_awiki_extension"])
				}
			},
		},
		{
			name:        "group e2ee rejoin plans canonical group add e2ee path",
			spec:        "group.e2ee.rejoin",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group", "member": "bob"},
			wantSummary: "Dry run: group e2ee rejoin planned",
			wantAction:  "group.e2ee.rejoin",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["canonical_command"] != "group add --e2ee" {
					t.Fatalf("plan.canonical_command = %#v, want group add --e2ee", plan["canonical_command"])
				}
				if plan["key_package_purpose"] != "normal" || plan["external_commit"] != false {
					t.Fatalf("plan = %#v, want normal package and no External Commit", plan)
				}
			},
		},

		{
			name:        "group e2ee process leave request plans owner remove",
			spec:        "group.e2ee.process-leave-request",
			setFlags:    map[string]string{"group": "did:wba:example.com:groups:demo:e1_group", "member": "bob", "leave-request-id": "lr-bob-1"},
			wantSummary: "Dry run: group e2ee leave request process planned",
			wantAction:  "group.e2ee.process_leave_request",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["member"] != "bob" || plan["leave_request_id"] != "lr-bob-1" {
					t.Fatalf("plan = %#v, want member and leave request id", plan)
				}
			},
		},
		{
			name:        "group create e2ee alias maps to group-e2ee request",
			spec:        "group.create",
			setFlags:    map[string]string{"name": "Secret Group", "e2ee": "true"},
			wantSummary: "Dry run: group create planned",
			wantAction:  "group.create",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request, ok := plan["request"].(map[string]any)
				if !ok {
					t.Fatalf("plan.request type = %T, want map[string]any", plan["request"])
				}
				if request["E2EE"] != true {
					t.Fatalf("request.E2EE = %#v, want true", request["E2EE"])
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
				case "group.leave":
					return app.runGroupLeave(cmd, nil)
				case "group.e2ee.status":
					return app.runGroupE2EEStatus(cmd, nil)
				case "group.e2ee.pending":
					return app.runGroupE2EEPending(cmd, nil)
				case "group.e2ee.repair":
					return app.runGroupE2EERepair(cmd, nil)
				case "group.e2ee.publish-key-package":
					return app.runGroupE2EEPublishKeyPackage(cmd, nil)
				case "group.e2ee.update-key":
					return app.runGroupE2EEUpdateKey(cmd, nil)
				case "group.e2ee.rejoin":
					return app.runGroupE2EERejoin(cmd, nil)
				case "group.e2ee.process-leave-request":
					return app.runGroupE2EEProcessLeaveRequest(cmd, nil)
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
