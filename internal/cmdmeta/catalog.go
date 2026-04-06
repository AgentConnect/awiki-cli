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
		{Name: "completion", Use: "completion", Short: "Generate shell completion scripts", Phase: "phase1", Implemented: true},
		{Name: "completion.bash", Use: "bash", Short: "Generate Bash completion", Phase: "phase1", Implemented: true, Handler: "completion.bash"},
		{Name: "completion.zsh", Use: "zsh", Short: "Generate Zsh completion", Phase: "phase1", Implemented: true, Handler: "completion.zsh"},
		{Name: "completion.fish", Use: "fish", Short: "Generate Fish completion", Phase: "phase1", Implemented: true, Handler: "completion.fish"},
		{Name: "completion.powershell", Use: "powershell", Short: "Generate PowerShell completion", Phase: "phase1", Implemented: true, Handler: "completion.powershell"},
		{Name: "config", Use: "config", Short: "Inspect resolved CLI configuration", Phase: "phase1", Implemented: true},
		{Name: "config.show", Use: "show", Short: "Show resolved configuration values", Phase: "phase1", Implemented: true, Handler: "config.show", Outputs: []string{"json", "pretty", "table"}},
		{Name: "id", Use: "id", Short: "Identity lifecycle commands", Phase: "phase1", Implemented: true},
		{Name: "id.status", Use: "status", Short: "Show identity status", Phase: "phase2", Implemented: true, Handler: "id.status", Outputs: []string{"json", "pretty"}},
		{Name: "id.create", Use: "create", Short: "Create a new local DID identity", Phase: "phase2", Implemented: true, Handler: "id.create", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "name", Type: "string", Usage: "Identity display name", Required: true}, {Name: "identity", Type: "string", Usage: "Identity alias override"}}},
		{Name: "id.register", Use: "register", Short: "Register a handle identity", Phase: "phase2", Implemented: true, Handler: "id.register", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "handle", Type: "string", Usage: "Handle local part", Required: true}, {Name: "phone", Type: "string", Usage: "Phone number for registration"}, {Name: "email", Type: "string", Usage: "Email address for registration"}, {Name: "otp", Type: "string", Usage: "Verification code"}, {Name: "invite-code", Type: "string", Usage: "Invite code if required"}, {Name: "wait", Type: "bool", Usage: "Wait for email verification before completing registration"}}},
		{Name: "id.bind", Use: "bind", Short: "Bind phone or email to the current identity", Phase: "phase2", Implemented: true, Handler: "id.bind", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "phone", Type: "string", Usage: "Phone number to bind"}, {Name: "email", Type: "string", Usage: "Email address to bind"}, {Name: "otp", Type: "string", Usage: "Verification code"}, {Name: "wait", Type: "bool", Usage: "Wait for email verification before completing the bind"}}},
		{Name: "id.resolve", Use: "resolve", Short: "Resolve a DID or handle", Phase: "phase2", Implemented: true, Handler: "id.resolve", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "handle", Type: "string", Usage: "Handle to resolve"}, {Name: "did", Type: "string", Usage: "DID to resolve"}}},
		{Name: "id.recover", Use: "recover", Short: "Recover a handle with phone verification", Phase: "phase2", Implemented: true, Handler: "id.recover", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "handle", Type: "string", Usage: "Handle local part", Required: true}, {Name: "phone", Type: "string", Usage: "Recovery phone number", Required: true}, {Name: "otp", Type: "string", Usage: "Verification code", Required: true}}},
		{Name: "id.list", Use: "list", Short: "List local identities", Phase: "phase2", Implemented: true, Handler: "id.list", Outputs: []string{"json", "pretty", "table"}},
		{Name: "id.current", Use: "current", Short: "Show the default identity", Phase: "phase2", Implemented: true, Handler: "id.current", Outputs: []string{"json", "pretty", "table"}},
		{Name: "id.use", Use: "use <identity>", Short: "Switch the default identity", Phase: "phase2", Implemented: true, Handler: "id.use", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "id.profile", Use: "profile", Short: "Read or update DID profile data", Phase: "phase2", Implemented: false},
		{Name: "id.profile.get", Use: "get", Short: "Get DID profile data", Phase: "phase2", Implemented: true, Handler: "id.profile.get", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "self", Type: "bool", Usage: "Read the active identity profile"}, {Name: "handle", Type: "string", Usage: "Read a profile by handle"}, {Name: "did", Type: "string", Usage: "Read a profile by DID"}}},
		{Name: "id.profile.set", Use: "set", Short: "Update DID profile data", Phase: "phase2", Implemented: true, Handler: "id.profile.set", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "display-name", Type: "string", Usage: "Profile display name"}, {Name: "bio", Type: "string", Usage: "Profile bio"}, {Name: "tags", Type: "string", Usage: "Comma-separated tags"}, {Name: "markdown", Type: "string", Usage: "Inline markdown body"}, {Name: "markdown-file", Type: "string", Usage: "Markdown file path"}}},
		{Name: "id.import-v1", Use: "import-v1", Short: "Import credentials from the v1 awiki-agent-id-message layout", Phase: "phase2", Implemented: true, Handler: "id.import-v1", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "name", Type: "string", Usage: "Import one legacy identity by name"}, {Name: "all", Type: "bool", Usage: "Import all detected legacy identities"}}},
		{Name: "msg", Use: "msg", Short: "Messaging commands", Phase: "phase1", Implemented: true},
		{Name: "msg.send", Use: "send", Short: "Send a direct or group message", Phase: "phase4", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "to", Type: "string", Usage: "Direct message target"}, {Name: "group", Type: "string", Usage: "Group target"}, {Name: "text", Type: "string", Usage: "Inline message text"}, {Name: "text-file", Type: "string", Usage: "Message file path"}, {Name: "type", Type: "string", Usage: "Message type", Default: "text"}, {Name: "secure", Type: "string", Usage: "Secure mode", Default: "off", Choices: []string{"off", "on"}}}},
		{Name: "msg.inbox", Use: "inbox", Short: "Read inbox messages", Phase: "phase4", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "scope", Type: "string", Usage: "Message scope", Default: "all", Choices: []string{"all", "direct", "group"}}, {Name: "with", Type: "string", Usage: "Direct peer filter"}, {Name: "group", Type: "string", Usage: "Group filter"}, {Name: "unread", Type: "bool", Usage: "Only unread messages"}, {Name: "limit", Type: "int", Usage: "Maximum number of results", Default: "20"}, {Name: "mark-read", Type: "bool", Usage: "Mark returned messages as read"}}},
		{Name: "msg.history", Use: "history", Short: "Read message history", Phase: "phase4", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "with", Type: "string", Usage: "Direct peer DID or handle", Required: true}, {Name: "limit", Type: "int", Usage: "Maximum number of rows", Default: "50"}, {Name: "cursor", Type: "string", Usage: "Pagination cursor"}}},
		{Name: "msg.mark-read", Use: "mark-read [MESSAGE_ID...]", Short: "Mark messages as read", Phase: "phase4", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "msg.secure", Use: "secure", Short: "Secure direct messaging commands", Phase: "phase5", Implemented: false},
		{Name: "msg.secure.status", Use: "status", Short: "Inspect secure messaging status", Phase: "phase5", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "with", Type: "string", Usage: "Target peer DID or handle"}}},
		{Name: "msg.secure.init", Use: "init", Short: "Initialize a secure session", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "with", Type: "string", Usage: "Target peer DID or handle", Required: true}}},
		{Name: "msg.secure.repair", Use: "repair", Short: "Repair a secure session", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "with", Type: "string", Usage: "Target peer DID or handle", Required: true}}},
		{Name: "msg.secure.failed", Use: "failed", Short: "List failed secure outbox items", Phase: "phase5", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "msg.secure.retry", Use: "retry <OUTBOX_ID>", Short: "Retry one failed secure outbox item", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "msg.secure.drop", Use: "drop <OUTBOX_ID>", Short: "Drop one failed secure outbox item", Phase: "phase5", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "group", Use: "group", Short: "Group lifecycle commands", Phase: "phase1", Implemented: true},
		{Name: "group.create", Use: "create", Short: "Create a new group", Phase: "phase4", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "name", Type: "string", Usage: "Group name", Required: true}, {Name: "slug", Type: "string", Usage: "Group slug"}, {Name: "description", Type: "string", Usage: "Group description"}, {Name: "goal", Type: "string", Usage: "Group goal"}, {Name: "rules", Type: "string", Usage: "Group rules"}}},
		{Name: "group.show", Use: "show", Short: "Show group details", Phase: "phase4", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group identifier", Required: true}}},
		{Name: "group.update", Use: "update", Short: "Update group metadata", Phase: "phase4", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group identifier", Required: true}, {Name: "name", Type: "string", Usage: "New group name"}, {Name: "description", Type: "string", Usage: "New group description"}, {Name: "goal", Type: "string", Usage: "New group goal"}, {Name: "rules", Type: "string", Usage: "New group rules"}}},
		{Name: "group.join", Use: "join", Short: "Join a group with the global join code", Phase: "phase4", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "code", Type: "string", Usage: "Global 6-digit join code", Required: true}}},
		{Name: "group.leave", Use: "leave", Short: "Leave a group", Phase: "phase4", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group identifier", Required: true}}},
		{Name: "group.kick", Use: "kick", Short: "Remove a member from a group", Phase: "phase4", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group identifier", Required: true}, {Name: "member", Type: "string", Usage: "Member DID", Required: true}}},
		{Name: "group.members", Use: "members", Short: "List active group members", Phase: "phase4", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group identifier", Required: true}, {Name: "limit", Type: "int", Usage: "Maximum number of rows", Default: "100"}}},
		{Name: "group.messages", Use: "messages", Short: "List group messages", Phase: "phase4", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group identifier", Required: true}, {Name: "limit", Type: "int", Usage: "Maximum number of rows", Default: "50"}, {Name: "cursor", Type: "string", Usage: "Pagination cursor"}}},
		{Name: "group.code", Use: "code", Short: "Manage group join codes", Phase: "phase4", Implemented: false},
		{Name: "group.code.get", Use: "get", Short: "Get the current group join code", Phase: "phase4", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group identifier", Required: true}}},
		{Name: "group.code.refresh", Use: "refresh", Short: "Refresh the current group join code", Phase: "phase4", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group identifier", Required: true}}},
		{Name: "group.code.enable", Use: "enable", Short: "Enable or disable group join by code", Phase: "phase4", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "group", Type: "string", Usage: "Group identifier", Required: true}, {Name: "enabled", Type: "bool", Usage: "Whether joining is enabled", Required: true}}},
		{Name: "runtime", Use: "runtime", Short: "Runtime mode, listener, and heartbeat commands", Phase: "phase1", Implemented: true},
		{Name: "runtime.status", Use: "status", Short: "Show runtime status", Phase: "phase6", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.setup", Use: "setup", Short: "Run runtime bootstrap and migration checks", Phase: "phase6", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "mode", Type: "string", Usage: "Runtime mode", Choices: []string{"http", "websocket"}}}},
		{Name: "runtime.mode", Use: "mode", Short: "Inspect or update runtime mode", Phase: "phase6", Implemented: false},
		{Name: "runtime.mode.get", Use: "get", Short: "Get the current runtime mode", Phase: "phase6", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.mode.set", Use: "set <MODE>", Short: "Set the runtime mode", Phase: "phase6", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener", Use: "listener", Short: "Manage the realtime listener service", Phase: "phase6", Implemented: false},
		{Name: "runtime.listener.status", Use: "status", Short: "Show listener status", Phase: "phase6", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.listener.install", Use: "install", Short: "Install the listener service", Phase: "phase6", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.start", Use: "start", Short: "Start the listener service", Phase: "phase6", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.stop", Use: "stop", Short: "Stop the listener service", Phase: "phase6", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.restart", Use: "restart", Short: "Restart the listener service", Phase: "phase6", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.listener.uninstall", Use: "uninstall", Short: "Uninstall the listener service", Phase: "phase6", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "runtime.heartbeat", Use: "heartbeat", Short: "Manage heartbeat tasks", Phase: "phase6", Implemented: false},
		{Name: "runtime.heartbeat.status", Use: "status", Short: "Show heartbeat status", Phase: "phase6", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "runtime.heartbeat.install", Use: "install", Short: "Install heartbeat automation", Phase: "phase6", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "every", Type: "string", Usage: "Heartbeat schedule", Default: "15m"}}},
		{Name: "runtime.heartbeat.run-once", Use: "run-once", Short: "Run heartbeat once", Phase: "phase6", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "people", Use: "people", Short: "People, relationships, and contacts commands", Phase: "phase1", Implemented: true},
		{Name: "people.search", Use: "search <QUERY>", Short: "Search users", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.follow", Use: "follow <TARGET>", Short: "Follow a user", Phase: "phase7", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "people.unfollow", Use: "unfollow <TARGET>", Short: "Unfollow a user", Phase: "phase7", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}},
		{Name: "people.status", Use: "status <TARGET>", Short: "Show relationship status", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.followers", Use: "followers", Short: "List followers", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.following", Use: "following", Short: "List following", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.contacts", Use: "contacts", Short: "Manage local contacts", Phase: "phase7", Implemented: false},
		{Name: "people.contacts.list", Use: "list", Short: "List local contacts", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "people.contacts.save", Use: "save", Short: "Save a local contact", Phase: "phase7", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "did", Type: "string", Usage: "Contact DID", Required: true}, {Name: "handle", Type: "string", Usage: "Contact handle"}, {Name: "reason", Type: "string", Usage: "Why the contact was saved"}}},
		{Name: "page", Use: "page", Short: "Content page commands", Phase: "phase1", Implemented: true},
		{Name: "page.create", Use: "create", Short: "Create a content page", Phase: "phase7", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Page slug", Required: true}, {Name: "title", Type: "string", Usage: "Page title", Required: true}, {Name: "markdown", Type: "string", Usage: "Inline markdown body"}, {Name: "markdown-file", Type: "string", Usage: "Markdown file path"}}},
		{Name: "page.list", Use: "list", Short: "List content pages", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "page.get", Use: "get", Short: "Get one content page", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Page slug", Required: true}}},
		{Name: "page.update", Use: "update", Short: "Update a content page", Phase: "phase7", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Page slug", Required: true}, {Name: "title", Type: "string", Usage: "Page title"}, {Name: "markdown", Type: "string", Usage: "Inline markdown body"}, {Name: "markdown-file", Type: "string", Usage: "Markdown file path"}}},
		{Name: "page.rename", Use: "rename", Short: "Rename a content page slug", Phase: "phase7", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Current page slug", Required: true}, {Name: "to", Type: "string", Usage: "New slug", Required: true}}},
		{Name: "page.delete", Use: "delete", Short: "Delete a content page", Phase: "phase7", Implemented: false, Handler: "stub", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "slug", Type: "string", Usage: "Page slug", Required: true}}},
		{Name: "debug", Use: "debug", Short: "Debugging and raw inspection commands", Phase: "phase1", Implemented: true},
		{Name: "debug.db", Use: "db", Short: "Database inspection helpers", Phase: "phase3", Implemented: true},
		{Name: "debug.db.query", Use: "query <SQL>", Short: "Execute a local SQLite query", Phase: "phase3", Implemented: true, Handler: "debug.db.query", Outputs: []string{"json", "pretty", "table"}},
		{Name: "debug.db.import-v1", Use: "import-v1", Short: "Import a legacy v1 local SQLite database", Phase: "phase3", Implemented: true, Handler: "debug.db.import-v1", SideEffect: true, Outputs: []string{"json", "pretty"}, Flags: []FlagSpec{{Name: "path", Type: "string", Usage: "Explicit legacy database path override"}}},
		{Name: "debug.raw", Use: "raw", Short: "Raw RPC helpers", Phase: "phase1", Implemented: false},
		{Name: "debug.raw.rpc", Use: "rpc", Short: "Call raw RPC endpoints", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty"}},
		{Name: "debug.schema-cache", Use: "schema-cache", Short: "Inspect generated schema metadata", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"json", "pretty", "table"}},
		{Name: "debug.logs", Use: "logs", Short: "Tail runtime logs", Phase: "phase7", Implemented: false, Handler: "stub", Outputs: []string{"ndjson", "pretty"}, Flags: []FlagSpec{{Name: "follow", Type: "bool", Usage: "Follow log output"}}},
	}
}
