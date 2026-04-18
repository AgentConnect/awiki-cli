package docs

import "testing"

func TestNewIndexPublishesMailTopic(t *testing.T) {
	t.Parallel()

	index := NewIndex()

	topic, ok := index.Lookup("mail")
	if !ok {
		t.Fatal(`Lookup("mail") = false, want true`)
	}
	if topic.Name != "mail" {
		t.Fatalf("topic.Name = %q, want %q", topic.Name, "mail")
	}
	if len(topic.References) == 0 || topic.References[0] != "docs/architecture/awiki-mail-cli.md" {
		t.Fatalf("topic.References = %v, want first reference to awiki-mail-cli.md", topic.References)
	}

	architecture, ok := index.Lookup("architecture")
	if !ok {
		t.Fatal(`Lookup("architecture") = false, want true`)
	}
	found := false
	for _, reference := range architecture.References {
		if reference == "docs/architecture/awiki-mail-cli.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("architecture.References = %v, want docs/architecture/awiki-mail-cli.md", architecture.References)
	}
}
