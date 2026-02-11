package plan

import (
	"context"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestPlanPredicate_UpdateChanged(t *testing.T) {
	p := PlanPredicate{}
	old := &api.Plan{}
	newObj := &api.Plan{}

	newObj.Generation = 2
	newObj.Status.ObservedGeneration = 1
	if !p.Update(event.TypedUpdateEvent[*api.Plan]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected changed=true")
	}

	newObj.Generation = 2
	newObj.Status.ObservedGeneration = 2
	if p.Update(event.TypedUpdateEvent[*api.Plan]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected changed=false")
	}
}

func TestProviderPredicate_ReconciledGating(t *testing.T) {
	pp := &ProviderPredicate{}
	prov := &api.Provider{}

	// create: only reconciled passes.
	prov.Generation = 2
	prov.Status.ObservedGeneration = 1
	if pp.Create(event.TypedCreateEvent[*api.Provider]{Object: prov}) {
		t.Fatalf("expected false when not reconciled")
	}
	prov.Status.ObservedGeneration = 2
	if !pp.Create(event.TypedCreateEvent[*api.Provider]{Object: prov}) {
		t.Fatalf("expected true when reconciled")
	}

	// generic: same gating.
	prov.Status.ObservedGeneration = 1
	if pp.Generic(event.TypedGenericEvent[*api.Provider]{Object: prov}) {
		t.Fatalf("expected false when not reconciled")
	}
	prov.Status.ObservedGeneration = 2
	if !pp.Generic(event.TypedGenericEvent[*api.Provider]{Object: prov}) {
		t.Fatalf("expected true when reconciled")
	}

	// update: avoid ensureWatch (needs handler/client); verify false when not reconciled.
	prov.Status.ObservedGeneration = 1
	if pp.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: prov, ObjectNew: prov}) {
		t.Fatalf("expected false when not reconciled")
	}
}

func TestMigrationPredicate(t *testing.T) {
	mp := MigrationPredicate{}

	m := &api.Migration{}
	// pending when not completed.
	if !mp.Create(event.TypedCreateEvent[*api.Migration]{Object: m}) {
		t.Fatalf("expected true when pending")
	}
	// update: only generation change.
	old := &api.Migration{}
	newObj := &api.Migration{}
	old.Generation = 1
	newObj.Generation = 2
	if !mp.Update(event.TypedUpdateEvent[*api.Migration]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected true when generation changed")
	}
	newObj.Generation = 1
	if mp.Update(event.TypedUpdateEvent[*api.Migration]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected false when generation same")
	}
	// delete: only if started.
	if mp.Delete(event.TypedDeleteEvent[*api.Migration]{Object: m}) {
		t.Fatalf("expected false when not started")
	}
	m.Status.MarkStarted()
	if !mp.Delete(event.TypedDeleteEvent[*api.Migration]{Object: m}) {
		t.Fatalf("expected true when started")
	}
}

func TestRequestForMigration(t *testing.T) {
	ctx := context.Background()

	// non-migration should return none.
	if got := RequestForMigration(ctx, &api.Plan{}); len(got) != 0 {
		t.Fatalf("expected empty list")
	}

	m := &api.Migration{}
	m.Spec.Plan = corev1.ObjectReference{Name: "p", Namespace: "ns"}
	got := RequestForMigration(ctx, m)
	if len(got) != 1 {
		t.Fatalf("expected 1 request, got %d", len(got))
	}
	if got[0].NamespacedName != (types.NamespacedName{Namespace: "ns", Name: "p"}) {
		t.Fatalf("unexpected request: %#v", got[0])
	}
}

func TestMapAndHookPredicates(t *testing.T) {
	np := NetMapPredicate{}
	dp := DsMapPredicate{}
	hp := HookPredicate{}

	nm := &api.NetworkMap{}
	nm.Generation = 2
	nm.Status.ObservedGeneration = 1
	if np.Update(event.TypedUpdateEvent[*api.NetworkMap]{ObjectOld: nm, ObjectNew: nm}) {
		t.Fatalf("expected false when not reconciled")
	}
	nm.Status.ObservedGeneration = 2
	if !np.Update(event.TypedUpdateEvent[*api.NetworkMap]{ObjectOld: nm, ObjectNew: nm}) {
		t.Fatalf("expected true when reconciled")
	}

	sm := &api.StorageMap{}
	sm.Generation = 2
	sm.Status.ObservedGeneration = 2
	if !dp.Update(event.TypedUpdateEvent[*api.StorageMap]{ObjectOld: sm, ObjectNew: sm}) {
		t.Fatalf("expected true when reconciled")
	}

	h := &api.Hook{}
	h.Generation = 3
	h.Status.ObservedGeneration = 2
	if hp.Update(event.TypedUpdateEvent[*api.Hook]{ObjectOld: h, ObjectNew: h}) {
		t.Fatalf("expected false when not reconciled")
	}
	h.Status.ObservedGeneration = 3
	if !hp.Update(event.TypedUpdateEvent[*api.Hook]{ObjectOld: h, ObjectNew: h}) {
		t.Fatalf("expected true when reconciled")
	}
}
