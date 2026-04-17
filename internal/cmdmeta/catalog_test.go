package cmdmeta

import (
	"strings"
	"testing"
)

func TestCatalogPublishesCanonicalGroupCommands(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	for _, name := range []string{"group get", "group add", "group remove", "group code get", "group code refresh", "group code enable"} {
		if _, ok := catalog.Lookup(name); !ok {
			t.Fatalf("Lookup(%q) = false, want true", name)
		}
	}

	for _, name := range []string{"group show", "group kick"} {
		if _, ok := catalog.Lookup(name); ok {
			t.Fatalf("Lookup(%q) = true, want false", name)
		}
	}
}

func TestCatalogPublishesHiddenReplaceDIDCommand(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	spec, ok := catalog.Lookup("id replace-did")
	if !ok {
		t.Fatal(`Lookup("id replace-did") = false, want true`)
	}
	if !spec.Hidden {
		t.Fatalf("spec.Hidden = %t, want true", spec.Hidden)
	}
}

func TestCatalogPublishesSkillCommands(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	for _, name := range []string{"skill index", "skill get", "skill sync", "skill export"} {
		spec, ok := catalog.Lookup(name)
		if !ok {
			t.Fatalf("Lookup(%q) = false, want true", name)
		}
		if !strings.HasPrefix(spec.Name, "skill.") {
			t.Fatalf("spec.Name = %q, want skill.*", spec.Name)
		}
	}
}
