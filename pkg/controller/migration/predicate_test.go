package migration

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestMigrationPredicate_Create_ReturnsTrue(t *testing.T) {
	p := MigrationPredicate{}
	m := &api.Migration{}
	if !p.Create(event.TypedCreateEvent[*api.Migration]{Object: m}) {
		t.Fatalf("expected true")
	}
}

func TestMigrationPredicate_Delete_ReturnsTrue(t *testing.T) {
	p := MigrationPredicate{}
	m := &api.Migration{}
	if !p.Delete(event.TypedDeleteEvent[*api.Migration]{Object: m}) {
		t.Fatalf("expected true")
	}
}

func TestMigrationPredicate_Update_ReturnsFalseWhenObservedGenerationUpToDate(t *testing.T) {
	p := MigrationPredicate{}
	old := &api.Migration{ObjectMeta: metav1.ObjectMeta{Generation: 5}}
	old.Status.ObservedGeneration = 5
	newObj := old.DeepCopy()
	if p.Update(event.TypedUpdateEvent[*api.Migration]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected false")
	}
}

func TestMigrationPredicate_Update_ReturnsTrueWhenObservedGenerationBehind(t *testing.T) {
	p := MigrationPredicate{}
	old := &api.Migration{ObjectMeta: metav1.ObjectMeta{Generation: 5}}
	old.Status.ObservedGeneration = 4
	newObj := old.DeepCopy()
	if !p.Update(event.TypedUpdateEvent[*api.Migration]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected true")
	}
}

func TestPlanPredicate_Create_ReconciledTrueOnlyWhenObservedGenerationEqualsGeneration(t *testing.T) {
	p := PlanPredicate{}
	pl := &api.Plan{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	pl.Status.ObservedGeneration = 2
	if !p.Create(event.TypedCreateEvent[*api.Plan]{Object: pl}) {
		t.Fatalf("expected true")
	}
	pl.Status.ObservedGeneration = 1
	if p.Create(event.TypedCreateEvent[*api.Plan]{Object: pl}) {
		t.Fatalf("expected false")
	}
}

func TestPlanPredicate_Update_ReconciledTrueOnlyWhenObservedGenerationEqualsGeneration(t *testing.T) {
	p := PlanPredicate{}
	old := &api.Plan{ObjectMeta: metav1.ObjectMeta{Generation: 3}}
	old.Status.ObservedGeneration = 3
	newObj := old.DeepCopy()
	if !p.Update(event.TypedUpdateEvent[*api.Plan]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected true")
	}
	newObj.Status.ObservedGeneration = 2
	if p.Update(event.TypedUpdateEvent[*api.Plan]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected false")
	}
}

func TestPlanPredicate_Generic_ReconciledTrueOnlyWhenObservedGenerationEqualsGeneration(t *testing.T) {
	p := PlanPredicate{}
	pl := &api.Plan{ObjectMeta: metav1.ObjectMeta{Generation: 7}}
	pl.Status.ObservedGeneration = 7
	if !p.Generic(event.TypedGenericEvent[*api.Plan]{Object: pl}) {
		t.Fatalf("expected true")
	}
	pl.Status.ObservedGeneration = 6
	if p.Generic(event.TypedGenericEvent[*api.Plan]{Object: pl}) {
		t.Fatalf("expected false")
	}
}

func TestPlanPredicate_Delete_AlwaysTrue(t *testing.T) {
	p := PlanPredicate{}
	pl := &api.Plan{}
	if !p.Delete(event.TypedDeleteEvent[*api.Plan]{Object: pl}) {
		t.Fatalf("expected true")
	}
}
