package cli

import (
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
	"github.com/agentconnect/awiki-cli/internal/output"
)

func TestPageDryRunPlansRenderStableContracts(t *testing.T) {
	cases := []struct {
		name        string
		spec        string
		setFlags    map[string]string
		wantSummary string
		wantAction  string
		verifyPlan  func(t *testing.T, plan map[string]any)
	}{
		{
			name:        "page create includes slug title and body bytes",
			spec:        "page.create",
			setFlags:    map[string]string{"slug": "hello", "title": "Hello", "markdown": "body", "visibility": "draft"},
			wantSummary: "Dry run: page create planned",
			wantAction:  "page.create",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request := mustMap(t, plan["request"], "plan.request")
				if request["slug"] != "hello" || request["title"] != "Hello" {
					t.Fatalf("request = %#v, want slug/title", request)
				}
				if request["visibility"] != "draft" || request["body_bytes"] != float64(4) {
					t.Fatalf("request = %#v, want visibility/body_bytes", request)
				}
			},
		},
		{
			name:        "page list exposes rpc metadata",
			spec:        "page.list",
			setFlags:    nil,
			wantSummary: "Dry run: page list planned",
			wantAction:  "page.list",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				if plan["rpc_method"] != "list" {
					t.Fatalf("plan.rpc_method = %#v, want list", plan["rpc_method"])
				}
			},
		},
		{
			name:        "page update reports changed fields",
			spec:        "page.update",
			setFlags:    map[string]string{"slug": "hello", "title": "New Title", "markdown": "updated", "visibility": "public"},
			wantSummary: "Dry run: page update planned",
			wantAction:  "page.update",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				changed, ok := plan["changed_fields"].([]any)
				if !ok || len(changed) != 3 {
					t.Fatalf("plan.changed_fields = %#v, want 3 entries", plan["changed_fields"])
				}
			},
		},
		{
			name:        "page rename uses old and new slug",
			spec:        "page.rename",
			setFlags:    map[string]string{"slug": "old", "to": "new"},
			wantSummary: "Dry run: page rename planned",
			wantAction:  "page.rename",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request := mustMap(t, plan["request"], "plan.request")
				if request["old_slug"] != "old" || request["new_slug"] != "new" {
					t.Fatalf("request = %#v, want old/new slug", request)
				}
			},
		},
		{
			name:        "page delete keeps slug in plan",
			spec:        "page.delete",
			setFlags:    map[string]string{"slug": "old"},
			wantSummary: "Dry run: page delete planned",
			wantAction:  "page.delete",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request := mustMap(t, plan["request"], "plan.request")
				if request["slug"] != "old" {
					t.Fatalf("request.slug = %#v, want old", request["slug"])
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
				case "page.create":
					return app.runPageCreate(cmd, nil)
				case "page.list":
					return app.runPageList(cmd, nil)
				case "page.update":
					return app.runPageUpdate(cmd, nil)
				case "page.rename":
					return app.runPageRename(cmd, nil)
				case "page.delete":
					return app.runPageDelete(cmd, nil)
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

func TestRunPageCreateValidatesRequiredFlagsBeforeService(t *testing.T) {
	t.Parallel()

	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON)}}
	catalog := cmdmeta.NewCatalog()
	cmd := app.commandFromSpec(catalog.MustLookup("page.create"))

	err := app.runPageCreate(cmd, nil)
	if err == nil {
		t.Fatal("runPageCreate() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "slug is required") {
		t.Fatalf("runPageCreate() error = %v, want slug validation", err)
	}
}

func mustMap(t *testing.T, value any, label string) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want map[string]any", label, value)
	}
	return result
}
