package handler

import (
	"sync/atomic"
	"testing"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestWatchManager_EnsurePeriodicEvents_EndStopsAllProviders(t *testing.T) {
	m := &WatchManager{}
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: types.UID("u1")}}

	var ticks int32
	m.EnsurePeriodicEvents(p, struct{}{}, 5*time.Millisecond, func() {
		atomic.AddInt32(&ticks, 1)
	})

	// Wait for at least one tick.
	deadline := time.Now().Add(200 * time.Millisecond)
	for atomic.LoadInt32(&ticks) == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if atomic.LoadInt32(&ticks) == 0 {
		t.Fatalf("expected ticks")
	}

	// Stop and ensure ticks don't keep increasing significantly.
	m.End()
	before := atomic.LoadInt32(&ticks)
	time.Sleep(20 * time.Millisecond)
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

	time.Sleep(20 * time.Millisecond)
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

	time.Sleep(15 * time.Millisecond)
	m.Deleted(p1)

	// p2 should still tick.
	before2 := atomic.LoadInt32(&t2)
	time.Sleep(20 * time.Millisecond)
	after2 := atomic.LoadInt32(&t2)
	m.End()

	if after2 <= before2 {
		t.Fatalf("expected p2 to keep ticking after deleting p1")
	}
}
