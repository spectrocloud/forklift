package hook

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestHookPredicate_CreateUpdateDelete(t *testing.T) {
	p := HookPredicate{}

	h := &api.Hook{}
	h.Generation = 1
	h.Status.ObservedGeneration = 0

	if !p.Create(event.TypedCreateEvent[*api.Hook]{Object: h}) {
		t.Fatalf("expected create=true")
	}

	// Update: changed when observed < generation.
	old := h.DeepCopy()
	h.Status.ObservedGeneration = 0
	h.Generation = 1
	if !p.Update(event.TypedUpdateEvent[*api.Hook]{ObjectOld: old, ObjectNew: h}) {
		t.Fatalf("expected update=true when generation advanced")
	}

	// Update: unchanged when observed == generation.
	h2 := &api.Hook{}
	h2.Generation = 5
	h2.Status.ObservedGeneration = 5
	old2 := h2.DeepCopy()
	if p.Update(event.TypedUpdateEvent[*api.Hook]{ObjectOld: old2, ObjectNew: h2}) {
		t.Fatalf("expected update=false when already reconciled")
	}

	if !p.Delete(event.TypedDeleteEvent[*api.Hook]{Object: h2}) {
		t.Fatalf("expected delete=true")
	}
}
