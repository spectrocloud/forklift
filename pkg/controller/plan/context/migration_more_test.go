package context

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNotEnoughDataError_Error(t *testing.T) {
	var e NotEnoughDataError
	if e.Error() == "" {
		t.Fatalf("expected message")
	}
}

func TestNew_ReturnsErrorWhenNetworkMapMissing(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	plan := &api.Plan{}
	plan.Referenced.Map.Network = nil
	plan.Referenced.Map.Storage = &api.StorageMap{}
	_, err := New(cl, plan, logging.WithName("t"))
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestNew_ReturnsErrorWhenStorageMapMissing(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	plan := &api.Plan{}
	plan.Referenced.Map.Network = &api.NetworkMap{}
	plan.Referenced.Map.Storage = nil
	_, err := New(cl, plan, logging.WithName("t"))
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestContext_SetMigration_NilNoChange(t *testing.T) {
	ctx := &Context{Migration: &api.Migration{}, Log: logging.WithName("t")}
	ctx.SetMigration(nil)
	if ctx.Migration == nil {
		t.Fatalf("expected migration unchanged")
	}
}

func TestContext_SetMigration_SetsMigration(t *testing.T) {
	ctx := &Context{Migration: &api.Migration{}, Log: logging.WithName("t")}
	m := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m1"}}
	ctx.SetMigration(m)
	if ctx.Migration != m {
		t.Fatalf("expected migration set")
	}
}

func TestContext_FindHook_Found(t *testing.T) {
	h1 := &api.Hook{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h1"}}
	h2 := &api.Hook{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h2"}}
	ctx := &Context{Hooks: []*api.Hook{h1, h2}}
	h, found := ctx.FindHook(core.ObjectReference{Namespace: "ns", Name: "h2"})
	if !found || h == nil || h.Name != "h2" {
		t.Fatalf("expected found h2, got found=%v hook=%#v", found, h)
	}
}

func TestContext_FindHook_NotFound(t *testing.T) {
	h1 := &api.Hook{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h1"}}
	ctx := &Context{Hooks: []*api.Hook{h1}}
	_, found := ctx.FindHook(core.ObjectReference{Namespace: "ns", Name: "missing"})
	if found {
		t.Fatalf("expected not found")
	}
}
