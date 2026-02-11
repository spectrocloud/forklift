package ovirt

import (
	"context"
	"testing"
	"time"

	"github.com/kubev2v/forklift/pkg/lib/logging"
)

func TestVMEventHandler_tripLatch_IsNonBlocking(t *testing.T) {
	h := &VMEventHandler{
		log:   logging.WithName("ovirt-watch-test"),
		latch: make(chan int8, 1),
	}
	// trip twice; second should not block (default branch).
	h.tripLatch()
	h.tripLatch()
}

func TestVMEventHandler_canceled(t *testing.T) {
	h := &VMEventHandler{log: logging.WithName("ovirt-watch-test")}
	h.context, h.cancel = context.WithCancel(context.Background())
	if h.canceled() {
		t.Fatalf("expected not canceled")
	}
	h.cancel()
	time.Sleep(time.Millisecond)
	if !h.canceled() {
		t.Fatalf("expected canceled")
	}
}
