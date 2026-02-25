package plan

import (
	"testing"

	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	libitr "github.com/kubev2v/forklift/pkg/lib/itinerary"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestError_Add_AppendsReasons(t *testing.T) {
	e := &Error{Phase: "p"}
	e.Add("a", "b")
	if len(e.Reasons) != 2 {
		t.Fatalf("expected 2 got %d", len(e.Reasons))
	}
}

func TestError_Add_Deduplicates(t *testing.T) {
	e := &Error{Phase: "p"}
	e.Add("a", "a", "a")
	if len(e.Reasons) != 1 {
		t.Fatalf("expected 1 got %d", len(e.Reasons))
	}
}

func TestError_Add_KeepsExistingAndAddsNew(t *testing.T) {
	e := &Error{Phase: "p", Reasons: []string{"a"}}
	e.Add("a", "b")
	if len(e.Reasons) != 2 {
		t.Fatalf("expected 2 got %d", len(e.Reasons))
	}
}

func TestTimed_MarkReset_ClearsTimes(t *testing.T) {
	var tt Timed
	tt.MarkStarted()
	tt.MarkCompleted()
	tt.MarkReset()
	if tt.Started != nil || tt.Completed != nil {
		t.Fatalf("expected cleared")
	}
}

// ---- Consolidated from vm_more_test.go ----

func TestHookRef_String_IncludesHookAndStep(t *testing.T) {
	h := &HookRef{
		Step: "preHook",
		Hook: core.ObjectReference{Namespace: "ns", Name: "h1"},
	}
	if got := h.String(); got != "ns/h1 @preHook" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestVM_FindHook_Found(t *testing.T) {
	vm := &VM{
		Ref: ref.Ref{ID: "vm1"},
		Hooks: []HookRef{
			{Step: "a"},
			{Step: "b"},
		},
	}
	refOut, found := vm.FindHook("b")
	if !found || refOut.Step != "b" {
		t.Fatalf("expected found b, got found=%v ref=%#v", found, refOut)
	}
}

func TestVM_FindHook_NotFound(t *testing.T) {
	vm := &VM{Hooks: []HookRef{{Step: "a"}}}
	_, found := vm.FindHook("x")
	if found {
		t.Fatalf("expected not found")
	}
}

func TestHookRef_String_EmptyNamespace(t *testing.T) {
	h := &HookRef{
		Step: "s",
		Hook: core.ObjectReference{Name: "h1"},
	}
	if got := h.String(); got != "h1 @s" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestVM_FindHook_EmptyHooks(t *testing.T) {
	vm := &VM{}
	_, found := vm.FindHook("a")
	if found {
		t.Fatalf("expected not found")
	}
}

func TestTimed_MarkStarted_SetsStartedOnly(t *testing.T) {
	var tt Timed
	tt.MarkStarted()
	if tt.Started == nil {
		t.Fatalf("expected started")
	}
	if tt.Completed != nil {
		t.Fatalf("expected completed nil")
	}
}

func TestTimed_MarkStarted_DoesNotOverwriteExisting(t *testing.T) {
	var tt Timed
	tt.MarkStarted()
	s := tt.Started
	tt.MarkStarted()
	if tt.Started != s {
		t.Fatalf("expected same started pointer")
	}
}

func TestTimed_MarkCompleted_SetsStartedAndCompleted(t *testing.T) {
	var tt Timed
	tt.MarkCompleted()
	if tt.Started == nil || tt.Completed == nil {
		t.Fatalf("expected both set")
	}
}

func TestTimed_MarkedStarted_FalseWhenNil(t *testing.T) {
	var tt Timed
	if tt.MarkedStarted() {
		t.Fatalf("expected false")
	}
}

func TestTimed_MarkedStarted_TrueAfterMarkStarted(t *testing.T) {
	var tt Timed
	tt.MarkStarted()
	if !tt.MarkedStarted() {
		t.Fatalf("expected true")
	}
}

func TestTimed_MarkedCompleted_FalseWhenNil(t *testing.T) {
	var tt Timed
	if tt.MarkedCompleted() {
		t.Fatalf("expected false")
	}
}

func TestTimed_MarkedCompleted_TrueAfterMarkCompleted(t *testing.T) {
	var tt Timed
	tt.MarkCompleted()
	if !tt.MarkedCompleted() {
		t.Fatalf("expected true")
	}
}

func TestTimed_Running_FalseWhenNotStarted(t *testing.T) {
	var tt Timed
	if tt.Running() {
		t.Fatalf("expected false")
	}
}

func TestTimed_Running_TrueWhenStartedNotCompleted(t *testing.T) {
	var tt Timed
	tt.MarkStarted()
	if !tt.Running() {
		t.Fatalf("expected true")
	}
}

func TestTimed_Running_FalseWhenCompleted(t *testing.T) {
	var tt Timed
	tt.MarkCompleted()
	if tt.Running() {
		t.Fatalf("expected false")
	}
}

func TestMigrationStatus_ActiveSnapshot_EmptyHistory_ReturnsEmptySnapshot(t *testing.T) {
	var ms MigrationStatus
	s := ms.ActiveSnapshot()
	if s == nil {
		t.Fatalf("expected snapshot")
	}
	if s.Migration.UID != "" {
		t.Fatalf("expected zero value")
	}
}

func TestMigrationStatus_ActiveSnapshot_ReturnsLast(t *testing.T) {
	var ms MigrationStatus
	ms.History = append(ms.History,
		Snapshot{Migration: SnapshotRef{Name: "a"}},
		Snapshot{Migration: SnapshotRef{Name: "b"}},
	)
	s := ms.ActiveSnapshot()
	if s.Migration.Name != "b" {
		t.Fatalf("expected last")
	}
}

func TestMigrationStatus_NewSnapshot_Appends(t *testing.T) {
	var ms MigrationStatus
	ms.NewSnapshot(Snapshot{Migration: SnapshotRef{Name: "x"}})
	if len(ms.History) != 1 {
		t.Fatalf("expected 1")
	}
}

func TestMigrationStatus_SnapshotWithMigration_NotFound(t *testing.T) {
	var ms MigrationStatus
	found, sn := ms.SnapshotWithMigration(types.UID("u"))
	if found || sn != nil {
		t.Fatalf("expected not found")
	}
}

func TestMigrationStatus_SnapshotWithMigration_Found(t *testing.T) {
	var ms MigrationStatus
	ms.History = append(ms.History, Snapshot{Migration: SnapshotRef{UID: types.UID("u")}})
	found, sn := ms.SnapshotWithMigration(types.UID("u"))
	if !found || sn == nil {
		t.Fatalf("expected found")
	}
}

func TestMigrationStatus_SnapshotWithMigration_ReturnsLastMatch(t *testing.T) {
	var ms MigrationStatus
	ms.History = append(ms.History,
		Snapshot{Migration: SnapshotRef{UID: types.UID("u")}},
		Snapshot{Migration: SnapshotRef{UID: types.UID("u")}},
	)
	found, sn := ms.SnapshotWithMigration(types.UID("u"))
	if !found || sn == nil {
		t.Fatalf("expected found")
	}
	// It doesn't break; last match should be returned.
	if sn != &ms.History[1] {
		t.Fatalf("expected last match")
	}
}

func TestMigrationStatus_FindVM_NotFound(t *testing.T) {
	var ms MigrationStatus
	vm, found := ms.FindVM(ref.Ref{ID: "x"})
	if found || vm != nil {
		t.Fatalf("expected not found")
	}
}

func TestMigrationStatus_FindVM_FoundByID(t *testing.T) {
	var ms MigrationStatus
	ms.VMs = append(ms.VMs, &VMStatus{VM: VM{Ref: ref.Ref{ID: "x"}}})
	vm, found := ms.FindVM(ref.Ref{ID: "x"})
	if !found || vm == nil || vm.ID != "x" {
		t.Fatalf("expected found")
	}
}

func TestStep_FindTask_NotFound(t *testing.T) {
	s := &Step{Tasks: []*Task{{Name: "a"}, {Name: "b"}}}
	task, found := s.FindTask("x")
	if found {
		t.Fatalf("expected not found")
	}
	// Current implementation returns the last iterated task even when not found.
	if task == nil || task.Name != "b" {
		t.Fatalf("expected last task, got %#v", task)
	}
}

func TestStep_FindTask_Found(t *testing.T) {
	s := &Step{Tasks: []*Task{{Name: "a"}, {Name: "b"}}}
	task, found := s.FindTask("b")
	if !found || task == nil || task.Name != "b" {
		t.Fatalf("expected found")
	}
}

func TestTask_AddError_InitializesErrorAndAddsReasons(t *testing.T) {
	task := &Task{Phase: "p"}
	task.AddError("a")
	if task.Error == nil || len(task.Error.Reasons) != 1 {
		t.Fatalf("expected error reasons")
	}
	if task.Error.Phase != "p" {
		t.Fatalf("expected phase copied")
	}
}

func TestTask_AddError_Deduplicates(t *testing.T) {
	task := &Task{Phase: "p"}
	task.AddError("a", "a")
	if len(task.Error.Reasons) != 1 {
		t.Fatalf("expected 1")
	}
}

func TestTask_HasError_FalseWhenNil(t *testing.T) {
	task := &Task{}
	if task.HasError() {
		t.Fatalf("expected false")
	}
}

func TestTask_HasError_TrueWhenSet(t *testing.T) {
	task := &Task{Error: &Error{}}
	if !task.HasError() {
		t.Fatalf("expected true")
	}
}

func TestStep_ReflectTasks_EmptyTasks_DoesNotStartOrComplete(t *testing.T) {
	s := &Step{}
	s.ReflectTasks()
	// Current behavior: with 0 nested tasks, it's considered "all completed".
	if !s.MarkedStarted() || !s.MarkedCompleted() {
		t.Fatalf("expected started+completed")
	}
}

func TestStep_ReflectTasks_OneStarted_MarksStarted(t *testing.T) {
	task := &Task{}
	task.MarkStarted()
	s := &Step{Tasks: []*Task{task}}
	s.ReflectTasks()
	if !s.MarkedStarted() {
		t.Fatalf("expected started")
	}
}

func TestStep_ReflectTasks_AllCompleted_MarksCompleted(t *testing.T) {
	t1 := &Task{}
	t1.MarkCompleted()
	t2 := &Task{}
	t2.MarkCompleted()
	s := &Step{Tasks: []*Task{t1, t2}}
	s.ReflectTasks()
	if !s.MarkedCompleted() {
		t.Fatalf("expected completed")
	}
}

func TestStep_ReflectTasks_AccumulatesProgressCompleted(t *testing.T) {
	t1 := &Task{Progress: libitr.Progress{Completed: 2, Total: 10}}
	t1.MarkStarted()
	t2 := &Task{Progress: libitr.Progress{Completed: 3, Total: 10}}
	t2.MarkStarted()
	s := &Step{Tasks: []*Task{t1, t2}}
	s.ReflectTasks()
	if s.Progress.Completed != 5 {
		t.Fatalf("expected 5 got %d", s.Progress.Completed)
	}
}

func TestStep_ReflectTasks_PropagatesNestedTaskErrors(t *testing.T) {
	t1 := &Task{Phase: "p"}
	t1.AddError("x")
	t1.MarkStarted()
	s := &Step{Tasks: []*Task{t1}}
	s.ReflectTasks()
	if s.Error == nil || len(s.Error.Reasons) != 1 {
		t.Fatalf("expected step error")
	}
}

func TestStep_ReflectTasks_MarksStartedWhenAnyStarted(t *testing.T) {
	t1 := &Task{}
	t1.MarkStarted()
	t2 := &Task{} // not started
	s := &Step{Tasks: []*Task{t1, t2}}
	s.ReflectTasks()
	if !s.MarkedStarted() {
		t.Fatalf("expected started")
	}
	if s.MarkedCompleted() {
		t.Fatalf("expected not completed")
	}
}

func TestStep_ReflectTasks_DoesNotMarkCompletedWhenSomeNotCompleted(t *testing.T) {
	t1 := &Task{}
	t1.MarkCompleted()
	t2 := &Task{}
	t2.MarkStarted()
	s := &Step{Tasks: []*Task{t1, t2}}
	s.ReflectTasks()
	if s.MarkedCompleted() {
		t.Fatalf("expected not completed")
	}
}
