package provider

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestProviderPredicate_Create_ReturnsTrue(t *testing.T) {
	p := ProviderPredicate{}
	obj := &api.Provider{}
	if !p.Create(event.TypedCreateEvent[*api.Provider]{Object: obj}) {
		t.Fatalf("expected true")
	}
}

func TestProviderPredicate_Delete_ReturnsTrue(t *testing.T) {
	p := ProviderPredicate{}
	obj := &api.Provider{}
	if !p.Delete(event.TypedDeleteEvent[*api.Provider]{Object: obj}) {
		t.Fatalf("expected true")
	}
}

func TestProviderPredicate_Update_ReturnsFalseWhenObservedGenerationUpToDate(t *testing.T) {
	p := ProviderPredicate{}
	old := &api.Provider{ObjectMeta: metav1.ObjectMeta{Generation: 5}}
	old.Status.ObservedGeneration = 5
	newObj := old.DeepCopy()
	if p.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected false")
	}
}

func TestProviderPredicate_Update_ReturnsTrueWhenObservedGenerationBehind(t *testing.T) {
	p := ProviderPredicate{}
	old := &api.Provider{ObjectMeta: metav1.ObjectMeta{Generation: 5}}
	old.Status.ObservedGeneration = 4
	newObj := old.DeepCopy()
	if !p.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected true")
	}
}
