package host

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestHostPredicate_CreateUpdateDelete(t *testing.T) {
	p := HostPredicate{}

	h := &api.Host{}
	h.Generation = 2
	h.Status.ObservedGeneration = 0

	if !p.Create(event.TypedCreateEvent[*api.Host]{Object: h}) {
		t.Fatalf("expected create=true")
	}

	old := h.DeepCopy()
	if !p.Update(event.TypedUpdateEvent[*api.Host]{ObjectOld: old, ObjectNew: h}) {
		t.Fatalf("expected update=true")
	}

	h2 := &api.Host{}
	h2.Generation = 1
	h2.Status.ObservedGeneration = 1
	old2 := h2.DeepCopy()
	if p.Update(event.TypedUpdateEvent[*api.Host]{ObjectOld: old2, ObjectNew: h2}) {
		t.Fatalf("expected update=false")
	}

	if !p.Delete(event.TypedDeleteEvent[*api.Host]{Object: h2}) {
		t.Fatalf("expected delete=true")
	}
}

func TestProviderPredicate_BasicsAndEnsureWatchEarlyReturn(t *testing.T) {
	pp := &ProviderPredicate{}

	// Create: only true when reconciled.
	pr := &api.Provider{}
	pr.Generation = 1
	pr.Status.ObservedGeneration = 0
	if pp.Create(event.TypedCreateEvent[*api.Provider]{Object: pr}) {
		t.Fatalf("expected create=false when not reconciled")
	}
	pr.Status.ObservedGeneration = 1
	if !pp.Create(event.TypedCreateEvent[*api.Provider]{Object: pr}) {
		t.Fatalf("expected create=true when reconciled")
	}

	// Update: when reconciled, ensureWatch() runs. We keep it safe by *not* setting Ready.
	pr2 := pr.DeepCopy()
	pr2.Status.DeleteCondition(libcnd.Ready)
	if !pp.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: pr, ObjectNew: pr2}) {
		t.Fatalf("expected update=true when reconciled")
	}

	// Update: when not reconciled, false.
	pr3 := pr.DeepCopy()
	pr3.Status.ObservedGeneration = 0
	if pp.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: pr, ObjectNew: pr3}) {
		t.Fatalf("expected update=false when not reconciled")
	}

	// Generic: mirrors reconciled.
	if !pp.Generic(event.TypedGenericEvent[*api.Provider]{Object: pr}) {
		t.Fatalf("expected generic=true when reconciled")
	}
	if pp.Generic(event.TypedGenericEvent[*api.Provider]{Object: pr3}) {
		t.Fatalf("expected generic=false when not reconciled")
	}

	// Delete: should not panic even with zero WatchManager.
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

func TestHostPredicate_Update_ChangedWhenObservedLessThanGeneration(t *testing.T) {
	p := HostPredicate{}
	hOld := &api.Host{}
	hNew := &api.Host{}
	hNew.Generation = 3
	hNew.Status.ObservedGeneration = 2
	if !p.Update(event.TypedUpdateEvent[*api.Host]{ObjectOld: hOld, ObjectNew: hNew}) {
		t.Fatalf("expected true")
	}
}

func TestHostPredicate_Update_NotChangedWhenObservedEqualsGeneration(t *testing.T) {
	p := HostPredicate{}
	hOld := &api.Host{}
	hNew := &api.Host{}
	hNew.Generation = 3
	hNew.Status.ObservedGeneration = 3
	if p.Update(event.TypedUpdateEvent[*api.Host]{ObjectOld: hOld, ObjectNew: hNew}) {
		t.Fatalf("expected false")
	}
}

func TestHostPredicate_Create_AlwaysTrue(t *testing.T) {
	p := HostPredicate{}
	if !p.Create(event.TypedCreateEvent[*api.Host]{Object: &api.Host{}}) {
		t.Fatalf("expected true")
	}
}

func TestHostPredicate_Delete_AlwaysTrue(t *testing.T) {
	p := HostPredicate{}
	if !p.Delete(event.TypedDeleteEvent[*api.Host]{Object: &api.Host{}}) {
		t.Fatalf("expected true")
	}
}
