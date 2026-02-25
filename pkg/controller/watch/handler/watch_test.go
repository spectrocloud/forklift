package handler

import (
	"sync/atomic"
	"testing"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// waitForCond polls until cond() returns true or the deadline is reached.
func waitForCond(deadline time.Time, cond func() bool) bool {
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

type stubStop struct {
	ended atomic.Bool
}

func (s *stubStop) End() {
	s.ended.Store(true)
}

func TestWatchManager_EnsurePeriodicEvents_TicksAndStops(t *testing.T) {
	m := &WatchManager{}
	p := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns",
			UID:       types.UID("uid1"),
		},
	}

	var ticks atomic.Int64
	m.EnsurePeriodicEvents(p, &struct{}{}, time.Millisecond*5, func() {
		ticks.Add(1)
	})

	// Wait for at least one tick.
	deadline := time.Now().Add(time.Second)
	for ticks.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond * 5)
	}
	if ticks.Load() == 0 {
		t.Fatalf("expected at least one tick")
	}

	// Calling EnsurePeriodicEvents again for the same kind should not create a second generator.
	before := ticks.Load()
	m.EnsurePeriodicEvents(p, &struct{}{}, time.Millisecond*5, func() {
		ticks.Add(1000)
	})
	if !waitForCond(time.Now().Add(time.Second), func() bool { return ticks.Load() > before }) {
		t.Fatalf("expected ticks to keep increasing")
	}

	// Deleted should stop all stoppables for the provider.
	m.Deleted(p)
	m.mutex.Lock()
	_, found := m.providerMap[p.UID]
	m.mutex.Unlock()
	if found {
		t.Fatalf("expected provider entry removed after Deleted()")
	}
}

func TestWatchManager_DeletedAndEnd_StopStoppables(t *testing.T) {
	m := &WatchManager{}
	p := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns",
			UID:       types.UID("uid1"),
		},
	}

	// Seed internal map with a custom stoppable.
	m.mutex.Lock()
	stoppables := m.ensureStoppablesUnlocked(p)
	ss := &stubStop{}
	(*stoppables)["KindX"] = ss
	m.mutex.Unlock()

	m.Deleted(p)
	if !ss.ended.Load() {
		t.Fatalf("expected stoppable.End() called on Deleted()")
	}

	// Ensure End() stops all remaining provider watches without panic.
	m.End()
}

func TestWatchManager_EnsurePeriodicEvents_EndStopsAllProviders(t *testing.T) {
	m := &WatchManager{}
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: types.UID("u1")}}

	var ticks int32
	m.EnsurePeriodicEvents(p, struct{}{}, 5*time.Millisecond, func() {
		atomic.AddInt32(&ticks, 1)
	})

	// Wait for at least one tick.
	if !waitForCond(time.Now().Add(time.Second), func() bool { return atomic.LoadInt32(&ticks) > 0 }) {
		t.Fatalf("expected at least one tick")
	}

	// Stop and ensure ticks don't keep increasing significantly.
	m.End()
	before := atomic.LoadInt32(&ticks)
	time.Sleep(30 * time.Millisecond) // allow any in-flight tick to land
	after := atomic.LoadInt32(&ticks)
	if after > before+1 {
		t.Fatalf("expected ticker stopped, ticks increased too much: before=%d after=%d", before, after)
	}
}

func TestWatchManager_EnsurePeriodicEvents_DeduplicatesByKind(t *testing.T) {
	m := &WatchManager{}
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: types.UID("u2")}}

	var ticks int32
	m.EnsurePeriodicEvents(p, &api.Provider{}, 5*time.Millisecond, func() {
		atomic.AddInt32(&ticks, 1)
	})
	// Call again with same kind; should not create a second generator.
	m.EnsurePeriodicEvents(p, &api.Provider{}, 5*time.Millisecond, func() {
		atomic.AddInt32(&ticks, 1000)
	})

	// Wait for several ticks to accumulate.
	if !waitForCond(time.Now().Add(time.Second), func() bool { return atomic.LoadInt32(&ticks) >= 3 }) {
		t.Fatalf("expected ticks to accumulate")
	}
	m.End()
	if atomic.LoadInt32(&ticks) >= 1000 {
		t.Fatalf("expected dedupe: second tickFunc should not run")
	}
}

func TestWatchManager_Deleted_StopsProviderOnly(t *testing.T) {
	m := &WatchManager{}
	p1 := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p1", UID: types.UID("u3")}}
	p2 := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p2", UID: types.UID("u4")}}

	var t1, t2 int32
	m.EnsurePeriodicEvents(p1, &api.Provider{}, 5*time.Millisecond, func() { atomic.AddInt32(&t1, 1) })
	m.EnsurePeriodicEvents(p2, &api.Provider{}, 5*time.Millisecond, func() { atomic.AddInt32(&t2, 1) })

	// Wait for both tickers to fire at least once.
	if !waitForCond(time.Now().Add(time.Second), func() bool {
		return atomic.LoadInt32(&t1) > 0 && atomic.LoadInt32(&t2) > 0
	}) {
		t.Fatalf("expected both tickers to fire")
	}

	m.Deleted(p1)

	// p2 should still tick after p1 is deleted.
	before2 := atomic.LoadInt32(&t2)
	if !waitForCond(time.Now().Add(time.Second), func() bool { return atomic.LoadInt32(&t2) > before2 }) {
		t.Fatalf("expected p2 to keep ticking after deleting p1")
	}
	m.End()
}
