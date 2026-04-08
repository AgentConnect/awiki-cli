package cmdmeta

import "testing"

func TestCatalogPublishesCanonicalGroupCommands(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	for _, name := range []string{"group show", "group kick", "group code get", "group code refresh", "group code enable"} {
		if _, ok := catalog.Lookup(name); !ok {
			t.Fatalf("Lookup(%q) = false, want true", name)
		}
	}

	for _, name := range []string{"group.get", "group add", "group remove"} {
		if _, ok := catalog.Lookup(name); ok {
			t.Fatalf("Lookup(%q) = true, want false", name)
		}
	}
}
