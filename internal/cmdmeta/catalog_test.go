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

func TestCatalogPublishesRefreshTokenCommand(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	spec, ok := catalog.Lookup("id refresh-token")
	if !ok {
		t.Fatal(`Lookup("id refresh-token") = false, want true`)
	}
	if spec.Hidden {
		t.Fatalf("spec.Hidden = %t, want false", spec.Hidden)
	}
	if !spec.SideEffect {
		t.Fatal("spec.SideEffect = false, want true")
	}
	if !strings.Contains(strings.ToLower(spec.Short), "refresh") {
		t.Fatalf("spec.Short = %q, want refresh wording", spec.Short)
	}
	if !strings.Contains(spec.Long, "did-auth.get_me") {
		t.Fatalf("spec.Long = %q, want did-auth.get_me guidance", spec.Long)
	}
}

func TestCatalogPublishesTopLevelMailCommands(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	for _, name := range []string{"mail", "mail inbox", "mail notify", "mail read", "mail mark-read", "mail account", "mail send", "mail attachment download"} {
		if _, ok := catalog.Lookup(name); !ok {
			t.Fatalf("Lookup(%q) = false, want true", name)
		}
	}

	for _, name := range []string{"msg mail", "msg mail inbox", "msg mail read", "msg mail send"} {
		if _, ok := catalog.Lookup(name); ok {
			t.Fatalf("Lookup(%q) = true, want false", name)
		}
	}
}

func TestCatalogPublishesConfigSetCommand(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	spec, ok := catalog.Lookup("config set")
	if !ok {
		t.Fatal(`Lookup("config set") = false, want true`)
	}
	if spec.Hidden {
		t.Fatalf("spec.Hidden = %t, want false", spec.Hidden)
	}
	if !spec.SideEffect {
		t.Fatal("spec.SideEffect = false, want true")
	}
	if len(spec.Flags) != 1 || spec.Flags[0].Name != "did-domain" {
		t.Fatalf("spec.Flags = %#v, want did-domain flag", spec.Flags)
	}
}

func TestCatalogPublishesTenantSiteCommands(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	for _, name := range []string{
		"site",
		"site root get",
		"site root set",
		"site page list",
		"site page get",
		"site page create",
		"site page update",
		"site page rename",
		"site page delete",
	} {
		if _, ok := catalog.Lookup(name); !ok {
			t.Fatalf("Lookup(%q) = false, want true", name)
		}
	}
}
