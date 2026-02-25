package openstack

import (
	"context"
	"testing"
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
	// cancel() closes the Done channel synchronously; no sleep needed.
	if !c.canceled() {
		t.Fatalf("expected canceled")
	}
}
