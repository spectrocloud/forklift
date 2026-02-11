package plan

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	"github.com/kubev2v/forklift/pkg/settings"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestPlanPredicate_Create_ReturnsTrue(t *testing.T) {
	p := PlanPredicate{}
	if !p.Create(event.TypedCreateEvent[*api.Plan]{Object: &api.Plan{}}) {
		t.Fatalf("expected true")
	}
}

func TestPlanPredicate_Delete_ReturnsTrue(t *testing.T) {
	p := PlanPredicate{}
	if !p.Delete(event.TypedDeleteEvent[*api.Plan]{Object: &api.Plan{}}) {
		t.Fatalf("expected true")
	}
}

func TestPlanPredicate_Update_ReturnsFalseWhenObservedGenerationUpToDate(t *testing.T) {
	p := PlanPredicate{}
	old := &api.Plan{ObjectMeta: metav1.ObjectMeta{Generation: 5}}
	old.Status.ObservedGeneration = 5
	newObj := old.DeepCopy()
	if p.Update(event.TypedUpdateEvent[*api.Plan]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected false")
	}
}

func TestPlanPredicate_Update_ReturnsTrueWhenObservedGenerationBehind(t *testing.T) {
	p := PlanPredicate{}
	old := &api.Plan{ObjectMeta: metav1.ObjectMeta{Generation: 5}}
	old.Status.ObservedGeneration = 4
	newObj := old.DeepCopy()
	if !p.Update(event.TypedUpdateEvent[*api.Plan]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected true")
	}
}

func TestProviderPredicate_Create_ReturnsTrueWhenReconciled(t *testing.T) {
	pp := &ProviderPredicate{}
	obj := &api.Provider{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 2
	if !pp.Create(event.TypedCreateEvent[*api.Provider]{Object: obj}) {
		t.Fatalf("expected true")
	}
}

func TestProviderPredicate_Create_ReturnsFalseWhenNotReconciled(t *testing.T) {
	pp := &ProviderPredicate{}
	obj := &api.Provider{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 1
	if pp.Create(event.TypedCreateEvent[*api.Provider]{Object: obj}) {
		t.Fatalf("expected false")
	}
}

func TestProviderPredicate_Generic_ReturnsTrueWhenReconciled(t *testing.T) {
	pp := &ProviderPredicate{}
	obj := &api.Provider{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 2
	if !pp.Generic(event.TypedGenericEvent[*api.Provider]{Object: obj}) {
		t.Fatalf("expected true")
	}
}

func TestProviderPredicate_Generic_ReturnsFalseWhenNotReconciled(t *testing.T) {
	pp := &ProviderPredicate{}
	obj := &api.Provider{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 1
	if pp.Generic(event.TypedGenericEvent[*api.Provider]{Object: obj}) {
		t.Fatalf("expected false")
	}
}

func TestProviderPredicate_Update_ReturnsFalseWhenNotReconciled(t *testing.T) {
	pp := &ProviderPredicate{}
	obj := &api.Provider{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 1
	if pp.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: obj, ObjectNew: obj}) {
		t.Fatalf("expected false")
	}
}

func TestProviderPredicate_Update_ReturnsTrueWhenReconciled(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	pp := &ProviderPredicate{
		channel: make(chan event.GenericEvent, 1),
		client:  fake.NewClientBuilder().WithScheme(s).Build(),
	}
	obj := &api.Provider{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 2
	if !pp.Update(event.TypedUpdateEvent[*api.Provider]{ObjectOld: obj, ObjectNew: obj}) {
		t.Fatalf("expected true")
	}
}

func TestProviderPredicate_Delete_ReturnsTrue(t *testing.T) {
	pp := &ProviderPredicate{}
	obj := &api.Provider{}
	if !pp.Delete(event.TypedDeleteEvent[*api.Provider]{Object: obj}) {
		t.Fatalf("expected true")
	}
}

func TestProviderPredicate_EnsureWatch_ReturnsEarlyWhenNotReady(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	pp := &ProviderPredicate{
		channel: make(chan event.GenericEvent, 1),
		client:  fake.NewClientBuilder().WithScheme(s).Build(),
	}
	tp := api.VSphere
	p := &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	p.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.False})
	pp.ensureWatch(p) // should return early
}

func TestProviderPredicate_EnsureWatch_IgnoresUnsupportedProvider(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	pp := &ProviderPredicate{
		channel: make(chan event.GenericEvent, 1),
		client:  fake.NewClientBuilder().WithScheme(s).Build(),
	}
	tp := api.ProviderType("Nope")
	p := &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	p.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True})
	pp.ensureWatch(p) // should swallow handler.New error
}

func TestProviderPredicate_EnsureWatch_HandlesWatchError(t *testing.T) {
	oldCA := settings.Settings.Inventory.TLS.CA
	oldDev := settings.Settings.Development
	t.Cleanup(func() {
		settings.Settings.Inventory.TLS.CA = oldCA
		settings.Settings.Development = oldDev
	})
	settings.Settings.Inventory.TLS.CA = "/this/path/does/not/exist.pem"
	settings.Settings.Development = false

	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	pp := &ProviderPredicate{
		channel: make(chan event.GenericEvent, 1),
		client:  fake.NewClientBuilder().WithScheme(s).Build(),
	}
	tp := api.VSphere
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}, Spec: api.ProviderSpec{Type: &tp}}
	p.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True})
	p.Status.ObservedGeneration = 1
	p.Generation = 1
	pp.ensureWatch(p)
}

func TestNetMapPredicate_Create_False(t *testing.T) {
	p := NetMapPredicate{}
	if p.Create(event.TypedCreateEvent[*api.NetworkMap]{Object: &api.NetworkMap{}}) {
		t.Fatalf("expected false")
	}
}

func TestNetMapPredicate_Update_ReturnsTrueWhenReconciled(t *testing.T) {
	p := NetMapPredicate{}
	obj := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 2
	if !p.Update(event.TypedUpdateEvent[*api.NetworkMap]{ObjectNew: obj}) {
		t.Fatalf("expected true")
	}
}

func TestNetMapPredicate_Update_ReturnsFalseWhenNotReconciled(t *testing.T) {
	p := NetMapPredicate{}
	obj := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 1
	if p.Update(event.TypedUpdateEvent[*api.NetworkMap]{ObjectNew: obj}) {
		t.Fatalf("expected false")
	}
}

func TestNetMapPredicate_Delete_True(t *testing.T) {
	p := NetMapPredicate{}
	if !p.Delete(event.TypedDeleteEvent[*api.NetworkMap]{Object: &api.NetworkMap{}}) {
		t.Fatalf("expected true")
	}
}

func TestNetMapPredicate_Generic_ReturnsTrueWhenReconciled(t *testing.T) {
	p := NetMapPredicate{}
	obj := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 2
	if !p.Generic(event.TypedGenericEvent[*api.NetworkMap]{Object: obj}) {
		t.Fatalf("expected true")
	}
}

func TestNetMapPredicate_Generic_ReturnsFalseWhenNotReconciled(t *testing.T) {
	p := NetMapPredicate{}
	obj := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 1
	if p.Generic(event.TypedGenericEvent[*api.NetworkMap]{Object: obj}) {
		t.Fatalf("expected false")
	}
}

func TestDsMapPredicate_Create_False(t *testing.T) {
	p := DsMapPredicate{}
	if p.Create(event.TypedCreateEvent[*api.StorageMap]{Object: &api.StorageMap{}}) {
		t.Fatalf("expected false")
	}
}

func TestDsMapPredicate_Update_ReturnsTrueWhenReconciled(t *testing.T) {
	p := DsMapPredicate{}
	obj := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 2
	if !p.Update(event.TypedUpdateEvent[*api.StorageMap]{ObjectNew: obj}) {
		t.Fatalf("expected true")
	}
}

func TestDsMapPredicate_Update_ReturnsFalseWhenNotReconciled(t *testing.T) {
	p := DsMapPredicate{}
	obj := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 1
	if p.Update(event.TypedUpdateEvent[*api.StorageMap]{ObjectNew: obj}) {
		t.Fatalf("expected false")
	}
}

func TestDsMapPredicate_Delete_True(t *testing.T) {
	p := DsMapPredicate{}
	if !p.Delete(event.TypedDeleteEvent[*api.StorageMap]{Object: &api.StorageMap{}}) {
		t.Fatalf("expected true")
	}
}

func TestDsMapPredicate_Generic_ReturnsTrueWhenReconciled(t *testing.T) {
	p := DsMapPredicate{}
	obj := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 2
	if !p.Generic(event.TypedGenericEvent[*api.StorageMap]{Object: obj}) {
		t.Fatalf("expected true")
	}
}

func TestDsMapPredicate_Generic_ReturnsFalseWhenNotReconciled(t *testing.T) {
	p := DsMapPredicate{}
	obj := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 1
	if p.Generic(event.TypedGenericEvent[*api.StorageMap]{Object: obj}) {
		t.Fatalf("expected false")
	}
}

func TestHookPredicate_Create_False(t *testing.T) {
	p := HookPredicate{}
	if p.Create(event.TypedCreateEvent[*api.Hook]{Object: &api.Hook{}}) {
		t.Fatalf("expected false")
	}
}

func TestHookPredicate_Update_ReturnsTrueWhenReconciled(t *testing.T) {
	p := HookPredicate{}
	obj := &api.Hook{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 2
	if !p.Update(event.TypedUpdateEvent[*api.Hook]{ObjectNew: obj}) {
		t.Fatalf("expected true")
	}
}

func TestHookPredicate_Update_ReturnsFalseWhenNotReconciled(t *testing.T) {
	p := HookPredicate{}
	obj := &api.Hook{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 1
	if p.Update(event.TypedUpdateEvent[*api.Hook]{ObjectNew: obj}) {
		t.Fatalf("expected false")
	}
}

func TestHookPredicate_Delete_True(t *testing.T) {
	p := HookPredicate{}
	if !p.Delete(event.TypedDeleteEvent[*api.Hook]{Object: &api.Hook{}}) {
		t.Fatalf("expected true")
	}
}

func TestHookPredicate_Generic_ReturnsTrueWhenReconciled(t *testing.T) {
	p := HookPredicate{}
	obj := &api.Hook{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 2
	if !p.Generic(event.TypedGenericEvent[*api.Hook]{Object: obj}) {
		t.Fatalf("expected true")
	}
}

func TestHookPredicate_Generic_ReturnsFalseWhenNotReconciled(t *testing.T) {
	p := HookPredicate{}
	obj := &api.Hook{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	obj.Status.ObservedGeneration = 1
	if p.Generic(event.TypedGenericEvent[*api.Hook]{Object: obj}) {
		t.Fatalf("expected false")
	}
}

func TestMigrationPredicate_Create_TrueWhenPending(t *testing.T) {
	p := MigrationPredicate{}
	m := &api.Migration{}
	if !p.Create(event.TypedCreateEvent[*api.Migration]{Object: m}) {
		t.Fatalf("expected true")
	}
}

func TestMigrationPredicate_Create_FalseWhenCompleted(t *testing.T) {
	p := MigrationPredicate{}
	m := &api.Migration{}
	m.Status.MarkCompleted()
	if p.Create(event.TypedCreateEvent[*api.Migration]{Object: m}) {
		t.Fatalf("expected false")
	}
}

func TestMigrationPredicate_Update_TrueWhenGenerationChanged(t *testing.T) {
	p := MigrationPredicate{}
	old := &api.Migration{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	newObj := &api.Migration{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	if !p.Update(event.TypedUpdateEvent[*api.Migration]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected true")
	}
}

func TestMigrationPredicate_Update_FalseWhenGenerationSame(t *testing.T) {
	p := MigrationPredicate{}
	old := &api.Migration{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	newObj := &api.Migration{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	if p.Update(event.TypedUpdateEvent[*api.Migration]{ObjectOld: old, ObjectNew: newObj}) {
		t.Fatalf("expected false")
	}
}

func TestMigrationPredicate_Delete_TrueWhenStarted(t *testing.T) {
	p := MigrationPredicate{}
	m := &api.Migration{}
	m.Status.MarkStarted()
	if !p.Delete(event.TypedDeleteEvent[*api.Migration]{Object: m}) {
		t.Fatalf("expected true")
	}
}

func TestMigrationPredicate_Delete_FalseWhenNotStarted(t *testing.T) {
	p := MigrationPredicate{}
	m := &api.Migration{}
	if p.Delete(event.TypedDeleteEvent[*api.Migration]{Object: m}) {
		t.Fatalf("expected false")
	}
}

func TestMigrationPredicate_Generic_False(t *testing.T) {
	p := MigrationPredicate{}
	if p.Generic(event.TypedGenericEvent[*api.Migration]{Object: &api.Migration{}}) {
		t.Fatalf("expected false")
	}
}
