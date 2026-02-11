package openstack

import (
	"context"
	"testing"
	"time"
)

func TestAdapterList_IsInitialized(t *testing.T) {
	if len(adapterList) < 5 {
		t.Fatalf("expected adapterList initialized, got %d", len(adapterList))
	}
}

func TestContext_canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Context{ctx: ctx}
	if c.canceled() {
		t.Fatalf("expected not canceled yet")
	}
	cancel()
	// allow cancellation to propagate
	time.Sleep(time.Millisecond)
	if !c.canceled() {
		t.Fatalf("expected canceled")
	}
}
