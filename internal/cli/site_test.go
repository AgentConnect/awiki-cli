package cli

import (
	"errors"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
	"github.com/agentconnect/awiki-cli/internal/output"
	awikisite "github.com/agentconnect/awiki-cli/internal/site"
)

func TestSiteDryRunPlansRenderStableContracts(t *testing.T) {
	cases := []struct {
		name        string
		spec        string
		setFlags    map[string]string
		wantSummary string
		wantAction  string
		verifyPlan  func(t *testing.T, plan map[string]any)
	}{
		{
			name:        "site root set includes domain and body bytes",
			spec:        "site.root.set",
			setFlags:    map[string]string{"domain": "tenant.example", "markdown": "body"},
			wantSummary: "Dry run: site root set planned",
			wantAction:  "site.root.set",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request := mustMap(t, plan["request"], "plan.request")
				if request["domain"] != "tenant.example" || request["body_bytes"] != float64(4) {
					t.Fatalf("request = %#v, want domain/body_bytes", request)
				}
			},
		},
		{
			name:        "site page rename uses old and new slug",
			spec:        "site.page.rename",
			setFlags:    map[string]string{"domain": "tenant.example", "slug": "old", "to": "new"},
			wantSummary: "Dry run: site page rename planned",
			wantAction:  "site.page.rename",
			verifyPlan: func(t *testing.T, plan map[string]any) {
				request := mustMap(t, plan["request"], "plan.request")
				if request["old_slug"] != "old" || request["new_slug"] != "new" {
					t.Fatalf("request = %#v, want old/new slug", request)
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
				case "site.root.set":
					return app.runSiteRootSet(cmd, nil)
				case "site.page.rename":
					return app.runSitePageRename(cmd, nil)
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

func TestSiteExitMapsForbiddenRPCCode(t *testing.T) {
	t.Parallel()

	app := &App{}
	err := app.siteExit(&awikisite.ServiceError{RPCCode: -32001, Message: "forbidden"}, "hint")
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("errors.As(%T, *output.ExitError) = false", err)
	}
	if exitErr.Detail.Code != "forbidden" {
		t.Fatalf("exitErr.Detail.Code = %q, want forbidden", exitErr.Detail.Code)
	}
	if exitErr.Code != 4 {
		t.Fatalf("exitErr.Code = %d, want 4", exitErr.Code)
	}
}

func TestSiteExitMapsBusinessErrorRPCCode(t *testing.T) {
	t.Parallel()

	app := &App{}
	err := app.siteExit(&awikisite.ServiceError{RPCCode: -32004, Message: "invalid slug"}, "hint")
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("errors.As(%T, *output.ExitError) = false", err)
	}
	if exitErr.Detail.Code != "invalid_argument" {
		t.Fatalf("exitErr.Detail.Code = %q, want invalid_argument", exitErr.Detail.Code)
	}
	if exitErr.Code != 2 {
		t.Fatalf("exitErr.Code = %d, want 2", exitErr.Code)
	}
}

func TestRunSiteRootSetRequiresExplicitBodySource(t *testing.T) {
	t.Parallel()

	app := &App{globals: GlobalOptions{Format: string(output.FormatJSON)}}
	catalog := cmdmeta.NewCatalog()
	cmd := app.commandFromSpec(catalog.MustLookup("site.root.set"))
	_ = cmd.Flags().Set("domain", "tenant.example")

	err := app.runSiteRootSet(cmd, nil)
	if err == nil {
		t.Fatal("runSiteRootSet() error = nil, want validation error")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("errors.As(%T, *output.ExitError) = false", err)
	}
	if exitErr.Detail.Code != "invalid_argument" {
		t.Fatalf("exitErr.Detail.Code = %q, want invalid_argument", exitErr.Detail.Code)
	}
	if exitErr.Detail.Hint != "Provide --markdown or --markdown-file." {
		t.Fatalf("exitErr.Detail.Hint = %q, want explicit body source hint", exitErr.Detail.Hint)
	}
}
