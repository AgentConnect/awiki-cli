package docs

import "strings"

type Topic struct {
	Name       string   `json:"name"`
	Summary    string   `json:"summary"`
	References []string `json:"references"`
}

type Index struct {
	topics map[string]Topic
	list   []Topic
}

func NewIndex() *Index {
	list := []Topic{
		{
			Name:    "overview",
			Summary: "Project-level implementation overview and roadmap",
			References: []string{
				"docs/plan/awiki-v2-implementation-plan.md",
			},
		},
		{
			Name:    "phase-0",
			Summary: "Frozen implementation constraints and audit outputs",
			References: []string{
				"docs/plan/phase-0/implementation-constraints.md",
				"docs/plan/phase-0/capability-mapping.md",
				"docs/plan/phase-0/audit-findings.md",
				"docs/plan/phase-0/adr-index.md",
			},
		},
		{
			Name:    "architecture",
			Summary: "Overall v2 architecture and command model",
			References: []string{
				"docs/architecture/awiki-v2-architecture.md",
				"docs/architecture/awiki-command-v2.md",
				"docs/architecture/awiki-mail-cli.md",
				"docs/architecture/awiki-skill-architecture.md",
			},
		},
		{
			Name:    "hosted-domains",
			Summary: "Hosted multi-domain workspace and service configuration",
			References: []string{
				"docs/architecture/multi-tenant-hosted-domain-implementation.md",
				"docs/plan/multi-tenant-hosted-domain-landing-plan.md",
				"docs/architecture/anp-service-discovery.md",
			},
		},
		{
			Name:    "mail",
			Summary: "Top-level mail command surface and service configuration",
			References: []string{
				"docs/architecture/awiki-mail-cli.md",
				"docs/architecture/awiki-v2-architecture.md",
				"docs/plan/phase-0/implementation-constraints.md",
			},
		},
		{
			Name:    "skills",
			Summary: "Current skill entrypoint and reference topology",
			References: []string{
				"skills/SKILL.md",
				"docs/architecture/awiki-skill-architecture.md",
				"skills/references/02-identity.md",
				"skills/references/03-messaging.md",
			},
		},
		{
			Name:    "output",
			Summary: "Output contract, dry-run, schema, and exit code rules",
			References: []string{
				"docs/architecture/output-format.md",
			},
		},
		{
			Name:    "review",
			Summary: "Secondary review checklist and dependency map for PR review",
			References: []string{
				"docs/harness/review-spec.md",
				"docs/plan/phase-0/implementation-constraints.md",
				"docs/plan/phase-0/audit-findings.md",
			},
		},
		{
			Name:    "storage",
			Summary: "Identity layout and SQLite baseline references",
			References: []string{
				"docs/plan/phase-0/implementation-constraints.md",
				"../awiki-agent-id-message/scripts/credential_layout.py",
				"../awiki-agent-id-message/scripts/local_store.py",
				"../awiki-agent-id-message/references/local-store-schema.md",
			},
		},
		{
			Name:    "runtime",
			Summary: "Runtime mode, listener, heartbeat, and migration references",
			References: []string{
				"docs/architecture/awiki-v2-architecture.md",
				"../awiki-agent-id-message/scripts/setup_realtime.py",
				"../awiki-agent-id-message/scripts/ws_listener.py",
				"../awiki-agent-id-message/references/WEBSOCKET_LISTENER.md",
			},
		},
	}
	index := &Index{topics: make(map[string]Topic, len(list)), list: list}
	for _, topic := range list {
		index.topics[strings.ToLower(topic.Name)] = topic
	}
	return index
}

func (i *Index) All() []Topic {
	if i == nil {
		return nil
	}
	return append([]Topic(nil), i.list...)
}

func (i *Index) Lookup(name string) (Topic, bool) {
	if i == nil {
		return Topic{}, false
	}
	topic, ok := i.topics[strings.ToLower(strings.TrimSpace(name))]
	return topic, ok
}
