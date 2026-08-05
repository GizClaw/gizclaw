package apitypes

import "testing"

func TestResourceMemoryLayoutDiscriminatorRoundTrip(t *testing.T) {
	resource := Resource{}
	input := MemoryLayoutResource{
		ApiVersion: ResourceAPIVersionGizclawAdminv1alpha1,
		Metadata:   ResourceMetadata{Id: "pet-memory"},
	}
	if err := resource.FromMemoryLayoutResource(input); err != nil {
		t.Fatal(err)
	}
	discriminator, err := resource.Discriminator()
	if err != nil {
		t.Fatal(err)
	}
	if discriminator != "MemoryLayout" {
		t.Fatalf("Discriminator() = %q, want MemoryLayout", discriminator)
	}
	value, err := resource.ValueByDiscriminator()
	if err != nil {
		t.Fatal(err)
	}
	layout, ok := value.(MemoryLayoutResource)
	if !ok {
		t.Fatalf("ValueByDiscriminator() = %T, want MemoryLayoutResource", value)
	}
	if layout.Kind != MemoryLayoutResourceKindMemoryLayout || layout.Metadata.Id != "pet-memory" {
		t.Fatalf("ValueByDiscriminator() = %#v", layout)
	}
}
