package ova

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func execPlan(name string, srcRef core.ObjectReference, vms ...*planapi.VMStatus) *api.Plan {
	p := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: name}}
	p.Spec.Provider.Source = srcRef
	p.Status.Migration.VMs = vms
	s := planapi.Snapshot{}
	s.SetCondition(libcnd.Condition{Type: "Executing", Status: libcnd.True})
	p.Status.Migration.History = []planapi.Snapshot{s}
	return p
}

func runningVM(id string) *planapi.VMStatus {
	vm := &planapi.VMStatus{}
	vm.ID = id
	vm.MarkStarted()
	return vm
}

func pendingVM(id string) *planapi.VMStatus {
	vm := &planapi.VMStatus{}
	vm.ID = id
	return vm
}

func canceledVM(id string) *planapi.VMStatus {
	vm := pendingVM(id)
	vm.SetCondition(libcnd.Condition{Type: Canceled, Status: libcnd.True})
	return vm
}

func TestNext_ReturnsNoVMWhenInFlightAtOrAboveMax(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	src := core.ObjectReference{Namespace: "ns", Name: "a"}
	plan := execPlan("p", src, pendingVM("1"))
	other := execPlan("p1", src, runningVM("x"))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(plan, other).Build()
	ctx := &plancontext.Context{Client: cl, Plan: plan, Log: logging.WithName("t")}
	s := &Scheduler{Context: ctx, MaxInFlight: 1}
	_, has, err := s.Next()
	if err != nil || has {
		t.Fatalf("expected no next, got has=%v err=%v", has, err)
	}
}

func TestNext_ReturnsFirstPendingVM(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	src := core.ObjectReference{Namespace: "ns", Name: "a"}
	plan := execPlan("p", src, pendingVM("1"), pendingVM("2"))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(plan).Build()
	ctx := &plancontext.Context{Client: cl, Plan: plan, Log: logging.WithName("t")}
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	vm, has, err := s.Next()
	if err != nil || !has || vm == nil || vm.ID != "1" {
		t.Fatalf("unexpected: has=%v err=%v vm=%#v", has, err, vm)
	}
}

func TestNext_SkipsCanceledVMs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	src := core.ObjectReference{Namespace: "ns", Name: "a"}
	plan := execPlan("p", src, canceledVM("1"), pendingVM("2"))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(plan).Build()
	ctx := &plancontext.Context{Client: cl, Plan: plan, Log: logging.WithName("t")}
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	vm, has, err := s.Next()
	if err != nil || !has || vm == nil || vm.ID != "2" {
		t.Fatalf("unexpected: has=%v err=%v vm=%#v", has, err, vm)
	}
}

func TestNext_SkipsStartedAndCompletedVMs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	src := core.ObjectReference{Namespace: "ns", Name: "a"}
	started := pendingVM("1")
	started.MarkStarted()
	completed := pendingVM("2")
	completed.MarkCompleted()
	plan := execPlan("p", src, started, completed, pendingVM("3"))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(plan).Build()
	ctx := &plancontext.Context{Client: cl, Plan: plan, Log: logging.WithName("t")}
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	vm, has, err := s.Next()
	if err != nil || !has || vm == nil || vm.ID != "3" {
		t.Fatalf("unexpected: has=%v err=%v vm=%#v", has, err, vm)
	}
}
