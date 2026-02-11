package plan

import (
	"reflect"
	"testing"

	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	libitr "github.com/kubev2v/forklift/pkg/lib/itinerary"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestDeepCopy_VMStatus(t *testing.T) {
	now := metav1.Now()
	vm := &VMStatus{
		VM: VM{
			Ref:        refapi.Ref{ID: "vm-1"},
			TargetName: "target",
		},
		Phase: "Started",
		Warm: &Warm{
			Successes:     1,
			NextPrecopyAt: &now,
		},
		Pipeline: []*Step{
			{
				Task: Task{
					Name:        "step1",
					Annotations: map[string]string{"k": "v"},
				},
				Tasks: []*Task{
					{Name: "task1"},
				},
			},
		},
	}

	cp := vm.DeepCopy()
	if cp == nil || cp == vm {
		t.Fatalf("DeepCopy returned invalid copy: %#v", cp)
	}
	if cp.ID != "vm-1" || cp.TargetName != "target" || cp.Phase != "Started" {
		t.Fatalf("unexpected deepcopy values: %#v", cp)
	}

	// Mutate copy, ensure original not affected (maps/slices).
	cp.Pipeline[0].Annotations["k"] = "changed"
	if vm.Pipeline[0].Annotations["k"] != "v" {
		t.Fatalf("expected annotations map to be deep-copied")
	}
	cp.Pipeline[0].Tasks[0].Name = "changed"
	if vm.Pipeline[0].Tasks[0].Name != "task1" {
		t.Fatalf("expected nested tasks slice to be deep-copied")
	}
}

func TestTimed_MarkAndRunning(t *testing.T) {
	var td Timed
	if td.MarkedStarted() || td.MarkedCompleted() || td.Running() {
		t.Fatalf("expected initial false state")
	}

	td.MarkStarted()
	if !td.MarkedStarted() || td.MarkedCompleted() || !td.Running() {
		t.Fatalf("unexpected state after MarkStarted: %#v", td)
	}

	// Idempotent start.
	startedAt := td.Started
	td.MarkStarted()
	if td.Started != startedAt {
		t.Fatalf("expected MarkStarted to be idempotent")
	}

	td.MarkCompleted()
	if !td.MarkedStarted() || !td.MarkedCompleted() || td.Running() {
		t.Fatalf("unexpected state after MarkCompleted: %#v", td)
	}

	td.MarkReset()
	if td.Started != nil || td.Completed != nil {
		t.Fatalf("expected reset state")
	}
}

func TestSnapshotRef_WithAndMatch(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "ns",
			Name:       "p1",
			UID:        types.UID("uid1"),
			Generation: 7,
		},
	}
	var ref SnapshotRef
	ref.With(pod)
	if ref.Namespace != "ns" || ref.Name != "p1" || ref.UID != types.UID("uid1") || ref.Generation != 7 {
		t.Fatalf("unexpected ref: %#v", ref)
	}
	if !ref.Match(pod) {
		t.Fatalf("expected match")
	}
	pod.Generation = 8
	if ref.Match(pod) {
		t.Fatalf("expected mismatch after generation change")
	}
}

func TestHookRef_StringAndVM_FindHook(t *testing.T) {
	vm := &VM{
		Ref: refapi.Ref{ID: "vm-1"},
		Hooks: []HookRef{
			{Step: "PreHook", Hook: corev1.ObjectReference{Namespace: "ns", Name: "h1"}},
			{Step: "PostHook", Hook: corev1.ObjectReference{Namespace: "ns", Name: "h2"}},
		},
	}
	h, found := vm.FindHook("PostHook")
	if !found || h.Hook.Name != "h2" {
		t.Fatalf("unexpected hook: found=%v ref=%#v", found, h)
	}
	if got := h.String(); got != "ns/h2 @PostHook" {
		t.Fatalf("unexpected string: %s", got)
	}
	_, found = vm.FindHook("Missing")
	if found {
		t.Fatalf("expected not found")
	}
}

func TestErrorsAndTasks(t *testing.T) {
	e := &Error{}
	e.Add("a", "b", "a")
	if len(e.Reasons) != 2 {
		t.Fatalf("expected de-duplicated reasons, got %#v", e.Reasons)
	}

	task := &Task{Name: "t1", Phase: "P"}
	if task.HasError() {
		t.Fatalf("expected no error")
	}
	task.AddError("x", "x", "y")
	if !task.HasError() || task.Error == nil || task.Error.Phase != "P" {
		t.Fatalf("unexpected task error: %#v", task.Error)
	}
	if len(task.Error.Reasons) != 2 {
		t.Fatalf("expected deduped reasons, got %#v", task.Error.Reasons)
	}
}

func TestStep_FindTask_AndReflectTasks(t *testing.T) {
	t1 := &Task{Name: "t1", Progress: libitr.Progress{Completed: 2}}
	t1.MarkStarted()
	t2 := &Task{Name: "t2", Progress: libitr.Progress{Completed: 3}}
	t2.MarkCompleted()
	t2.AddError("boom")

	step := &Step{
		Task: Task{Name: "step"},
		Tasks: []*Task{
			t1,
			t2,
		},
	}

	got, found := step.FindTask("t2")
	if !found || got == nil || got.Name != "t2" {
		t.Fatalf("unexpected FindTask result: found=%v task=%#v", found, got)
	}

	step.ReflectTasks()
	if step.Progress.Completed != 5 {
		t.Fatalf("unexpected completed: %d", step.Progress.Completed)
	}
	if !step.MarkedStarted() {
		t.Fatalf("expected step to be marked started")
	}
	if step.Error == nil || !reflect.DeepEqual(step.Error.Reasons, []string{"boom"}) {
		t.Fatalf("expected error to be reflected, got %#v", step.Error)
	}

	// Only one task is completed at this point.
	if step.MarkedCompleted() {
		t.Fatalf("expected step to not be marked completed yet")
	}

	// Once all tasks are completed, the step should be marked completed.
	t1.MarkCompleted()
	step.ReflectTasks()
	if !step.MarkedCompleted() {
		t.Fatalf("expected step to be marked completed")
	}
}

func TestMigrationStatus_SnapshotsAndVMs(t *testing.T) {
	var ms MigrationStatus
	if got := ms.ActiveSnapshot(); got == nil {
		t.Fatalf("expected non-nil snapshot")
	}

	uid := types.UID("m1")
	ms.NewSnapshot(Snapshot{Migration: SnapshotRef{UID: uid}})
	ms.NewSnapshot(Snapshot{Migration: SnapshotRef{UID: types.UID("m2")}})

	active := ms.ActiveSnapshot()
	if active == nil || active.Migration.UID != types.UID("m2") {
		t.Fatalf("unexpected active snapshot: %#v", active)
	}

	found, snap := ms.SnapshotWithMigration(uid)
	if !found || snap == nil || snap.Migration.UID != uid {
		t.Fatalf("unexpected SnapshotWithMigration: found=%v snap=%#v", found, snap)
	}

	ms.VMs = []*VMStatus{{VM: VM{Ref: refapi.Ref{ID: "vm-1"}}}}
	vm, found := ms.FindVM(refapi.Ref{ID: "vm-1"})
	if !found || vm == nil || vm.ID != "vm-1" {
		t.Fatalf("unexpected FindVM: found=%v vm=%#v", found, vm)
	}
}

func TestVMStatus_PipelineHelpers(t *testing.T) {
	s1 := &Step{Task: Task{Name: "s1"}}
	s1.MarkStarted()
	s2 := &Step{Task: Task{Name: "s2"}}
	s2.MarkCompleted()
	s2.AddError("e1")

	vm := &VMStatus{
		VM:       VM{Ref: refapi.Ref{ID: "vm-1"}},
		Phase:    "Running",
		Pipeline: []*Step{s1, s2},
	}

	step, found := vm.FindStep("s2")
	if !found || step == nil || step.Name != "s2" {
		t.Fatalf("unexpected FindStep: found=%v step=%#v", found, step)
	}

	vm.ReflectPipeline()
	if !vm.MarkedStarted() {
		t.Fatalf("expected VM to be marked started")
	}
	// Not completed because only 1/2 steps completed.
	if vm.MarkedCompleted() {
		t.Fatalf("expected VM to not be marked completed")
	}
	if vm.Error == nil || len(vm.Error.Reasons) != 1 || vm.Error.Reasons[0] != "e1" {
		t.Fatalf("expected error to be reflected, got %#v", vm.Error)
	}

	// Complete all steps, then reflect again => completed.
	s1.MarkCompleted()
	vm.ReflectPipeline()
	if !vm.MarkedCompleted() {
		t.Fatalf("expected VM to be marked completed")
	}
}

func TestPrecopy_Deltas(t *testing.T) {
	p := &Precopy{}
	p.WithDeltas(map[string]string{"disk1": "d1", "disk2": "d2"})
	m := p.DeltaMap()
	if len(m) != 2 || m["disk1"] != "d1" || m["disk2"] != "d2" {
		t.Fatalf("unexpected delta map: %#v", m)
	}
}

func TestGeneratedDeepCopy_PlanPackage(t *testing.T) {
	now := metav1.Now()

	errObj := &Error{Phase: "P", Reasons: []string{"a", "b"}}
	errCopy := errObj.DeepCopy()
	if errCopy == nil || errCopy == errObj || len(errCopy.Reasons) != 2 {
		t.Fatalf("unexpected Error deepcopy: %#v", errCopy)
	}
	errCopy.Reasons[0] = "changed"
	if errObj.Reasons[0] != "a" {
		t.Fatalf("expected reasons slice deep-copied")
	}

	hr := &HookRef{Step: "S", Hook: corev1.ObjectReference{Name: "h", Namespace: "ns"}}
	if hr.DeepCopy() == nil {
		t.Fatalf("expected HookRef deepcopy")
	}

	task1 := &Task{
		Timed:       Timed{Started: &now},
		Name:        "t1",
		Annotations: map[string]string{"k": "v"},
		Progress:    libitr.Progress{Completed: 1},
		Error:       errObj,
	}
	task2 := &Task{Name: "t2"}
	step := &Step{
		Task:  Task{Name: "s1"},
		Tasks: []*Task{task1, task2, nil},
	}
	stepCopy := step.DeepCopy()
	if stepCopy == nil || stepCopy == step || len(stepCopy.Tasks) != 3 || stepCopy.Tasks[0] == task1 {
		t.Fatalf("unexpected Step deepcopy: %#v", stepCopy)
	}
	stepCopy.Tasks[0].Annotations["k"] = "changed"
	if task1.Annotations["k"] != "v" {
		t.Fatalf("expected annotations map deep-copied")
	}

	warm := &Warm{
		Successes:     1,
		NextPrecopyAt: &now,
		Precopies: []Precopy{
			{
				Start:  &now,
				End:    &now,
				Deltas: []DiskDelta{{Disk: "d1", DeltaID: "x"}},
			},
		},
	}

	vmStatus := &VMStatus{
		VM: VM{
			Ref:        refapi.Ref{ID: "vm-1", Name: "n1"},
			TargetName: "t",
		},
		Phase: "Running",
		Warm:  warm,
		Pipeline: []*Step{
			step,
		},
		Error: errObj,
	}
	vmCopy := vmStatus.DeepCopy()
	if vmCopy == nil || vmCopy == vmStatus || vmCopy.Warm == warm || vmCopy.Pipeline[0] == step {
		t.Fatalf("unexpected VMStatus deepcopy: %#v", vmCopy)
	}

	ms := &MigrationStatus{
		Timed: Timed{Started: &now},
		History: []Snapshot{
			{Migration: SnapshotRef{UID: types.UID("m1")}},
		},
		VMs: []*VMStatus{vmStatus, nil},
	}
	msCopy := ms.DeepCopy()
	if msCopy == nil || msCopy == ms || len(msCopy.History) != 1 || msCopy.VMs[0] == vmStatus {
		t.Fatalf("unexpected MigrationStatus deepcopy: %#v", msCopy)
	}
}

func TestGeneratedDeepCopy_Plan_DeepCopyFunctionsCovered(t *testing.T) {
	// This test explicitly calls the generated DeepCopy() methods that are not
	// exercised by the higher-level object graph tests above (which mostly call
	// DeepCopyInto() via nested structs).
	now := metav1.Now()

	dd := (&DiskDelta{Disk: "d1", DeltaID: "x"}).DeepCopy()
	if dd == nil || dd.Disk != "d1" || dd.DeltaID != "x" {
		t.Fatalf("unexpected DiskDelta deepcopy: %#v", dd)
	}

	mp := (&Map{}).DeepCopy()
	if mp == nil {
		t.Fatalf("expected Map deepcopy")
	}

	p := (&Precopy{
		Start:  &now,
		End:    &now,
		Deltas: []DiskDelta{{Disk: "d1", DeltaID: "x"}},
	}).DeepCopy()
	if p == nil || p.Start == nil || p.End == nil || len(p.Deltas) != 1 {
		t.Fatalf("unexpected Precopy deepcopy: %#v", p)
	}

	snap := (&Snapshot{
		Map:       SnapshotMap{Network: SnapshotRef{Namespace: "ns", Name: "net"}, Storage: SnapshotRef{Namespace: "ns", Name: "st"}},
		Migration: SnapshotRef{UID: types.UID("m1")},
	}).DeepCopy()
	if snap == nil {
		t.Fatalf("expected Snapshot deepcopy")
	}

	sm := (&SnapshotMap{}).DeepCopy()
	if sm == nil {
		t.Fatalf("expected SnapshotMap deepcopy")
	}

	sr := (&SnapshotRef{Namespace: "ns", Name: "n"}).DeepCopy()
	if sr == nil || sr.Namespace != "ns" || sr.Name != "n" {
		t.Fatalf("unexpected SnapshotRef deepcopy: %#v", sr)
	}

	srp := (&SnapshotRefPair{
		Source:      SnapshotRef{Namespace: "s", Name: "a"},
		Destination: SnapshotRef{Namespace: "d", Name: "b"},
	}).DeepCopy()
	if srp == nil || srp.Source.Namespace != "s" || srp.Destination.Namespace != "d" {
		t.Fatalf("unexpected SnapshotRefPair deepcopy: %#v", srp)
	}

	task := (&Task{Name: "t", Annotations: map[string]string{"k": "v"}}).DeepCopy()
	if task == nil || task.Name != "t" || task.Annotations["k"] != "v" {
		t.Fatalf("unexpected Task deepcopy: %#v", task)
	}
	task.Annotations["k"] = "changed"
	if task.Annotations["k"] != "changed" {
		t.Fatalf("expected task copy to be mutable")
	}

	td := (&Timed{Started: &now, Completed: &now}).DeepCopy()
	if td == nil || td.Started == nil || td.Completed == nil {
		t.Fatalf("unexpected Timed deepcopy: %#v", td)
	}

	vm := (&VM{
		Ref:        refapi.Ref{ID: "vm-1"},
		TargetName: "t",
		Hooks:      []HookRef{{Step: "S", Hook: corev1.ObjectReference{Name: "h"}}},
	}).DeepCopy()
	if vm == nil || vm.ID != "vm-1" || vm.TargetName != "t" || len(vm.Hooks) != 1 {
		t.Fatalf("unexpected VM deepcopy: %#v", vm)
	}

	w := (&Warm{NextPrecopyAt: &now, Precopies: []Precopy{{Start: &now}}}).DeepCopy()
	if w == nil || w.NextPrecopyAt == nil || len(w.Precopies) != 1 {
		t.Fatalf("unexpected Warm deepcopy: %#v", w)
	}
}
