package network

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestMapPredicate_CreateUpdateDelete(t *testing.T) {
	p := MapPredicate{}

	m := &api.NetworkMap{}
	m.Generation = 2
	m.Status.ObservedGeneration = 0

	if !p.Create(event.TypedCreateEvent[*api.NetworkMap]{Object: m}) {
		t.Fatalf("expected create=true")
	}
	old := m.DeepCopy()
	if !p.Update(event.TypedUpdateEvent[*api.NetworkMap]{ObjectOld: old, ObjectNew: m}) {
		t.Fatalf("expected update=true")
	}

	m2 := &api.NetworkMap{}
	m2.Generation = 1
	m2.Status.ObservedGeneration = 1
	old2 := m2.DeepCopy()
	if p.Update(event.TypedUpdateEvent[*api.NetworkMap]{ObjectOld: old2, ObjectNew: m2}) {
		t.Fatalf("expected update=false")
	}

	if !p.Delete(event.TypedDeleteEvent[*api.NetworkMap]{Object: m2}) {
		t.Fatalf("expected delete=true")
	}
}

func TestProviderPredicate_BasicsAndEnsureWatchEarlyReturn(t *testing.T) {
	pp := &ProviderPredicate{}

	pr := &api.Provider{}
	pr.Generation = 1
	pr.Status.ObservedGeneration = 1

	if !pp.Create(event.TypedCreateEvent[*api.Provider]{Object: pr}) {
		t.Fatalf("expected create=true")
	}

	// Update: reconciled => ensureWatch called. Keep it safe by *not* setting Ready.
	pr2 := pr.DeepCopy()
	pr2.Status.DeleteCondition(libcnd.Ready)
	if !pp.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: pr, ObjectNew: pr2}) {
		t.Fatalf("expected update=true")
	}

	pr3 := pr.DeepCopy()
	pr3.Status.ObservedGeneration = 0
	if pp.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: pr, ObjectNew: pr3}) {
		t.Fatalf("expected update=false")
	}

	if !pp.Generic(event.TypedGenericEvent[*api.Provider]{Object: pr}) {
		t.Fatalf("expected generic=true")
	}
	if pp.Generic(event.TypedGenericEvent[*api.Provider]{Object: pr3}) {
		t.Fatalf("expected generic=false")
	}

	if !pp.Delete(event.TypedDeleteEvent[*api.Provider]{Object: pr}) {
		t.Fatalf("expected delete=true")
	}
}

func TestProviderPredicate_ensureWatch_NotReady_NoPanic(t *testing.T) {
	pp := &ProviderPredicate{}
	p := &api.Provider{}
	p.Status.DeleteCondition(libcnd.Ready)
	pp.ensureWatch(p)
}

func TestProviderPredicate_ensureWatch_ReadyUnsupportedType_NoPanic(t *testing.T) {
	pp := &ProviderPredicate{}
	p := &api.Provider{}
	p.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True, Category: libcnd.Required})
	// Spec.Type nil => Undefined => handler.New should error.
	pp.ensureWatch(p)
}

func TestProviderPredicate_Update_ReconciledReadyUnsupportedType_ReturnsTrue(t *testing.T) {
	pp := &ProviderPredicate{}
	pOld := &api.Provider{}
	pNew := &api.Provider{}
	pNew.Generation = 1
	pNew.Status.ObservedGeneration = 1
	pNew.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True, Category: libcnd.Required})
	if !pp.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: pOld, ObjectNew: pNew}) {
		t.Fatalf("expected true")
	}
}

func TestProviderPredicate_Create_NotReconciled_False(t *testing.T) {
	pp := &ProviderPredicate{}
	p := &api.Provider{}
	p.Generation = 2
	p.Status.ObservedGeneration = 1
	if pp.Create(event.TypedCreateEvent[*api.Provider]{Object: p}) {
		t.Fatalf("expected false")
	}
}

func TestProviderPredicate_Generic_NotReconciled_False(t *testing.T) {
	pp := &ProviderPredicate{}
	p := &api.Provider{}
	p.Generation = 2
	p.Status.ObservedGeneration = 1
	if pp.Generic(event.TypedGenericEvent[*api.Provider]{Object: p}) {
		t.Fatalf("expected false")
	}
}

func TestProviderPredicate_Update_NotReconciled_False(t *testing.T) {
	pp := &ProviderPredicate{}
	pOld := &api.Provider{}
	pNew := &api.Provider{}
	pNew.Generation = 2
	pNew.Status.ObservedGeneration = 1
	if pp.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: pOld, ObjectNew: pNew}) {
		t.Fatalf("expected false")
	}
}

func TestMapPredicate_Update_ChangedWhenObservedLessThanGeneration(t *testing.T) {
	p := MapPredicate{}
	old := &api.NetworkMap{}
	newObj := &api.NetworkMap{}
	newObj.Generation = 3
	newObj.Status.ObservedGeneration = 2
	if !p.Update(event.TypedUpdateEvent[*api.NetworkMap]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected true")
	}
}

func TestMapPredicate_Update_NotChangedWhenObservedEqualsGeneration(t *testing.T) {
	p := MapPredicate{}
	old := &api.NetworkMap{}
	newObj := &api.NetworkMap{}
	newObj.Generation = 3
	newObj.Status.ObservedGeneration = 3
	if p.Update(event.TypedUpdateEvent[*api.NetworkMap]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected false")
	}
}

func TestMapPredicate_Create_AlwaysTrue(t *testing.T) {
	p := MapPredicate{}
	if !p.Create(event.TypedCreateEvent[*api.NetworkMap]{Object: &api.NetworkMap{}}) {
		t.Fatalf("expected true")
	}
}

func TestMapPredicate_Delete_AlwaysTrue(t *testing.T) {
	p := MapPredicate{}
	if !p.Delete(event.TypedDeleteEvent[*api.NetworkMap]{Object: &api.NetworkMap{}}) {
		t.Fatalf("expected true")
	}
}
