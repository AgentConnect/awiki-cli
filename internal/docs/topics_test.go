package docs

import "testing"

func TestIndexLookupNormalizesTopicNames(t *testing.T) {
	index := NewIndex()
	cases := []struct {
		name      string
		query     string
		wantName  string
		wantFound bool
	}{
		{
			name:      "exact match",
			query:     "overview",
			wantName:  "overview",
			wantFound: true,
		},
		{
			name:      "case insensitive and trimmed",
			query:     "  PhAsE-0  ",
			wantName:  "phase-0",
			wantFound: true,
		},
		{
			name:      "unknown topic",
			query:     "missing-topic",
			wantFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			topic, ok := index.Lookup(tc.query)
			if ok != tc.wantFound {
				t.Fatalf("Lookup(%q) found = %t, want %t", tc.query, ok, tc.wantFound)
			}
			if !tc.wantFound {
				if topic.Name != "" || topic.Summary != "" || len(topic.References) != 0 {
					t.Fatalf("Lookup(%q) topic = %#v, want zero-value fields", tc.query, topic)
				}
				return
			}
			if topic.Name != tc.wantName {
				t.Fatalf("Lookup(%q) topic.Name = %q, want %q", tc.query, topic.Name, tc.wantName)
			}
			if topic.Summary == "" {
				t.Fatalf("Lookup(%q) returned empty summary", tc.query)
			}
			if len(topic.References) == 0 {
				t.Fatalf("Lookup(%q) returned no references", tc.query)
			}
		})
	}
}

func TestIndexAllReturnsStableCopy(t *testing.T) {
	index := NewIndex()
	first := index.All()
	if len(first) == 0 {
		t.Fatal("All() returned no topics")
	}

	originalName := first[0].Name
	first[0].Name = "mutated"
	first = append(first, Topic{Name: "extra"})

	second := index.All()
	if len(second) == 0 {
		t.Fatal("All() returned no topics on second call")
	}
	if second[0].Name != originalName {
		t.Fatalf("second All()[0].Name = %q, want %q", second[0].Name, originalName)
	}
	if len(second) != len(index.list) {
		t.Fatalf("len(second) = %d, want %d", len(second), len(index.list))
	}

	topic, ok := index.Lookup(originalName)
	if !ok {
		t.Fatalf("Lookup(%q) = not found after mutating All() result", originalName)
	}
	if topic.Name != originalName {
		t.Fatalf("Lookup(%q) topic.Name = %q, want %q", originalName, topic.Name, originalName)
	}
}

func TestNilIndexBehaviorsRemainSafe(t *testing.T) {
	var index *Index

	if got := index.All(); got != nil {
		t.Fatalf("nil Index All() = %#v, want nil", got)
	}
	if topic, ok := index.Lookup("overview"); ok {
		t.Fatalf("nil Index Lookup() found = %t, topic = %#v, want false and zero Topic", ok, topic)
	}
}

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
