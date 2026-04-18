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

func TestCatalogPublishesPublicDangerousReplaceDIDCommand(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	spec, ok := catalog.Lookup("id replace-did")
	if !ok {
		t.Fatal(`Lookup("id replace-did") = false, want true`)
	}
	if spec.Hidden {
		t.Fatalf("spec.Hidden = %t, want false", spec.Hidden)
	}
	if !spec.SideEffect {
		t.Fatal("spec.SideEffect = false, want true")
	}
	if !strings.Contains(strings.ToLower(spec.Short), "danger") {
		t.Fatalf("spec.Short = %q, want danger warning", spec.Short)
	}
	if !strings.Contains(spec.Long, "--identity") {
		t.Fatalf("spec.Long = %q, want target identity guidance", spec.Long)
	}
}
