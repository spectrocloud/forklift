package plan

import (
	"context"
	"testing"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	"github.com/kubev2v/forklift/pkg/controller/base"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	"github.com/kubev2v/forklift/pkg/lib/condition"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newPlanScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1.AddToScheme: %v", err)
	}
	if err := api.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("api.AddToScheme: %v", err)
	}
	return s
}

func newPlanReconciler(t *testing.T, objs ...runtime.Object) (*Reconciler, client.Client) {
	t.Helper()
	s := newPlanScheme(t)
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&api.Plan{}).
		WithRuntimeObjects(objs...).
		Build()
	r := &Reconciler{
		Reconciler: base.Reconciler{
			Client:        cl,
			EventRecorder: record.NewFakeRecorder(50),
			Log:           logging.WithName("test-plan-controller"),
		},
	}
	return r, cl
}

func TestReconciler_isDanglingArchivedPlan_TrueWhenArchivedAndNoSource(t *testing.T) {
	r := &Reconciler{}
	p := &api.Plan{}
	p.Spec.Archived = true
	p.Referenced.Provider.Source = nil
	if !r.isDanglingArchivedPlan(p) {
		t.Fatalf("expected true")
	}
}

func TestReconciler_isDanglingArchivedPlan_FalseWhenNotArchived(t *testing.T) {
	r := &Reconciler{}
	p := &api.Plan{}
	p.Spec.Archived = false
	p.Referenced.Provider.Source = nil
	if r.isDanglingArchivedPlan(p) {
		t.Fatalf("expected false")
	}
}

func TestReconciler_isDanglingArchivedPlan_FalseWhenSourcePresent(t *testing.T) {
	r := &Reconciler{}
	p := &api.Plan{}
	p.Spec.Archived = true
	p.Referenced.Provider.Source = &api.Provider{}
	if r.isDanglingArchivedPlan(p) {
		t.Fatalf("expected false")
	}
}

func TestReconciler_newSnapshot_CreatesActiveSnapshot(t *testing.T) {
	r := &Reconciler{}
	srcType := api.OpenShift
	dstType := api.OpenShift
	plan := &api.Plan{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p1", UID: types.UID("puid"), Generation: 1},
	}
	plan.Referenced.Provider.Source = &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src", UID: types.UID("suid"), Generation: 2}, Spec: api.ProviderSpec{Type: &srcType}}
	plan.Referenced.Provider.Destination = &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dst", UID: types.UID("duid"), Generation: 3}, Spec: api.ProviderSpec{Type: &dstType}}
	plan.Referenced.Map.Network = &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm", UID: types.UID("nmuid"), Generation: 4}}
	plan.Referenced.Map.Storage = &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm", UID: types.UID("smuid"), Generation: 5}}

	mig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m1", UID: types.UID("muid"), Generation: 6}}

	ctx := &plancontext.Context{Plan: plan, Migration: mig}
	snap := r.newSnapshot(ctx)
	if snap == nil {
		t.Fatalf("expected snapshot")
	}
	if snap.Plan.UID != types.UID("puid") || snap.Migration.UID != types.UID("muid") {
		t.Fatalf("unexpected snapshot refs: %#v", snap)
	}
	if snap.Provider.Source.UID != types.UID("suid") || snap.Provider.Destination.UID != types.UID("duid") {
		t.Fatalf("unexpected provider refs: %#v", snap.Provider)
	}
	if snap.Map.Network.UID != types.UID("nmuid") || snap.Map.Storage.UID != types.UID("smuid") {
		t.Fatalf("unexpected map refs: %#v", snap.Map)
	}
}

func TestReconciler_matchSnapshot_ReturnsTrueWhenMatched(t *testing.T) {
	r := &Reconciler{}
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: types.UID("puid"), Generation: 1}}
	src := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src", UID: types.UID("suid"), Generation: 1}}
	dst := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dst", UID: types.UID("duid"), Generation: 1}}
	nm := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm", UID: types.UID("nmuid"), Generation: 1}}
	sm := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm", UID: types.UID("smuid"), Generation: 1}}
	plan.Referenced.Provider.Source = src
	plan.Referenced.Provider.Destination = dst
	plan.Referenced.Map.Network = nm
	plan.Referenced.Map.Storage = sm
	mig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("muid"), Generation: 1}}
	ctx := &plancontext.Context{Plan: plan, Migration: mig}
	_ = r.newSnapshot(ctx)

	matched := r.matchSnapshot(ctx)
	if !matched {
		t.Fatalf("expected matched")
	}
}

func TestReconciler_matchSnapshot_MismatchMarksCanceledAndClearsExecuting(t *testing.T) {
	r := &Reconciler{}
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: types.UID("puid"), Generation: 1}}
	src := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src", UID: types.UID("suid"), Generation: 1}}
	dst := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dst", UID: types.UID("duid"), Generation: 1}}
	nm := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm", UID: types.UID("nmuid"), Generation: 1}}
	sm := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm", UID: types.UID("smuid"), Generation: 1}}
	plan.Referenced.Provider.Source = src
	plan.Referenced.Provider.Destination = dst
	plan.Referenced.Map.Network = nm
	plan.Referenced.Map.Storage = sm
	mig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("muid"), Generation: 1}}
	ctx := &plancontext.Context{Plan: plan, Migration: mig}
	_ = r.newSnapshot(ctx)

	plan.Status.SetCondition(condition.Condition{Type: Executing, Status: condition.True})
	snap := plan.Status.Migration.ActiveSnapshot()
	snap.SetCondition(condition.Condition{Type: Executing, Status: condition.True})

	// Mismatch by bumping plan generation.
	plan.Generation = 2
	matched := r.matchSnapshot(ctx)
	if matched {
		t.Fatalf("expected mismatch")
	}
	if !snap.HasCondition(Canceled) {
		t.Fatalf("expected snapshot canceled")
	}
	if plan.Status.HasCondition(Executing) {
		t.Fatalf("expected plan executing cleared")
	}
}

func TestReconciler_matchSnapshot_MismatchSourceProvider_Cancels(t *testing.T) {
	r := &Reconciler{}
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: types.UID("puid"), Generation: 1}}
	src := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src", UID: types.UID("suid"), Generation: 1}}
	dst := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dst", UID: types.UID("duid"), Generation: 1}}
	nm := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm", UID: types.UID("nmuid"), Generation: 1}}
	sm := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm", UID: types.UID("smuid"), Generation: 1}}
	plan.Referenced.Provider.Source = src
	plan.Referenced.Provider.Destination = dst
	plan.Referenced.Map.Network = nm
	plan.Referenced.Map.Storage = sm
	mig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("muid"), Generation: 1}}
	ctx := &plancontext.Context{Plan: plan, Migration: mig}
	_ = r.newSnapshot(ctx)

	// mismatch source provider generation
	src.Generation = 2
	matched := r.matchSnapshot(ctx)
	if matched {
		t.Fatalf("expected mismatch")
	}
	if !plan.Status.Migration.ActiveSnapshot().HasCondition(Canceled) {
		t.Fatalf("expected canceled")
	}
}

func TestReconciler_matchSnapshot_MismatchDestinationProvider_Cancels(t *testing.T) {
	r := &Reconciler{}
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: types.UID("puid"), Generation: 1}}
	src := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src", UID: types.UID("suid"), Generation: 1}}
	dst := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dst", UID: types.UID("duid"), Generation: 1}}
	nm := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm", UID: types.UID("nmuid"), Generation: 1}}
	sm := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm", UID: types.UID("smuid"), Generation: 1}}
	plan.Referenced.Provider.Source = src
	plan.Referenced.Provider.Destination = dst
	plan.Referenced.Map.Network = nm
	plan.Referenced.Map.Storage = sm
	mig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("muid"), Generation: 1}}
	ctx := &plancontext.Context{Plan: plan, Migration: mig}
	_ = r.newSnapshot(ctx)

	dst.Generation = 2
	matched := r.matchSnapshot(ctx)
	if matched {
		t.Fatalf("expected mismatch")
	}
	if !plan.Status.Migration.ActiveSnapshot().HasCondition(Canceled) {
		t.Fatalf("expected canceled")
	}
}

func TestReconciler_matchSnapshot_MismatchNetworkMap_Cancels(t *testing.T) {
	r := &Reconciler{}
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: types.UID("puid"), Generation: 1}}
	src := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src", UID: types.UID("suid"), Generation: 1}}
	dst := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dst", UID: types.UID("duid"), Generation: 1}}
	nm := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm", UID: types.UID("nmuid"), Generation: 1}}
	sm := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm", UID: types.UID("smuid"), Generation: 1}}
	plan.Referenced.Provider.Source = src
	plan.Referenced.Provider.Destination = dst
	plan.Referenced.Map.Network = nm
	plan.Referenced.Map.Storage = sm
	mig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("muid"), Generation: 1}}
	ctx := &plancontext.Context{Plan: plan, Migration: mig}
	_ = r.newSnapshot(ctx)

	nm.Generation = 2
	matched := r.matchSnapshot(ctx)
	if matched {
		t.Fatalf("expected mismatch")
	}
	if !plan.Status.Migration.ActiveSnapshot().HasCondition(Canceled) {
		t.Fatalf("expected canceled")
	}
}

func TestReconciler_matchSnapshot_MismatchStorageMap_Cancels(t *testing.T) {
	r := &Reconciler{}
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: types.UID("puid"), Generation: 1}}
	src := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src", UID: types.UID("suid"), Generation: 1}}
	dst := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dst", UID: types.UID("duid"), Generation: 1}}
	nm := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm", UID: types.UID("nmuid"), Generation: 1}}
	sm := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm", UID: types.UID("smuid"), Generation: 1}}
	plan.Referenced.Provider.Source = src
	plan.Referenced.Provider.Destination = dst
	plan.Referenced.Map.Network = nm
	plan.Referenced.Map.Storage = sm
	mig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("muid"), Generation: 1}}
	ctx := &plancontext.Context{Plan: plan, Migration: mig}
	_ = r.newSnapshot(ctx)

	sm.Generation = 2
	matched := r.matchSnapshot(ctx)
	if matched {
		t.Fatalf("expected mismatch")
	}
	if !plan.Status.Migration.ActiveSnapshot().HasCondition(Canceled) {
		t.Fatalf("expected canceled")
	}
}

func TestReconciler_activeMigration_ReturnsNilWhenSnapshotTerminal(t *testing.T) {
	r := &Reconciler{}
	plan := &api.Plan{}
	plan.Status.Migration.NewSnapshot(planapi.Snapshot{})
	snap := plan.Status.Migration.ActiveSnapshot()
	snap.SetCondition(condition.Condition{Type: Succeeded, Status: condition.True})
	m, err := r.activeMigration(plan)
	if err != nil || m != nil {
		t.Fatalf("expected nil,nil got %v,%v", m, err)
	}
}

func TestReconciler_activeMigration_WhenMigrationNil_DeletesExecutingOnSnapshot(t *testing.T) {
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	migRef := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("u1"), Generation: 1}}
	plan.Status.Migration.NewSnapshot(planapi.Snapshot{})
	snap := plan.Status.Migration.ActiveSnapshot()
	snap.Migration.With(migRef)
	snap.SetCondition(condition.Condition{Type: Executing, Status: condition.True})

	r, _ := newPlanReconciler(t /* no migration object */)
	_, _ = r.activeMigration(plan)
	if snap.HasCondition(Executing) {
		t.Fatalf("expected executing cleared when migration missing")
	}
}

func TestReconciler_activeMigration_NotFoundMarksSnapshotCanceled(t *testing.T) {
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	migRef := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("u1"), Generation: 1}}
	plan.Status.Migration.NewSnapshot(planapi.Snapshot{})
	snap := plan.Status.Migration.ActiveSnapshot()
	snap.Migration.With(migRef)

	r, _ := newPlanReconciler(t /* no migration object */)
	m, err := r.activeMigration(plan)
	if err != nil || m != nil {
		t.Fatalf("expected nil,nil got %v,%v", m, err)
	}
	if !snap.HasCondition(Canceled) {
		t.Fatalf("expected snapshot canceled when migration missing")
	}
}

func TestReconciler_activeMigration_UIDMismatchMarksCanceled(t *testing.T) {
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	// Snapshot expects UID=u1, but object in cluster is UID=u2.
	snapMig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("u1"), Generation: 1}}
	clusterMig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: types.UID("u2"), Generation: 1}}
	plan.Status.Migration.NewSnapshot(planapi.Snapshot{})
	snap := plan.Status.Migration.ActiveSnapshot()
	snap.Migration.With(snapMig)

	r, _ := newPlanReconciler(t, clusterMig)
	m, err := r.activeMigration(plan)
	if err != nil || m != nil {
		t.Fatalf("expected nil,nil got %v,%v", m, err)
	}
	if !snap.HasCondition(Canceled) {
		t.Fatalf("expected snapshot canceled on UID mismatch")
	}
}

func TestReconciler_activeMigration_OtherGetErrorPropagates(t *testing.T) {
	plan := &api.Plan{}
	plan.Status.Migration.NewSnapshot(planapi.Snapshot{})
	snap := plan.Status.Migration.ActiveSnapshot()
	snap.Migration.Namespace = "ns"
	snap.Migration.Name = "m"
	snap.Migration.UID = types.UID("u1")

	r, cl := newPlanReconciler(t)
	// Wrap client to force error on Get.
	r.Client = &errGetClient{Client: cl, err: context.DeadlineExceeded}
	_, err := r.activeMigration(plan)
	if err == nil {
		t.Fatalf("expected error")
	}
}

type errGetClient struct {
	client.Client
	err error
}

func (e *errGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return e.err
}

func TestReconciler_pendingMigrations_FiltersAndSorts(t *testing.T) {
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	// Active snapshot canceled for uid=ignore.
	plan.Status.Migration.NewSnapshot(planapi.Snapshot{})
	plan.Status.Migration.ActiveSnapshot().Migration.UID = types.UID("ignore")
	plan.Status.Migration.ActiveSnapshot().SetCondition(condition.Condition{Type: Canceled, Status: condition.True})

	m1 := api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "b", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))}}
	m1.Spec.Plan = corev1.ObjectReference{Namespace: "ns", Name: "p"}
	m2 := api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "a", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))}}
	m2.Spec.Plan = corev1.ObjectReference{Namespace: "ns", Name: "p"}
	m3 := api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "other", Name: "x"}}
	m3.Spec.Plan = corev1.ObjectReference{Namespace: "other", Name: "p"}
	m4 := api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "term"}}
	m4.Spec.Plan = corev1.ObjectReference{Namespace: "ns", Name: "p"}
	m4.Status.SetCondition(condition.Condition{Type: Succeeded, Status: condition.True})

	listObj := &api.MigrationList{Items: []api.Migration{m1, m2, m3, m4}}

	r, cl := newPlanReconciler(t, listObj)
	// fake client stores list items as runtime objects individually; add them too.
	_ = cl.Create(context.Background(), &listObj.Items[0])
	_ = cl.Create(context.Background(), &listObj.Items[1])
	_ = cl.Create(context.Background(), &listObj.Items[2])
	_ = cl.Create(context.Background(), &listObj.Items[3])

	got, err := r.pendingMigrations(plan)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(got))
	}
	// same timestamp => sort by namespace/name
	if got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("unexpected order: %s,%s", got[0].Name, got[1].Name)
	}
}

func TestReconciler_pendingMigrations_IgnoresMigrationWhenSnapshotCanceledForSameUID(t *testing.T) {
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	plan.Status.Migration.NewSnapshot(planapi.Snapshot{})
	snap := plan.Status.Migration.ActiveSnapshot()
	snap.Migration.UID = types.UID("u1")
	snap.SetCondition(condition.Condition{Type: Canceled, Status: condition.True})

	m1 := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m1", UID: types.UID("u1")}}
	m1.Spec.Plan = corev1.ObjectReference{Namespace: "ns", Name: "p"}
	m2 := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m2", UID: types.UID("u2")}}
	m2.Spec.Plan = corev1.ObjectReference{Namespace: "ns", Name: "p"}

	r, _ := newPlanReconciler(t, m1, m2)
	got, err := r.pendingMigrations(plan)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 || got[0].UID != types.UID("u2") {
		t.Fatalf("unexpected pending: %#v", got)
	}
}

func TestReconciler_postpone_ReturnsTrueWhenHostNotReconciled(t *testing.T) {
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	p.Generation = 1
	p.Status.ObservedGeneration = 1
	h := &api.Host{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h"}}
	h.Generation = 2
	h.Status.ObservedGeneration = 1
	r, _ := newPlanReconciler(t, p, h)
	postpone, err := r.postpone()
	if err != nil || !postpone {
		t.Fatalf("expected postpone true nil, got %v %v", postpone, err)
	}
}

func TestReconciler_postpone_ReturnsTrueWhenHookNotReconciled(t *testing.T) {
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	p.Generation = 1
	p.Status.ObservedGeneration = 1
	hk := &api.Hook{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "hk"}}
	hk.Generation = 2
	hk.Status.ObservedGeneration = 1
	r, _ := newPlanReconciler(t, p, hk)
	postpone, err := r.postpone()
	if err != nil || !postpone {
		t.Fatalf("expected postpone true nil, got %v %v", postpone, err)
	}
}

func TestReconciler_postpone_ReturnsTrueWhenProviderNotReconciled(t *testing.T) {
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	p.Generation = 2
	p.Status.ObservedGeneration = 1
	r, _ := newPlanReconciler(t, p)
	postpone, err := r.postpone()
	if err != nil || !postpone {
		t.Fatalf("expected postpone true nil, got %v %v", postpone, err)
	}
}

func TestReconciler_postpone_ReturnsTrueWhenNetworkMapNotReconciled(t *testing.T) {
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	p.Generation = 1
	p.Status.ObservedGeneration = 1
	nm := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm"}}
	nm.Generation = 2
	nm.Status.ObservedGeneration = 1
	r, _ := newPlanReconciler(t, p, nm)
	postpone, err := r.postpone()
	if err != nil || !postpone {
		t.Fatalf("expected postpone true nil, got %v %v", postpone, err)
	}
}

func TestReconciler_postpone_StorageMapNotReconciled_IsNotDetected_CurrentBehavior(t *testing.T) {
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	p.Generation = 1
	p.Status.ObservedGeneration = 1
	// No network maps => loop that (incorrectly) checks netMapList for storage maps won't run.
	sm := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm"}}
	sm.Generation = 2
	sm.Status.ObservedGeneration = 1
	r, _ := newPlanReconciler(t, p, sm)
	postpone, err := r.postpone()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if postpone {
		t.Fatalf("expected postpone=false due to current bug (storage maps not checked)")
	}
}

func TestReconciler_updatePlanStatus_SetsObservedGenerationAndUpdatesStatus(t *testing.T) {
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	plan.Generation = 7
	r, cl := newPlanReconciler(t, plan)

	if err := r.updatePlanStatus(plan); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	got := &api.Plan{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "p"}, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.ObservedGeneration != 7 {
		t.Fatalf("expected observedGeneration=7 got %d", got.Status.ObservedGeneration)
	}
}

func TestReconciler_updatePlanStatus_PropagatesStatusUpdateError(t *testing.T) {
	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	r, cl := newPlanReconciler(t, plan)
	r.Client = &errStatusClient{Client: cl, err: k8serr.NewForbidden(corev1.Resource("plans"), "p", nil)}
	err := r.updatePlanStatus(plan)
	if err == nil {
		t.Fatalf("expected error")
	}
}

type errStatusClient struct {
	client.Client
	err error
}

func (e *errStatusClient) Status() client.StatusWriter { return errStatusWriter{err: e.err} }

type errStatusWriter struct{ err error }

func (e errStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return e.err
}
func (e errStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return e.err
}
func (e errStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return e.err
}
