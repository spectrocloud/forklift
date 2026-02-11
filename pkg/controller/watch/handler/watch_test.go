package handler

import (
	"sync/atomic"
	"testing"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

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
	time.Sleep(time.Millisecond * 20)
	after := ticks.Load()
	if after-before <= 0 {
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
