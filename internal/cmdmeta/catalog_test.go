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

func TestGroupE2EECatalogDoesNotPublishOutOfScopeJoinOrRecoverySurfaces(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	for _, name := range []string{
		"group e2ee external-commit",
		"group e2ee external_commit",
		"group e2ee cloud-snapshot",
		"group e2ee snapshot",
		"group e2ee multi-device-sync",
		"group e2ee k1-recover",
		"group e2ee k1_recover",
		"group e2ee get-join-info",
		"group e2ee accept-welcome",
	} {
		if _, ok := catalog.Lookup(name); ok {
			t.Fatalf("Lookup(%q) = true, want false for hidden P6 non-goal", name)
		}
	}

	for _, spec := range catalog.Specs() {
		if !strings.HasPrefix(spec.Name, "group.e2ee") {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			spec.Name,
			spec.Use,
			spec.Short,
			spec.Long,
		}, " "))
		for _, flag := range spec.Flags {
			haystack += " " + strings.ToLower(flag.Name+" "+flag.Usage)
		}
		for _, forbidden := range []string{
			"external commit",
			"external-commit",
			"external_commit",
			"cloud snapshot",
			"cloud-snapshot",
			"multi-device",
			"k1-recover",
			"k1_recover",
		} {
			if strings.Contains(haystack, forbidden) {
				t.Fatalf("group E2EE catalog spec %q contains forbidden non-goal %q", spec.Name, forbidden)
			}
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
