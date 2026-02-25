package container

import (
	"errors"
	"testing"

	"github.com/kubev2v/forklift/pkg/lib/inventory/model"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type stubCollector struct {
	name string
	own  metav1.Object

	startErr error

	started   int
	shutdown  int
	reset     int
	hasParity bool
}

func (s *stubCollector) Name() string                                    { return s.name }
func (s *stubCollector) Owner() metav1.Object                            { return s.own }
func (s *stubCollector) Start() error                                    { s.started++; return s.startErr }
func (s *stubCollector) Shutdown()                                       { s.shutdown++ }
func (s *stubCollector) HasParity() bool                                 { return s.hasParity }
func (s *stubCollector) DB() model.DB                                    { return nil }
func (s *stubCollector) Test() (int, error)                              { return 0, nil }
func (s *stubCollector) Follow(interface{}, []string, interface{}) error { return nil }
func (s *stubCollector) Reset()                                          { s.reset++ }
func (s *stubCollector) Version() (string, string, string, string, error) {
	return "", "", "", "", nil
}

func podOwner(uid string) *core.Pod {
	return &core.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid)}}
}

func cmOwner(uid string) *core.ConfigMap {
	return &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid)}}
}

func TestNew_InitialListEmpty(t *testing.T) {
	c := New()
	if got := c.List(); len(got) != 0 {
		t.Fatalf("expected empty")
	}
}

func TestContainer_key_UsesKindAndUID_Pod(t *testing.T) {
	c := New()
	o := podOwner("u1")
	k := c.key(o)
	if k.Kind != "Pod" || k.UID != types.UID("u1") {
		t.Fatalf("unexpected key: %#v", k)
	}
}

func TestContainer_key_UsesKindAndUID_ConfigMap(t *testing.T) {
	c := New()
	o := cmOwner("u1")
	k := c.key(o)
	if k.Kind != "ConfigMap" || k.UID != types.UID("u1") {
		t.Fatalf("unexpected key: %#v", k)
	}
}

func TestContainer_Add_StartCalledOnce(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1")}
	if err := c.Add(col); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if col.started != 1 {
		t.Fatalf("expected start=1 got %d", col.started)
	}
}

func TestContainer_Add_SetsGetFoundTrue(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_ = c.Add(col)
	got, found := c.Get(o)
	if !found || got != col {
		t.Fatalf("expected found collector")
	}
}

func TestContainer_Get_NotFoundFalse(t *testing.T) {
	c := New()
	_, found := c.Get(podOwner("u1"))
	if found {
		t.Fatalf("expected not found")
	}
}

func TestContainer_List_ContainsAdded(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1")}
	_ = c.Add(col)
	list := c.List()
	if len(list) != 1 || list[0] != col {
		t.Fatalf("unexpected list: %#v", list)
	}
}

func TestContainer_List_TwoDifferentKindsSameUID_AreDistinct(t *testing.T) {
	c := New()
	colA := &stubCollector{name: "a", own: podOwner("u1")}
	colB := &stubCollector{name: "b", own: cmOwner("u1")}
	if err := c.Add(colA); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := c.Add(colB); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(c.List()) != 2 {
		t.Fatalf("expected 2")
	}
}

func TestContainer_Add_DuplicateSameKindUID_Err(t *testing.T) {
	c := New()
	o1 := podOwner("u1")
	o2 := podOwner("u1") // same kind+uid => duplicate key
	col1 := &stubCollector{name: "a", own: o1}
	col2 := &stubCollector{name: "b", own: o2}
	if err := c.Add(col1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := c.Add(col2); err == nil {
		t.Fatalf("expected err")
	}
}

func TestContainer_Add_Duplicate_DoesNotStartSecond(t *testing.T) {
	c := New()
	o1 := podOwner("u1")
	o2 := podOwner("u1")
	col1 := &stubCollector{name: "a", own: o1}
	col2 := &stubCollector{name: "b", own: o2}
	_ = c.Add(col1)
	_ = c.Add(col2)
	if col2.started != 0 {
		t.Fatalf("expected start=0 got %d", col2.started)
	}
}

func TestContainer_Add_StartError_ReturnsWrappedError(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1"), startErr: errors.New("boom")}
	if err := c.Add(col); err == nil {
		t.Fatalf("expected err")
	}
}

func TestContainer_Add_StartError_CollectorStillInMap(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o, startErr: errors.New("boom")}
	_ = c.Add(col)
	_, found := c.Get(o)
	if !found {
		t.Fatalf("expected found (added before Start)")
	}
}

func TestContainer_Replace_WhenMissing_StartCalled(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1")}
	_, _, err := c.Replace(col)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if col.started != 1 {
		t.Fatalf("expected started")
	}
}

func TestContainer_Replace_WhenMissing_GetReturnsNew(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_, _, _ = c.Replace(col)
	got, found := c.Get(o)
	if !found || got != col {
		t.Fatalf("expected replaced collector")
	}
}

func TestContainer_Replace_WhenMissing_DoesNotShutdownAny(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1")}
	_, _, _ = c.Replace(col)
	if col.shutdown != 0 {
		t.Fatalf("expected shutdown=0")
	}
}

func TestContainer_Replace_WhenExisting_ShutsDownOld(t *testing.T) {
	c := New()
	o := podOwner("u1")
	old := &stubCollector{name: "old", own: o}
	newC := &stubCollector{name: "new", own: o}
	_ = c.Add(old)
	_, _, _ = c.Replace(newC)
	if old.shutdown != 1 {
		t.Fatalf("expected old shutdown=1 got %d", old.shutdown)
	}
}

func TestContainer_Replace_WhenExisting_StartsNew(t *testing.T) {
	c := New()
	o := podOwner("u1")
	old := &stubCollector{name: "old", own: o}
	newC := &stubCollector{name: "new", own: o}
	_ = c.Add(old)
	_, _, _ = c.Replace(newC)
	if newC.started != 1 {
		t.Fatalf("expected new started")
	}
}

func TestContainer_Replace_WhenExisting_GetReturnsNew(t *testing.T) {
	c := New()
	o := podOwner("u1")
	old := &stubCollector{name: "old", own: o}
	newC := &stubCollector{name: "new", own: o}
	_ = c.Add(old)
	_, _, _ = c.Replace(newC)
	got, found := c.Get(o)
	if !found || got != newC {
		t.Fatalf("expected new collector")
	}
}

func TestContainer_Replace_StartError_ReturnsError(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1"), startErr: errors.New("boom")}
	_, _, err := c.Replace(col)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestContainer_Replace_StartError_StillReplaces(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o, startErr: errors.New("boom")}
	_, _, _ = c.Replace(col)
	got, found := c.Get(o)
	if !found || got != col {
		t.Fatalf("expected replaced even on start error")
	}
}

func TestContainer_Replace_ReturnValues_CurrentBehaviorNilFalse(t *testing.T) {
	// Replace() currently does not assign named return p/found due to shadowing.
	c := New()
	o := podOwner("u1")
	old := &stubCollector{name: "old", own: o}
	newC := &stubCollector{name: "new", own: o}
	_ = c.Add(old)
	p, found, err := c.Replace(newC)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p != nil || found {
		t.Fatalf("expected (nil,false) due to current implementation, got (%v,%v)", p, found)
	}
}

func TestContainer_Delete_NotFoundFalse(t *testing.T) {
	c := New()
	_, found := c.Delete(podOwner("u1"))
	if found {
		t.Fatalf("expected false")
	}
}

func TestContainer_Delete_FoundTrueAndReturnsCollector(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_ = c.Add(col)
	got, found := c.Delete(o)
	if !found || got != col {
		t.Fatalf("expected deleted collector")
	}
}

func TestContainer_Delete_CallsShutdown(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_ = c.Add(col)
	_, _ = c.Delete(o)
	if col.shutdown != 1 {
		t.Fatalf("expected shutdown=1 got %d", col.shutdown)
	}
}

func TestContainer_Delete_RemovesFromGet(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_ = c.Add(col)
	_, _ = c.Delete(o)
	_, found := c.Get(o)
	if found {
		t.Fatalf("expected removed")
	}
}

func TestContainer_AddThenDeleteThenAddAgain_Works(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_ = c.Add(col)
	_, _ = c.Delete(o)
	col2 := &stubCollector{name: "b", own: o}
	if err := c.Add(col2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestContainer_List_OrderIndependent_LengthMatches(t *testing.T) {
	c := New()
	_ = c.Add(&stubCollector{name: "a", own: podOwner("u1")})
	_ = c.Add(&stubCollector{name: "b", own: podOwner("u2")})
	if len(c.List()) != 2 {
		t.Fatalf("expected 2")
	}
}

func TestContainer_Add_AllowsSameKindDifferentUID(t *testing.T) {
	c := New()
	if err := c.Add(&stubCollector{name: "a", own: podOwner("u1")}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if err := c.Add(&stubCollector{name: "b", own: podOwner("u2")}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestContainer_Add_Duplicate_DoesNotOverwriteExisting(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col1 := &stubCollector{name: "a", own: o}
	col2 := &stubCollector{name: "b", own: podOwner("u1")}
	_ = c.Add(col1)
	_ = c.Add(col2)
	got, found := c.Get(o)
	if !found || got != col1 {
		t.Fatalf("expected original kept")
	}
}

func TestContainer_Add_Duplicate_MapSizeStaysOne(t *testing.T) {
	c := New()
	_ = c.Add(&stubCollector{name: "a", own: podOwner("u1")})
	_ = c.Add(&stubCollector{name: "b", own: podOwner("u1")})
	if len(c.List()) != 1 {
		t.Fatalf("expected size 1")
	}
}

func TestContainer_Delete_NotFound_DoesNotPanic(t *testing.T) {
	c := New()
	_, _ = c.Delete(podOwner("u1"))
}

func TestContainer_Delete_Twice_SecondNotFound(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_ = c.Add(col)
	_, found1 := c.Delete(o)
	_, found2 := c.Delete(o)
	if !found1 || found2 {
		t.Fatalf("expected found then not found")
	}
}

func TestContainer_Delete_ShutdownCalledOnceEvenIfDeletedTwice(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_ = c.Add(col)
	_, _ = c.Delete(o)
	_, _ = c.Delete(o)
	if col.shutdown != 1 {
		t.Fatalf("expected shutdown=1 got %d", col.shutdown)
	}
}

func TestContainer_Replace_Twice_ShutsDownPreviousEachTime(t *testing.T) {
	c := New()
	o := podOwner("u1")
	a := &stubCollector{name: "a", own: o}
	b := &stubCollector{name: "b", own: o}
	d := &stubCollector{name: "d", own: o}
	_, _, _ = c.Replace(a)
	_, _, _ = c.Replace(b)
	_, _, _ = c.Replace(d)
	if a.shutdown != 1 || b.shutdown != 1 || d.shutdown != 0 {
		t.Fatalf("unexpected shutdown counts: a=%d b=%d d=%d", a.shutdown, b.shutdown, d.shutdown)
	}
}

func TestContainer_Replace_SameCollector_ShutsDownAndStartsAgain(t *testing.T) {
	c := New()
	o := podOwner("u1")
	a := &stubCollector{name: "a", own: o}
	_, _, _ = c.Replace(a)
	_, _, _ = c.Replace(a)
	if a.shutdown != 1 {
		t.Fatalf("expected shutdown=1 got %d", a.shutdown)
	}
	if a.started != 2 {
		t.Fatalf("expected started=2 got %d", a.started)
	}
}

func TestContainer_Replace_DoesNotReturnOldCollector_CurrentBehavior(t *testing.T) {
	c := New()
	o := podOwner("u1")
	old := &stubCollector{name: "old", own: o}
	_ = c.Add(old)
	p, found, _ := c.Replace(&stubCollector{name: "new", own: o})
	if p != nil || found {
		t.Fatalf("expected (nil,false)")
	}
}

func TestContainer_Get_IgnoresNamespaceNameOnlyUsesUIDKind(t *testing.T) {
	c := New()
	p1 := &core.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID("u1"), Namespace: "a", Name: "x"}}
	p2 := &core.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID("u1"), Namespace: "b", Name: "y"}}
	col := &stubCollector{name: "a", own: p1}
	_ = c.Add(col)
	got, found := c.Get(p2)
	if !found || got != col {
		t.Fatalf("expected found by same uid+kind")
	}
}

func TestContainer_Add_DuplicateEvenIfDifferentNamespaceName(t *testing.T) {
	c := New()
	p1 := &core.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID("u1"), Namespace: "a", Name: "x"}}
	p2 := &core.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID("u1"), Namespace: "b", Name: "y"}}
	col1 := &stubCollector{name: "a", own: p1}
	col2 := &stubCollector{name: "b", own: p2}
	_ = c.Add(col1)
	if err := c.Add(col2); err == nil {
		t.Fatalf("expected duplicate err")
	}
}

func TestContainer_Add_StartError_DoesNotCallShutdown(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1"), startErr: errors.New("boom")}
	_ = c.Add(col)
	if col.shutdown != 0 {
		t.Fatalf("expected shutdown=0")
	}
}

func TestContainer_Add_StartError_StartCalledOnce(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1"), startErr: errors.New("boom")}
	_ = c.Add(col)
	if col.started != 1 {
		t.Fatalf("expected started=1 got %d", col.started)
	}
}

func TestContainer_Replace_StartError_StartCalledOnce(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1"), startErr: errors.New("boom")}
	_, _, _ = c.Replace(col)
	if col.started != 1 {
		t.Fatalf("expected started=1 got %d", col.started)
	}
}

func TestContainer_Replace_StartError_DoesNotShutdownNew(t *testing.T) {
	c := New()
	col := &stubCollector{name: "a", own: podOwner("u1"), startErr: errors.New("boom")}
	_, _, _ = c.Replace(col)
	if col.shutdown != 0 {
		t.Fatalf("expected shutdown=0")
	}
}

func TestContainer_Replace_StartError_StillShutsDownOldIfPresent(t *testing.T) {
	c := New()
	o := podOwner("u1")
	old := &stubCollector{name: "old", own: o}
	_ = c.Add(old)
	bad := &stubCollector{name: "bad", own: o, startErr: errors.New("boom")}
	_, _, _ = c.Replace(bad)
	if old.shutdown != 1 {
		t.Fatalf("expected old shutdown")
	}
}

func TestContainer_Get_AfterReplace_ReturnsReplaced(t *testing.T) {
	c := New()
	o := podOwner("u1")
	a := &stubCollector{name: "a", own: o}
	b := &stubCollector{name: "b", own: o}
	_, _, _ = c.Replace(a)
	_, _, _ = c.Replace(b)
	got, found := c.Get(o)
	if !found || got != b {
		t.Fatalf("expected b")
	}
}

func TestContainer_List_AfterReplace_SizeOneForSameKey(t *testing.T) {
	c := New()
	o := podOwner("u1")
	_, _, _ = c.Replace(&stubCollector{name: "a", own: o})
	_, _, _ = c.Replace(&stubCollector{name: "b", own: o})
	if len(c.List()) != 1 {
		t.Fatalf("expected 1")
	}
}

func TestContainer_List_AfterDelete_SizeDecrements(t *testing.T) {
	c := New()
	_ = c.Add(&stubCollector{name: "a", own: podOwner("u1")})
	_ = c.Add(&stubCollector{name: "b", own: podOwner("u2")})
	_, _ = c.Delete(podOwner("u1"))
	if len(c.List()) != 1 {
		t.Fatalf("expected 1")
	}
}

func TestContainer_Delete_ReturnedCollectorIsSamePointer(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_ = c.Add(col)
	got, found := c.Delete(o)
	if !found || got != col {
		t.Fatalf("expected same pointer")
	}
}

func TestContainer_Delete_DoesNotShutdownUnrelatedCollector(t *testing.T) {
	c := New()
	o1 := podOwner("u1")
	o2 := podOwner("u2")
	a := &stubCollector{name: "a", own: o1}
	b := &stubCollector{name: "b", own: o2}
	_ = c.Add(a)
	_ = c.Add(b)
	_, _ = c.Delete(o1)
	if b.shutdown != 0 {
		t.Fatalf("expected b.shutdown=0")
	}
}

func TestContainer_Add_ListThenGet_AllConsistent(t *testing.T) {
	c := New()
	o := podOwner("u1")
	col := &stubCollector{name: "a", own: o}
	_ = c.Add(col)
	if len(c.List()) != 1 {
		t.Fatalf("expected list size 1")
	}
	got, found := c.Get(o)
	if !found || got != col {
		t.Fatalf("expected get matches")
	}
}
