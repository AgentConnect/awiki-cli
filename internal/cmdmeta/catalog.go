package cmdmeta

import (
	"fmt"
	"sort"
	"strings"
)

type FlagSpec struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Usage      string   `json:"usage"`
	Default    string   `json:"default,omitempty"`
	Required   bool     `json:"required,omitempty"`
	Choices    []string `json:"choices,omitempty"`
	Deprecated bool     `json:"deprecated,omitempty"`
}

type CommandSpec struct {
	Name        string     `json:"name"`
	Use         string     `json:"use"`
	Short       string     `json:"short"`
	Long        string     `json:"long,omitempty"`
	Aliases     []string   `json:"aliases,omitempty"`
	Phase       string     `json:"phase"`
	Hidden      bool       `json:"hidden,omitempty"`
	Implemented bool       `json:"implemented"`
	Handler     string     `json:"handler,omitempty"`
	SideEffect  bool       `json:"side_effect"`
	Outputs     []string   `json:"outputs,omitempty"`
	Flags       []FlagSpec `json:"flags,omitempty"`
}

type Catalog struct {
	specs []CommandSpec
	index map[string]CommandSpec
}

func NewCatalog() *Catalog {
	specs := defaultSpecs()
	index := make(map[string]CommandSpec, len(specs))
	for _, spec := range specs {
		index[normalizeName(spec.Name)] = spec
	}
	return &Catalog{specs: specs, index: index}
}

func (c *Catalog) Specs() []CommandSpec {
	if c == nil {
		return nil
	}
	return append([]CommandSpec(nil), c.specs...)
}

func (c *Catalog) Lookup(raw string) (CommandSpec, bool) {
	if c == nil {
		return CommandSpec{}, false
	}
	name := normalizeName(raw)
	spec, ok := c.index[name]
	return spec, ok
}

func (c *Catalog) MustLookup(raw string) CommandSpec {
	spec, ok := c.Lookup(raw)
	if !ok {
		panic(fmt.Sprintf("command metadata not found: %s", raw))
	}
	return spec
}

func (c *Catalog) ChildrenOf(parent string) []CommandSpec {
	if c == nil {
		return nil
	}
	needle := normalizeName(parent)
	children := make([]CommandSpec, 0)
	for _, spec := range c.specs {
		if parentName(spec.Name) == needle {
			children = append(children, spec)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	return children
}

func normalizeName(raw string) string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "awiki-cli"))
	trimmed = strings.TrimSpace(trimmed)
	trimmed = strings.ReplaceAll(trimmed, " ", ".")
	trimmed = strings.Trim(trimmed, ".")
	return strings.ToLower(trimmed)
}

func parentName(name string) string {
	normalized := normalizeName(name)
	last := strings.LastIndex(normalized, ".")
	if last < 0 {
		return ""
	}
	return normalized[:last]
}

func defaultSpecs() []CommandSpec {
	return []CommandSpec{
		{Name: "status", Use: "status", Short: "Show the current phase-1 CLI status", Phase: "phase1", Implemented: true, Handler: "status", Outputs: []string{"json", "pretty", "table"}},
		{Name: "docs", Use: "docs [topic]", Short: "Show built-in documentation topics", Phase: "phase1", Implemented: true, Handler: "docs", Outputs: []string{"json", "pretty", "table"}},
		{Name: "schema", Use: "schema [command]", Short: "Show the static command contract", Phase: "phase1", Implemented: true, Handler: "schema", Outputs: []string{"json", "pretty", "table"}},
		{Name: "doctor", Use: "doctor", Short: "Run baseline environment and storage diagnostics", Phase: "phase1", Implemented: true, Handler: "doctor", Outputs: []string{"json", "pretty", "table"}},
		{Name: "version", Use: "version", Short: "Show build information", Phase: "phase1", Implemented: true, Handler: "version", Outputs: []string{"json", "pretty", "table"}},
		{Name: "upgrade", Use: "upgrade", Short: "Check for newer awiki-cli versions and show upgrade hints", Phase: "phase2", Implemented: true, Handler: "upgrade", Outputs: []string{"json", "pretty", "table"}},
		{Name: "init", Use: "init", Short: "Initialize the awiki-cli workspace and config.yaml", Phase: "phase1", Implemented: true, Handler: "init", SideEffect: true, Outputs: []string{"json", "pretty", "table"}},
		{Name: "completion", Use: "completion", Short: "Generate shell completion scripts", Phase: "phase1", Implemented: true},
		{Name: "completion.bash", Use: "bash", Short: "Generate Bash completion", Phase: "phase1", Implemented: true, Handler: "completion.bash"},
		{Name: "completion.zsh", Use: "zsh", Short: "Generate Zsh completion", Phase: "phase1", Implemented: true, Handler: "completion.zsh"},
		{Name: "completion.fish", Use: "fish", Short: "Generate Fish completion", Phase: "phase1", Implemented: true, Handler: "completion.fish"},
		{Name: "completion.powershell", Use: "powershell", Short: "Generate PowerShell completion", Phase: "phase1", Implemented: true, Handler: "completion.powershell"},
		{Name: "config", Use: "config", Short: "Inspect resolved CLI configuration", Phase: "phase1", Implemented: true},
		{Name: "config.show", Use: "show", Short: "Show resolved configuration values", Phase: "phase1", Implemented: true, Handler: "config.show", Outputs: []string{"json", "pretty", "table"}},
		{Name: "id", Use: "id", Short: "Identity lifecycle commands", Phase: "phase1", Implemented: true},
		{Name: "id.status", Use: "status", Short: "Show identity status", Phase: "phase2", Implemented: true, Handler: "id.status", Outputs: []string{"json", "pretty"}},
		{Name: "id.create", Use: "create", Short: "Create local DID material for bootstrap or migration", Phase: "phase2", Hidden: true, Implemented: true, Handler: "id.create", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "name", Type: "string", Usage: "Identity display name", Required: true}, {Name: "identity", Type: "string", Usage: "Identity alias override"}}},
		{Name: "id.register", Use: "register", Short: "Register a handle-backed user identity", Phase: "phase3", Implemented: true, Handler: "id.register", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "handle", Type: "string", Usage: "Handle local part", Required: true}, {Name: "phone", Type: "string", Usage: "Phone number for registration"}, {Name: "email", Type: "string", Usage: "Email address for registration"}, {Name: "otp", Type: "string", Usage: "Verification code"}, {Name: "invite-code", Type: "string", Usage: "Invite code if required"}, {Name: "wait", Type: "bool", Usage: "Wait for email verification before completing registration"}}},
		{Name: "id.bind", Use: "bind", Short: "Bind phone or email to the current identity", Phase: "phase3", Implemented: true, Handler: "id.bind", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "phone", Type: "string", Usage: "Phone number to bind"}, {Name: "email", Type: "string", Usage: "Email address to bind"}, {Name: "otp", Type: "string", Usage: "Verification code"}, {Name: "wait", Type: "bool", Usage: "Wait for email verification before completing the bind"}}},
		{Name: "id.resolve", Use: "resolve", Short: "Resolve a DID or handle", Phase: "phase3", Implemented: true, Handler: "id.resolve", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "handle", Type: "string", Usage: "Handle to resolve"}, {Name: "did", Type: "string", Usage: "DID to resolve"}}},
		{Name: "id.recover", Use: "recover", Short: "Recover a handle with phone verification", Phase: "phase3", Implemented: true, Handler: "id.recover", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "handle", Type: "string", Usage: "Handle local part", Required: true}, {Name: "phone", Type: "string", Usage: "Recovery phone number", Required: true}, {Name: "otp", Type: "string", Usage: "Verification code"}}},
		{Name: "id.replace-did", Use: "replace-did", Short: "Replace the current handle DID with a new e1 DID", Phase: "phase3", Hidden: true, Implemented: true, Handler: "id.replace-did", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "is-public", Type: "bool", Usage: "Override the public visibility flag"}, {Name: "is-agent", Type: "bool", Usage: "Override the agent flag"}, {Name: "role", Type: "string", Usage: "Override the role value; pass an empty string to clear it"}, {Name: "endpoint-url", Type: "string", Usage: "Override the endpoint URL; pass an empty string to clear it"}}},
		{Name: "id.list", Use: "list", Short: "List local identities", Phase: "phase2", Implemented: true, Handler: "id.list", Outputs: []string{"json", "pretty", "table"}},
		{Name: "id.current", Use: "current", Short: "Show the default identity", Phase: "phase2", Implemented: true, Handler: "id.current", Outputs: []string{"json", "pretty", "table"}},
		{Name: "id.use", Use: "use <identity>", Short: "Switch the default identity", Phase: "phase2", Implemented: true, Handler: "id.use", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "id.profile", Use: "profile", Short: "Read or update DID profile data", Phase: "phase3", Implemented: true},
		{Name: "id.profile.get", Use: "get", Short: "Get DID profile data", Phase: "phase3", Implemented: true, Handler: "id.profile.get", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "self", Type: "bool", Usage: "Read the active identity profile"}, {Name: "handle", Type: "string", Usage: "Read a profile by handle"}, {Name: "did", Type: "string", Usage: "Read a profile by DID"}}},
		{Name: "id.profile.set", Use: "set", Short: "Update DID profile data", Phase: "phase3", Implemented: true, Handler: "id.profile.set", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "display-name", Type: "string", Usage: "Profile display name"}, {Name: "bio", Type: "string", Usage: "Profile bio"}, {Name: "tags", Type: "string", Usage: "Comma-separated tags"}, {Name: "markdown", Type: "string", Usage: "Inline markdown body"}, {Name: "markdown-file", Type: "string", Usage: "Markdown file path"}}},
		{Name: "id.import-v1", Use: "import-v1", Short: "Import credentials from the v1 awiki-agent-id-message layout", Phase: "phase2", Implemented: true, Handler: "id.import-v1", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "name", Type: "string", Usage: "Import one legacy identity by name"}, {Name: "all", Type: "bool", Usage: "Import all detected legacy identities"}}},
		{Name: "msg", Use: "msg", Short: "Messaging commands", Phase: "phase1", Implemented: true},
		{Name: "msg.send", Use: "send", Short: "Send a direct or group message", Phase: "phase5", Implemented: true, Handler: "msg.send", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "to", Type: "string", Usage: "Direct message target"}, {Name: "group", Type: "string", Usage: "Group target"}, {Name: "text", Type: "string", Usage: "Inline message text or attachment caption"}, {Name: "text-file", Type: "string", Usage: "Message body or attachment caption file path"}, {Name: "file", Type: "string", Usage: "Attachment file path"}, {Name: "mime-type", Type: "string", Usage: "Attachment MIME type override"}, {Name: "type", Type: "string", Usage: "Message type", Default: "text"}, {Name: "secure", Type: "string", Usage: "Secure mode", Default: "off", Choices: []string{"off", "on"}}}},
		{Name: "msg.attachment", Use: "attachment", Short: "Attachment commands", Phase: "phase5", Implemented: true},
		{Name: "msg.attachment.download", Use: "download", Short: "Download one attachment from a direct or group message", Phase: "phase5", Implemented: true, Handler: "msg.attachment.download", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "with", Type: "string", Usage: "Direct peer DID or handle"}, {Name: "group", Type: "string", Usage: "Group DID"}, {Name: "message-id", Type: "string", Usage: "Visible message id or raw message_id", Required: true}, {Name: "attachment-id", Type: "string", Usage: "Attachment id when the message contains multiple attachments"}, {Name: "output", Type: "string", Usage: "Output file path", Required: true}}},
		{Name: "msg.inbox", Use: "inbox", Short: "Read inbox messages", Phase: "phase5", Implemented: true, Handler: "msg.inbox", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "scope", Type: "string", Usage: "Message scope", Default: "all", Choices: []string{"all", "direct", "group"}}, {Name: "with", Type: "string", Usage: "Direct peer filter"}, {Name: "group", Type: "string", Usage: "Group filter"}, {Name: "unread", Type: "bool", Usage: "Only unread messages"}, {Name: "limit", Type: "int", Usage: "Maximum number of results", Default: "20"}, {Name: "mark-read", Type: "bool", Usage: "Mark returned messages as read"}}},
		{Name: "msg.history", Use: "history", Short: "Read message history", Phase: "phase5", Implemented: true, Handler: "msg.history", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "with", Type: "string", Usage: "Direct peer DID or handle", Required: true}, {Name: "limit", Type: "int", Usage: "Maximum number of rows", Default: "50"}, {Name: "cursor", Type: "string", Usage: "Pagination cursor"}}},
		{Name: "msg.mark-read", Use: "mark-read [MESSAGE_ID...]", Short: "Mark messages as read", Phase: "phase5", Implemented: true, Handler: "msg.mark-read", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "msg.secure", Use: "secure", Short: "Secure direct messaging commands", Phase: "phase5", Implemented: false},
		{Name: "msg.secure.status", Use: "status", Short: "Inspect secure messaging status", Phase: "phase5", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "with", Type: "string", Usage: "Target peer DID or handle"}}},
		{Name: "msg.secure.init", Use: "init", Short: "Initialize a secure session", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "with", Type: "string", Usage: "Target peer DID or handle", Required: true}}},
		{Name: "msg.secure.repair", Use: "repair", Short: "Repair a secure session", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "with", Type: "string", Usage: "Target peer DID or handle", Required: true}}},
		{Name: "msg.secure.failed", Use: "failed", Short: "List failed secure outbox items", Phase: "phase5", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "msg.secure.retry", Use: "retry <OUTBOX_ID>", Short: "Retry one failed secure outbox item", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "msg.secure.drop", Use: "drop <OUTBOX_ID>", Short: "Drop one failed secure outbox item", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "group", Use: "group", Short: "Group lifecycle commands", Phase: "phase1", Implemented: true},
		{Name: "group.create", Use: "create", Short: "Create a new group", Phase: "phase5", Implemented: true, Handler: "group.create", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "name", Type: "string", Usage: "Group display name", Required: true}, {Name: "description", Type: "string", Usage: "Group description"}, {Name: "discoverability", Type: "string", Usage: "Discoverability mode", Default: "private"}, {Name: "admission-mode", Type: "string", Usage: "Admission mode", Default: "open-join"}, {Name: "slug", Type: "string", Usage: "Group slug"}, {Name: "goal", Type: "string", Usage: "Group goal"}, {Name: "rules", Type: "string", Usage: "Group rules"}, {Name: "message-prompt", Type: "string", Usage: "Default group prompt"}, {Name: "doc-url", Type: "string", Usage: "Group document URL"}, {Name: "attachments-allowed", Type: "bool", Usage: "Allow attachments"}, {Name: "max-members", Type: "string", Usage: "Maximum group members"}, {Name: "member-max-messages", Type: "int", Usage: "Per-member message limit"}, {Name: "member-max-total-chars", Type: "int", Usage: "Per-member total char limit"}}},
		{Name: "group.get", Use: "get", Short: "Show group details", Aliases: []string{"show"}, Phase: "phase5", Implemented: true, Handler: "group.get", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}}},
		{Name: "group.join", Use: "join", Short: "Join an open group", Phase: "phase5", Implemented: true, Handler: "group.join", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}, {Name: "reason", Type: "string", Usage: "Join reason"}}},
		{Name: "group.add", Use: "add", Short: "Add a member to a group", Phase: "phase5", Implemented: true, Handler: "group.add", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}, {Name: "member", Type: "string", Usage: "Member DID or handle", Required: true}, {Name: "role", Type: "string", Usage: "Member role", Default: "member"}}},
		{Name: "group.remove", Use: "remove", Short: "Remove a member from a group", Aliases: []string{"kick"}, Phase: "phase5", Implemented: true, Handler: "group.remove", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}, {Name: "member", Type: "string", Usage: "Member DID or handle", Required: true}, {Name: "reason", Type: "string", Usage: "Removal reason"}}},
		{Name: "group.leave", Use: "leave", Short: "Leave a group", Phase: "phase5", Implemented: true, Handler: "group.leave", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}}},
		{Name: "group.update", Use: "update", Short: "Update group profile or policy", Phase: "phase5", Implemented: true, Handler: "group.update", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}, {Name: "name", Type: "string", Usage: "New group display name"}, {Name: "description", Type: "string", Usage: "New group description"}, {Name: "discoverability", Type: "string", Usage: "Discoverability mode"}, {Name: "admission-mode", Type: "string", Usage: "Admission mode"}, {Name: "slug", Type: "string", Usage: "New group slug"}, {Name: "goal", Type: "string", Usage: "New group goal"}, {Name: "rules", Type: "string", Usage: "New group rules"}, {Name: "message-prompt", Type: "string", Usage: "New group prompt"}, {Name: "doc-url", Type: "string", Usage: "New group document URL"}, {Name: "attachments-allowed", Type: "bool", Usage: "Allow attachments"}, {Name: "max-members", Type: "string", Usage: "Maximum group members"}, {Name: "member-max-messages", Type: "int", Usage: "Per-member message limit"}, {Name: "member-max-total-chars", Type: "int", Usage: "Per-member total char limit"}}},
		{Name: "group.members", Use: "members", Short: "List active group members", Phase: "phase5", Implemented: true, Handler: "group.members", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}, {Name: "limit", Type: "int", Usage: "Maximum number of rows", Default: "100"}}},
		{Name: "group.messages", Use: "messages", Short: "List group messages", Phase: "phase5", Implemented: true, Handler: "group.messages", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}, {Name: "limit", Type: "int", Usage: "Maximum number of rows", Default: "50"}, {Name: "cursor", Type: "string", Usage: "Pagination cursor"}}},
		{Name: "group.code", Use: "code", Short: "Inspect or manage group join codes", Phase: "phase5", Implemented: false},
		{Name: "group.code.get", Use: "get", Short: "Show group join code status", Phase: "phase5", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}}},
		{Name: "group.code.refresh", Use: "refresh", Short: "Rotate the current group join code", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}}},
		{Name: "group.code.enable", Use: "enable", Short: "Enable or disable group join codes", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group DID", Required: true}, {Name: "enabled", Type: "bool", Usage: "Whether join codes are enabled", Required: true}}},
		{Name: "runtime", Use: "runtime", Short: "Runtime mode, listener, and heartbeat commands", Phase: "phase1", Implemented: true},
		{Name: "runtime.status", Use: "status", Short: "Show runtime status", Phase: "phase7", Implemented: true, Handler: "runtime.status", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.apply", Use: "apply", Short: "Apply the configured runtime state", Phase: "phase7", Implemented: true, Handler: "runtime.apply", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.setup", Use: "setup", Short: "Run runtime bootstrap and migration checks", Phase: "phase7", Implemented: true, Handler: "runtime.setup", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "mode", Type: "string", Usage: "Runtime mode", Choices: []string{"http", "websocket"}}}},
		{Name: "runtime.mode", Use: "mode", Short: "Inspect or update runtime mode", Phase: "phase7", Implemented: false},
		{Name: "runtime.mode.get", Use: "get", Short: "Get the current runtime mode", Phase: "phase7", Implemented: true, Handler: "runtime.mode.get", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.mode.set", Use: "set <MODE>", Short: "Set the runtime mode", Phase: "phase7", Implemented: true, Handler: "runtime.mode.set", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener", Use: "listener", Short: "Manage the realtime listener service", Phase: "phase7", Implemented: false},
		{Name: "runtime.listener.status", Use: "status", Short: "Show listener status", Phase: "phase7", Implemented: true, Handler: "runtime.listener.status", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.listener.install", Use: "install", Short: "Install the listener service", Phase: "phase7", Implemented: true, Handler: "runtime.listener.install", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.start", Use: "start", Short: "Start the listener service", Phase: "phase7", Implemented: true, Handler: "runtime.listener.start", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.stop", Use: "stop", Short: "Stop the listener service", Phase: "phase7", Implemented: true, Handler: "runtime.listener.stop", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.restart", Use: "restart", Short: "Restart the listener service", Phase: "phase7", Implemented: true, Handler: "runtime.listener.restart", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.uninstall", Use: "uninstall", Short: "Uninstall the listener service", Phase: "phase7", Implemented: true, Handler: "runtime.listener.uninstall", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.config", Use: "config", Short: "Inspect or update listener configuration", Phase: "phase7", Implemented: false},
		{Name: "runtime.listener.config.show", Use: "show", Short: "Show listener configuration", Phase: "phase7", Implemented: true, Handler: "runtime.listener.config.show", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.listener.config.set", Use: "set", Short: "Update listener configuration", Phase: "phase7", Implemented: true, Handler: "runtime.listener.config.set", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "enabled", Type: "bool", Usage: "Enable or disable listener management"}, {Name: "auto-install", Type: "bool", Usage: "Automatically install the listener service"}, {Name: "auto-start", Type: "bool", Usage: "Automatically start the listener service"}}},
		{Name: "runtime.listener.enable", Use: "enable", Short: "Enable the listener and apply runtime state", Phase: "phase7", Implemented: true, Handler: "runtime.listener.enable", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.disable", Use: "disable", Short: "Disable the listener and apply runtime state", Phase: "phase7", Implemented: true, Handler: "runtime.listener.disable", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.host-notify", Use: "host-notify", Short: "Inspect or update host notification settings", Phase: "phase7", Implemented: false},
		{Name: "runtime.host-notify.config", Use: "config", Short: "Inspect or update host notification configuration", Phase: "phase7", Implemented: false},
		{Name: "runtime.host-notify.config.show", Use: "show", Short: "Show host notification configuration", Phase: "phase7", Implemented: true, Handler: "runtime.host-notify.config.show", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.host-notify.config.set", Use: "set", Short: "Update host notification configuration", Phase: "phase7", Implemented: true, Handler: "runtime.host-notify.config.set", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "sink", Type: "string", Usage: "Host notification sink", Choices: []string{"noop", "log", "file", "openclaw"}}}},
		{Name: "runtime.host-notify.openclaw", Use: "openclaw", Short: "Manage OpenClaw host notification settings", Phase: "phase7", Implemented: false},
		{Name: "runtime.host-notify.openclaw.set", Use: "set", Short: "Update OpenClaw host notification settings", Phase: "phase7", Implemented: true, Handler: "runtime.host-notify.openclaw.set", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "hook-url", Type: "string", Usage: "OpenClaw hook URL"}, {Name: "agent-id", Type: "string", Usage: "OpenClaw agent id"}, {Name: "hook-name", Type: "string", Usage: "OpenClaw hook name"}}},
		{Name: "runtime.host-notify.openclaw.set-token", Use: "set-token", Short: "Store the OpenClaw hook token in config", Phase: "phase7", Implemented: true, Handler: "runtime.host-notify.openclaw.set-token", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "value", Type: "string", Usage: "OpenClaw hook token", Required: true}}},
		{Name: "runtime.host-notify.openclaw.clear-token", Use: "clear-token", Short: "Clear the stored OpenClaw hook token", Phase: "phase7", Implemented: true, Handler: "runtime.host-notify.openclaw.clear-token", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.heartbeat", Use: "heartbeat", Short: "Manage heartbeat tasks", Phase: "phase7", Implemented: false},
		{Name: "runtime.heartbeat.status", Use: "status", Short: "Show heartbeat status", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.heartbeat.install", Use: "install", Short: "Install heartbeat automation", Phase: "phase7", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "every", Type: "string", Usage: "Heartbeat schedule", Default: "15m"}}},
		{Name: "runtime.heartbeat.run-once", Use: "run-once", Short: "Run heartbeat once", Phase: "phase7", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "people", Use: "people", Short: "People, relationships, and contacts commands", Phase: "phase1", Implemented: true},
		{Name: "people.search", Use: "search <QUERY>", Short: "Search users", Phase: "phase8", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.follow", Use: "follow <TARGET>", Short: "Follow a user", Phase: "phase8", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "people.unfollow", Use: "unfollow <TARGET>", Short: "Unfollow a user", Phase: "phase8", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "people.status", Use: "status <TARGET>", Short: "Show relationship status", Phase: "phase8", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.followers", Use: "followers", Short: "List followers", Phase: "phase8", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.following", Use: "following", Short: "List following", Phase: "phase8", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.contacts", Use: "contacts", Short: "Manage local contacts", Phase: "phase8", Implemented: false},
		{Name: "people.contacts.list", Use: "list", Short: "List local contacts", Phase: "phase8", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.contacts.save", Use: "save", Short: "Save a local contact", Phase: "phase8", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "did", Type: "string", Usage: "Contact DID", Required: true}, {Name: "handle", Type: "string", Usage: "Contact handle"}, {Name: "reason", Type: "string", Usage: "Why the contact was saved"}}},
		{Name: "page", Use: "page", Short: "Content page commands", Phase: "phase1", Implemented: true},
		{Name: "page.create", Use: "create", Short: "Create a content page", Phase: "phase8", Implemented: true, Handler: "page.create", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Page slug"}, {Name: "title", Type: "string", Usage: "Page title"}, {Name: "markdown", Type: "string", Usage: "Inline markdown body"}, {Name: "markdown-file", Type: "string", Usage: "Markdown file path"}, {Name: "visibility", Type: "string", Usage: "Page visibility", Default: "public", Choices: []string{"public", "draft", "unlisted"}}}},
		{Name: "page.list", Use: "list", Short: "List content pages", Phase: "phase8", Implemented: true, Handler: "page.list", Outputs: []string{"json", "pretty", "table"}},
		{Name: "page.get", Use: "get", Short: "Get one content page", Phase: "phase8", Implemented: true, Handler: "page.get", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Page slug", Required: true}}},
		{Name: "page.update", Use: "update", Short: "Update a content page", Phase: "phase8", Implemented: true, Handler: "page.update", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Page slug", Required: true}, {Name: "title", Type: "string", Usage: "Page title"}, {Name: "markdown", Type: "string", Usage: "Inline markdown body"}, {Name: "markdown-file", Type: "string", Usage: "Markdown file path"}, {Name: "visibility", Type: "string", Usage: "Page visibility", Choices: []string{"public", "draft", "unlisted"}}}},
		{Name: "page.rename", Use: "rename", Short: "Rename a content page slug", Phase: "phase8", Implemented: true, Handler: "page.rename", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Current page slug", Required: true}, {Name: "to", Type: "string", Usage: "New slug", Required: true}}},
		{Name: "page.delete", Use: "delete", Short: "Delete a content page", Phase: "phase8", Implemented: true, Handler: "page.delete", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Page slug", Required: true}}},
		{Name: "debug", Use: "debug", Short: "Debugging and raw inspection commands", Phase: "phase1", Implemented: true},
		{Name: "debug.db", Use: "db", Short: "Database inspection helpers", Phase: "phase4", Implemented: true},
		{Name: "debug.db.query", Use: "query <SQL>", Short: "Execute a local SQLite query", Phase: "phase4", Implemented: true, Handler: "debug.db.query", Outputs: []string{"json", "pretty", "table"}},
		{Name: "debug.db.import-v1", Use: "import-v1", Short: "Import a legacy v1 local SQLite database", Phase: "phase4", Implemented: true, Handler: "debug.db.import-v1", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "path", Type: "string", Usage: "Explicit legacy database path override"}}},
		{Name: "debug.raw", Use: "raw", Short: "Raw RPC helpers", Phase: "phase1", Implemented: false},
		{Name: "debug.raw.rpc", Use: "rpc", Short: "Call raw RPC endpoints", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty"}},
		{Name: "debug.schema-cache", Use: "schema-cache", Short: "Inspect generated schema metadata", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "debug.logs", Use: "logs", Short: "Tail runtime logs", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"ndjson", "pretty"}, Flags: []FlagSpec{{Name: "follow", Type: "bool", Usage: "Follow log output"}}},
	}
}
