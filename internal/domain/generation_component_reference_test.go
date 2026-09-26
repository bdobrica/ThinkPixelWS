package domain

import (
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	"github.com/google/uuid"
)

func TestGenerationComponentReferences(t *testing.T) {
	portable := GenerationComponentReference{ComponentID: uuid.Must(uuid.NewV7()), Kind: GenerationReferencePortableSnapshot, Ref: shared.DigestBytes([]byte("snapshot manifest")).String()}
	checkpoint := GenerationComponentReference{ComponentID: portable.ComponentID, Kind: GenerationReferenceProviderCheckpoint, Ref: "snapshot-uid:123", Provider: "kubernetes", TargetID: "homelab"}
	for _, test := range []struct {
		name       string
		refs       []GenerationComponentReference
		durability GenerationDurability
		valid      bool
	}{
		{"portable", []GenerationComponentReference{portable}, GenerationDurabilityPortable, true},
		{"checkpoint", []GenerationComponentReference{checkpoint}, GenerationDurabilityProviderLocal, true},
		{"checkpoint is not portable", []GenerationComponentReference{checkpoint}, GenerationDurabilityPortable, false},
		{"duplicate", []GenerationComponentReference{portable, portable}, GenerationDurabilityPortable, false},
		{"missing component", []GenerationComponentReference{{Kind: portable.Kind, Ref: portable.Ref}}, GenerationDurabilityPortable, false},
		{"mutable ref", []GenerationComponentReference{{ComponentID: portable.ComponentID, Kind: portable.Kind, Ref: "latest"}}, GenerationDurabilityPortable, false},
		{"unscoped checkpoint", []GenerationComponentReference{{ComponentID: portable.ComponentID, Kind: checkpoint.Kind, Ref: checkpoint.Ref}}, GenerationDurabilityProviderLocal, false},
		{"unknown kind", []GenerationComponentReference{{ComponentID: portable.ComponentID, Kind: "pvc", Ref: "working-volume"}}, GenerationDurabilityProviderLocal, false},
		{"too many", make([]GenerationComponentReference, 257), GenerationDurabilityPortable, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := validNewWorkspaceGeneration(t)
			input.ComponentReferences, input.Durability = test.refs, test.durability
			_, err := input.WorkspaceGeneration(time.Now())
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
		})
	}
	input := validNewWorkspaceGeneration(t)
	input.ComponentReferences = []GenerationComponentReference{portable}
	g, err := input.WorkspaceGeneration(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	input.ComponentReferences[0].Ref = "changed"
	if g.ComponentReferences[0] != portable {
		t.Fatal("constructor retained caller's slice")
	}
}
