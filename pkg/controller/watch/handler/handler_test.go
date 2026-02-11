package handler

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestHandler_Enqueue_RecoversFromSendOnClosedChannel(t *testing.T) {
	ch := make(EventChannel)
	close(ch)
	h := &Handler{channel: ch}

	// Sending to a closed channel panics; Enqueue must recover.
	h.Enqueue(event.GenericEvent{})
}

func TestHandler_MatchAndMatchProvider(t *testing.T) {
	p := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"},
	}
	h := &Handler{provider: p}

	if !h.Match(p, corev1.ObjectReference{Name: "p1", Namespace: "ns"}) {
		t.Fatalf("expected match")
	}
	if h.Match(p, corev1.ObjectReference{Name: "p2", Namespace: "ns"}) {
		t.Fatalf("expected no match")
	}
	if !h.MatchProvider(corev1.ObjectReference{Name: "p1", Namespace: "ns"}) {
		t.Fatalf("expected MatchProvider true")
	}
}

func TestHandler_StartedEndAndErrorEndedDoesNotRepair(t *testing.T) {
	h := &Handler{}
	h.Started(7)
	h.End()

	// When ended=true, Error() should not attempt to Repair() (so nil watch is safe).
	h.Error(nil, assertErr("boom"))
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
