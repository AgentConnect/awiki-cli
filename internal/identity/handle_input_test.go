package identity

import "testing"

func TestNormalizeHandleInput(t *testing.T) {
	t.Parallel()

	t.Run("bare", func(t *testing.T) {
		t.Parallel()
		got, err := NormalizeHandleInput("Alice", "Tenant.Example.")
		if err != nil {
			t.Fatalf("NormalizeHandleInput() error = %v", err)
		}
		if got.LocalPart != "alice" || got.FullHandle != "alice.tenant.example" || got.EffectiveDomain != "tenant.example" || got.ExplicitDomain {
			t.Fatalf("NormalizeHandleInput(bare) = %+v", got)
		}
	})

	t.Run("full", func(t *testing.T) {
		t.Parallel()
		got, err := NormalizeHandleInput("Alice.Other.Example", "tenant.example")
		if err != nil {
			t.Fatalf("NormalizeHandleInput() error = %v", err)
		}
		if got.LocalPart != "alice" || got.FullHandle != "alice.other.example" || got.EffectiveDomain != "other.example" || !got.ExplicitDomain {
			t.Fatalf("NormalizeHandleInput(full) = %+v", got)
		}
	})

	t.Run("wba", func(t *testing.T) {
		t.Parallel()
		got, err := NormalizeHandleInput("wba://Alice.Other.Example", "tenant.example")
		if err != nil {
			t.Fatalf("NormalizeHandleInput() error = %v", err)
		}
		if got.FullHandle != "alice.other.example" || !got.ExplicitDomain {
			t.Fatalf("NormalizeHandleInput(wba) = %+v", got)
		}
	})
}
