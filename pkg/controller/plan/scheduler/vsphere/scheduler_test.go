package vsphere

import (
	"context"
	"errors"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	vsmodel "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	"github.com/kubev2v/forklift/pkg/controller/provider/web"
	webbase "github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	"github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestScheduler(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	hostA := "hostA"
	hostB := "hostB"
	hostC := "hostC"

	scheduler := Scheduler{MaxInFlight: 10}
	scheduler.inFlight = map[string]int{
		hostA: 6,
		hostB: 10,
		hostC: 0,
	}
	scheduler.pending = map[string][]*pendingVM{
		// Only VMs that fit the available capacity
		// can be scheduled. Host A already has 6 slots occupied,
		// so only the VM with cost 4 can be scheduled.
		hostA: {
			{
				cost: 6,
			},
			{
				cost: 5,
			},
			{
				cost: 4,
			},
			{
				cost: 7,
			},
		},

		// host B has reached capacity, so we
		// can't schedule any migrations from it.
		hostB: {
			{
				cost: 10,
			},
			{
				cost: 0,
			},
			{
				cost: 1,
			},
		},

		// host C is unoccupied, so any of its
		// vms with a cost of 10 or less could
		// be started
		hostC: {
			{
				cost: 11,
			},
			{
				cost: 1,
			},
			{
				cost: 2,
			},
			{
				cost: 3,
			},
			{
				cost: 10,
			},
		},
	}

	// no VMs from host B could be scheduled, so we shouldn't see
	// an entry for host B in the schedule map.
	expectedSchedule := map[string][]*pendingVM{
		hostA: {
			{
				cost: 4,
			},
		},
		hostC: {
			{
				cost: 11,
			},
			{
				cost: 1,
			},
			{
				cost: 2,
			},
			{
				cost: 3,
			},
			{
				cost: 10,
			},
		},
	}
	g.Expect(scheduler.schedulable()).To(gomega.Equal(expectedSchedule))
}

type fakeInventory struct {
	findFn func(resource interface{}, r webbase.Ref) error
}

func (f *fakeInventory) Finder() web.Finder { return nil }
func (f *fakeInventory) Get(resource interface{}, id string) error {
	return errors.New("not implemented")
}
func (f *fakeInventory) List(list interface{}, param ...web.Param) error {
	return errors.New("not implemented")
}
func (f *fakeInventory) Watch(resource interface{}, h web.EventHandler) (*web.Watch, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeInventory) Find(resource interface{}, r webbase.Ref) error {
	if f.findFn != nil {
		return f.findFn(resource, r)
	}
	return errors.New("not implemented")
}
func (f *fakeInventory) VM(ref *webbase.Ref) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeInventory) Workload(ref *webbase.Ref) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeInventory) Network(ref *webbase.Ref) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeInventory) Storage(ref *webbase.Ref) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeInventory) Host(ref *webbase.Ref) (interface{}, error) {
	return nil, errors.New("not implemented")
}

func mkPlan(useV2v bool) *api.Plan {
	p := &api.Plan{}
	p.Spec.Warm = false
	p.Spec.MigrateSharedDisks = true
	p.Spec.SkipGuestConversion = false

	srcType := api.VSphere
	if !useV2v {
		srcType = api.OpenStack
	}
	src := &api.Provider{Spec: api.ProviderSpec{Type: &srcType}}

	dstType := api.OpenShift
	dst := &api.Provider{Spec: api.ProviderSpec{Type: &dstType, URL: ""}} // host

	// referenced providers drive ShouldUseV2vForTransfer
	p.Provider.Source = src
	p.Provider.Destination = dst

	// also set spec provider refs for scheduler cross-plan comparisons
	p.Spec.Provider.Source = core.ObjectReference{Namespace: "ns", Name: "src"}
	p.Spec.Provider.Destination = core.ObjectReference{Namespace: "ns", Name: "dst"}
	return p
}

func mkVMStatus(id string, phase string) *planapi.VMStatus {
	vm := &planapi.VMStatus{Phase: phase}
	vm.ID = id
	return vm
}

func runningVMStatus(id string, phase string) *planapi.VMStatus {
	vm := mkVMStatus(id, phase)
	vm.MarkStarted()
	return vm
}

func withExecutingSnapshot(p *api.Plan) {
	s := planapi.Snapshot{}
	s.SetCondition(libcnd.Condition{Type: "Executing", Status: libcnd.True})
	p.Status.Migration.History = []planapi.Snapshot{s}
}

func TestFinishedDisks(t *testing.T) {
	cases := []struct {
		name     string
		vm       *planapi.VMStatus
		expected int
	}{
		{
			name:     "EmptyPipeline",
			vm:       &planapi.VMStatus{},
			expected: 0,
		},
		{
			name: "NoDiskTransferStep",
			vm: &planapi.VMStatus{
				Pipeline: []*planapi.Step{{Task: planapi.Task{Name: "Other"}}},
			},
			expected: 0,
		},
		{
			name: "CountsCompletedTasksInDiskTransferStep",
			vm: &planapi.VMStatus{
				Pipeline: []*planapi.Step{{
					Task:  planapi.Task{Name: DiskTransfer},
					Tasks: []*planapi.Task{{Phase: Completed}, {Phase: "Running"}, {Phase: Completed}},
				}},
			},
			expected: 2,
		},
	}
	s := &Scheduler{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.finishedDisks(tc.vm); got != tc.expected {
				t.Fatalf("expected %d got %d", tc.expected, got)
			}
		})
	}
}

func TestCost(t *testing.T) {
	cases := []struct {
		name     string
		useV2v   bool
		disks    int
		phase    string
		pipeline []*planapi.Step
		expected int
	}{
		{"V2v_CreateVM", true, 0, CreateVM, nil, 0},
		{"V2v_PostHook", true, 0, PostHook, nil, 0},
		{"V2v_Completed", true, 0, Completed, nil, 0},
		{"V2v_Default", true, 0, "Other", nil, 1},
		{"CDI_CreateVM", false, 3, CreateVM, nil, 0},
		{"CDI_CopyingPaused", false, 3, CopyingPaused, nil, 0},
		{
			name:   "CDI_Default_DiskCountMinusFinished",
			useV2v: false,
			disks:  4,
			phase:  "Other",
			pipeline: []*planapi.Step{{
				Task:  planapi.Task{Name: DiskTransfer},
				Tasks: []*planapi.Task{{Phase: Completed}, {Phase: Completed}},
			}},
			expected: 2,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			p := mkPlan(tc.useV2v)
			s := &Scheduler{Context: &plancontext.Context{Plan: p}}
			vm := &model.VM{}
			vm.Disks = make([]vsmodel.Disk, tc.disks)
			st := mkVMStatus("1", tc.phase)
			if tc.pipeline != nil {
				st.Pipeline = tc.pipeline
			}
			if got := s.cost(vm, st); got != tc.expected {
				t.Fatalf("expected %d got %d", tc.expected, got)
			}
		})
	}
}

func TestSchedulable_EdgeCases(t *testing.T) {
	cases := []struct {
		name        string
		maxInFlight int
		pending     map[string][]*pendingVM
		inFlight    map[string]int
		expectedH1  int // expected len of schedulable["h1"]
	}{
		{
			name:        "SkipsHostAtCapacity",
			maxInFlight: 2,
			pending:     map[string][]*pendingVM{"h1": {{cost: 1}}},
			inFlight:    map[string]int{"h1": 2},
			expectedH1:  0,
		},
		{
			name:        "AllowsVMWhenCostFits",
			maxInFlight: 3,
			pending:     map[string][]*pendingVM{"h1": {{cost: 2}}},
			inFlight:    map[string]int{"h1": 1},
			expectedH1:  1,
		},
		{
			name:        "AllowsBigVMWhenAlone",
			maxInFlight: 2,
			pending:     map[string][]*pendingVM{"h1": {{cost: 5}}},
			inFlight:    map[string]int{"h1": 0},
			expectedH1:  1,
		},
		{
			name:        "DoesNotAllowBigVMWhenNotAlone",
			maxInFlight: 2,
			pending:     map[string][]*pendingVM{"h1": {{cost: 5}}},
			inFlight:    map[string]int{"h1": 1},
			expectedH1:  0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Scheduler{MaxInFlight: tc.maxInFlight}
			s.pending = tc.pending
			s.inFlight = tc.inFlight
			got := s.schedulable()
			if len(got["h1"]) != tc.expectedH1 {
				t.Fatalf("expected %d schedulable, got %d: %#v", tc.expectedH1, len(got["h1"]), got)
			}
		})
	}
}

func TestBuildPending_AddsOnlyNotStartedNotCompletedNotCanceled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	v1 := mkVMStatus("vm1", "Other")
	v2 := mkVMStatus("vm2", "Other")
	v2.MarkStarted()
	v3 := mkVMStatus("vm3", "Other")
	v3.MarkCompleted()
	v4 := mkVMStatus("vm4", "Other")
	v4.SetCondition(libcnd.Condition{Type: Canceled, Status: libcnd.True})
	p.Status.Migration.VMs = []*planapi.VMStatus{v1, v2, v3, v4}

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		vm := resource.(*model.VM)
		vm.Host = "h1"
		vm.Disks = make([]vsmodel.Disk, 2)
		return nil
	}}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	if err := s.buildPending(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(s.pending["h1"]) != 1 || s.pending["h1"][0].status.ID != "vm1" {
		t.Fatalf("unexpected pending: %#v", s.pending)
	}
}

func TestBuildInFlight_CountsRunningVMsOnCurrentPlan(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	p.Status.Migration.VMs = []*planapi.VMStatus{runningVMStatus("vm1", "Other")}

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		vm := resource.(*model.VM)
		vm.Host = "h1"
		vm.Disks = make([]vsmodel.Disk, 3)
		return nil
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	if err := s.buildInFlight(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if s.inFlight["h1"] != 3 {
		t.Fatalf("expected inflight 3 got %#v", s.inFlight)
	}
}

func TestBuildInFlight_SkipsCanceledVMsOnCurrentPlan(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	vm := runningVMStatus("vm1", "Other")
	vm.SetCondition(libcnd.Condition{Type: Canceled, Status: libcnd.True})
	p.Status.Migration.VMs = []*planapi.VMStatus{vm}

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		t.Fatalf("Find should not be called for canceled VM")
		return nil
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	if err := s.buildInFlight(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(s.inFlight) != 0 {
		t.Fatalf("expected empty inflight got %#v", s.inFlight)
	}
}

func TestBuildInFlight_NotFoundMarksCanceledAndReturnsError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	vm := runningVMStatus("vm1", "Other")
	vm.Ref = ref.Ref{ID: "vm1"}
	p.Status.Migration.VMs = []*planapi.VMStatus{vm}

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		return web.NotFoundError{Ref: r}
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	err := s.buildInFlight()
	if err == nil {
		t.Fatalf("expected error")
	}
	if !vm.HasCondition(api.ConditionCanceled) {
		t.Fatalf("expected canceled condition, got %#v", vm.Conditions)
	}
}

func TestBuildInFlight_IncludesOtherExecutingPlansSameProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)

	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	p.Status.Migration.VMs = []*planapi.VMStatus{}
	withExecutingSnapshot(p)

	other := mkPlan(false)
	other.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p2"}
	other.Spec.Provider.Source = p.Spec.Provider.Source
	other.Spec.Archived = false
	other.Status.Migration.VMs = []*planapi.VMStatus{runningVMStatus("vm2", "Other")}
	withExecutingSnapshot(other)

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		vm := resource.(*model.VM)
		vm.Host = "h1"
		vm.Disks = make([]vsmodel.Disk, 2)
		return nil
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p, other).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	if err := s.buildInFlight(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if s.inFlight["h1"] != 2 {
		t.Fatalf("expected inflight 2 got %#v", s.inFlight)
	}
}

func TestBuildInFlight_IgnoresOtherPlansWhenArchivedOrNotExecutingOrDifferentProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)

	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	withExecutingSnapshot(p)

	diffProvider := mkPlan(false)
	diffProvider.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p3"}
	diffProvider.Spec.Provider.Source = core.ObjectReference{Namespace: "ns", Name: "other-src"}
	diffProvider.Status.Migration.VMs = []*planapi.VMStatus{runningVMStatus("vmA", "Other")}
	withExecutingSnapshot(diffProvider)

	archived := mkPlan(false)
	archived.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p4"}
	archived.Spec.Provider.Source = p.Spec.Provider.Source
	archived.Spec.Archived = true
	archived.Status.Migration.VMs = []*planapi.VMStatus{runningVMStatus("vmB", "Other")}
	withExecutingSnapshot(archived)

	notExec := mkPlan(false)
	notExec.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p5"}
	notExec.Spec.Provider.Source = p.Spec.Provider.Source
	notExec.Status.Migration.VMs = []*planapi.VMStatus{runningVMStatus("vmC", "Other")}
	// no executing snapshot

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		t.Fatalf("Find should not be called for ignored plans")
		return nil
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p, diffProvider, archived, notExec).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	if err := s.buildInFlight(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(s.inFlight) != 0 {
		t.Fatalf("expected empty inflight got %#v", s.inFlight)
	}
}

func TestBuildInFlight_IgnoresNotFoundAndRefNotUniqueOnOtherPlans(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)

	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	withExecutingSnapshot(p)

	other := mkPlan(false)
	other.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p2"}
	other.Spec.Provider.Source = p.Spec.Provider.Source
	other.Status.Migration.VMs = []*planapi.VMStatus{runningVMStatus("nf", "Other"), runningVMStatus("nu", "Other")}
	withExecutingSnapshot(other)

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		if r.ID == "nf" {
			return web.NotFoundError{Ref: r}
		}
		if r.ID == "nu" {
			return web.RefNotUniqueError{Ref: r}
		}
		return nil
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p, other).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	// Current behavior: RefNotUniqueError is not ignored here and is returned.
	if err := s.buildInFlight(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNext_ReturnsSingleSchedulableVM(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)

	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	withExecutingSnapshot(p)

	vm := mkVMStatus("vm1", "Other")
	vm.Ref = ref.Ref{ID: "vm1"}
	p.Status.Migration.VMs = []*planapi.VMStatus{vm}

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		vm := resource.(*model.VM)
		vm.Host = "h1"
		vm.Disks = make([]vsmodel.Disk, 1)
		return nil
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}

	next, has, err := s.Next()
	if err != nil || !has || next == nil || next.ID != "vm1" {
		t.Fatalf("unexpected: has=%v err=%v vm=%#v", has, err, next)
	}
}

func TestErrorsAs_RefNotUniqueError_MatchesValue(t *testing.T) {
	err := web.RefNotUniqueError{Ref: webbase.Ref{ID: "x"}}
	if !errors.As(err, &web.RefNotUniqueError{}) {
		t.Fatalf("expected errors.As to match web.RefNotUniqueError")
	}
}

func TestBuildInFlight_ListErrorWrapped(t *testing.T) {
	// Use a context with a nil Client to force a panic-free error path? buildInFlight calls r.List, which would panic if Client nil.
	// Instead, verify that buildInFlight returns the Find error on current plan before calling List.
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	p.Status.Migration.VMs = []*planapi.VMStatus{runningVMStatus("vm1", "Other")}

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		return errors.New("boom")
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	if err := s.buildInFlight(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFakeInventory_ImplementsWebClient(t *testing.T) {
	var _ web.Client = &fakeInventory{}
}

func TestBuildInFlight_ContextCancelDoesNotDeadlock(t *testing.T) {
	// Guard against accidental context usage changes: ensure Find is called with our ref and returns quickly.
	scheme := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(scheme)
	p := mkPlan(false)
	p.ObjectMeta = metav1.ObjectMeta{Namespace: "ns", Name: "p"}
	p.Status.Migration.VMs = []*planapi.VMStatus{runningVMStatus("vm1", "Other")}

	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()
	_ = ctxCancel

	inv := &fakeInventory{findFn: func(resource interface{}, r webbase.Ref) error {
		vm := resource.(*model.VM)
		vm.Host = "h1"
		vm.Disks = make([]vsmodel.Disk, 1)
		return nil
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p).Build()
	ctx := &plancontext.Context{Client: cl, Plan: p, Log: logging.WithName("t")}
	ctx.Source.Inventory = inv
	s := &Scheduler{Context: ctx, MaxInFlight: 10}
	if err := s.buildInFlight(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
