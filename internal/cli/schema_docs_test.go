package cli

import (
	"encoding/json"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/cmdmeta"
	docindex "github.com/agentconnect/awiki-cli/internal/docs"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

func TestRunSchemaExposesTopLevelMailCommand(t *testing.T) {
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", t.TempDir())

	app := newSchemaDocsTestApp()
	root := &cobra.Command{Use: "awiki-cli"}
	cmd := &cobra.Command{Use: "schema"}
	root.AddCommand(cmd)

	rendered, err := captureStdout(func() error {
		return app.runSchema(cmd, []string{"mail"})
	})
	if err != nil {
		t.Fatalf("runSchema(mail) error = %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(rendered), &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; output=%q", err, rendered)
	}
	if ok, _ := envelope["ok"].(bool); !ok {
		t.Fatalf("envelope.ok = %v, want true", envelope["ok"])
	}
	if command, _ := envelope["command"].(string); command != "awiki-cli schema" {
		t.Fatalf("envelope.command = %q, want %q", command, "awiki-cli schema")
	}

	data, _ := envelope["data"].(map[string]any)
	commandData, _ := data["command"].(map[string]any)
	if name, _ := commandData["name"].(string); name != "mail" {
		t.Fatalf("data.command.name = %q, want %q", name, "mail")
	}

	children, _ := data["children"].([]any)
	childNames := make(map[string]bool, len(children))
	for _, child := range children {
		item, _ := child.(map[string]any)
		name, _ := item["name"].(string)
		childNames[name] = true
	}
	for _, name := range []string{"mail.inbox", "mail.notify", "mail.read", "mail.send", "mail.attachment"} {
		if !childNames[name] {
			t.Fatalf("schema children missing %q; got=%v", name, childNames)
		}
	}
	if childNames["msg.mail"] {
		t.Fatalf("schema children unexpectedly include legacy msg.mail entry: %v", childNames)
	}
}

func TestRunDocsExposesMailTopic(t *testing.T) {
	t.Setenv("AWIKI_CLI_WORKSPACE_HOME_DIR", t.TempDir())

	app := newSchemaDocsTestApp()
	root := &cobra.Command{Use: "awiki-cli"}
	cmd := &cobra.Command{Use: "docs"}
	root.AddCommand(cmd)

	rendered, err := captureStdout(func() error {
		return app.runDocs(cmd, []string{"mail"})
	})
	if err != nil {
		t.Fatalf("runDocs(mail) error = %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(rendered), &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; output=%q", err, rendered)
	}
	if ok, _ := envelope["ok"].(bool); !ok {
		t.Fatalf("envelope.ok = %v, want true", envelope["ok"])
	}
	if command, _ := envelope["command"].(string); command != "awiki-cli docs" {
		t.Fatalf("envelope.command = %q, want %q", command, "awiki-cli docs")
	}

	data, _ := envelope["data"].(map[string]any)
	topic, _ := data["topic"].(map[string]any)
	if name, _ := topic["name"].(string); name != "mail" {
		t.Fatalf("data.topic.name = %q, want %q", name, "mail")
	}
	references, _ := topic["references"].([]any)
	if len(references) == 0 {
		t.Fatal("data.topic.references = empty, want mail references")
	}
	if first, _ := references[0].(string); first != "docs/architecture/awiki-mail-cli.md" {
		t.Fatalf("data.topic.references[0] = %q, want %q", first, "docs/architecture/awiki-mail-cli.md")
	}
}

func newSchemaDocsTestApp() *App {
	return &App{
		globals: GlobalOptions{Format: string(output.FormatJSON)},
		catalog: cmdmeta.NewCatalog(),
		docs:    docindex.NewIndex(),
	}
}
