package openstack

import (
	"context"
	"errors"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	providerapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/provider"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	webopenstack "github.com/kubev2v/forklift/pkg/controller/provider/web/openstack"
	watchhandler "github.com/kubev2v/forklift/pkg/controller/watch/handler"
	libweb "github.com/kubev2v/forklift/pkg/lib/inventory/web"
	"github.com/kubev2v/forklift/pkg/settings"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type listErrClient struct{ client.Client }

func (c listErrClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return errors.New("boom")
}

func TestNew_ReturnsHandler(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 1)
	h, err := New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if h == nil {
		t.Fatalf("expected handler")
	}
}

func TestHandler_Watch_ReturnsErrWhenCAFileMissing(t *testing.T) {
	oldCA := settings.Settings.Inventory.TLS.CA
	oldDev := settings.Settings.Development
	t.Cleanup(func() {
		settings.Settings.Inventory.TLS.CA = oldCA
		settings.Settings.Development = oldDev
	})
	settings.Settings.Inventory.TLS.CA = "/this/path/does/not/exist.pem"
	settings.Settings.Development = false

	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 1)
	h, err := New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err creating handler: %v", err)
	}
	if err := h.Watch(&watchhandler.WatchManager{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandler_CreatedUpdatedDeleted_EnqueueReferencedPlans(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	plan1 := &api.Plan{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "plan1"},
		Spec: api.PlanSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
			},
			VMs: []planapi.VM{{Ref: refapi.Ref{ID: "vm1"}}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, plan1).Build()
	ch := make(chan event.GenericEvent, 10)
	h, err := New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	vm := &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "vm1", Path: "/a/b/vm1"}}}
	h.Created(libweb.Event{Resource: vm})
	h.Updated(libweb.Event{Resource: vm, Updated: &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "vm1", Path: "/a/b/vm2"}}}})
	h.Deleted(libweb.Event{Resource: vm})

	if len(ch) != 3 {
		t.Fatalf("expected 3 enqueued, got %d", len(ch))
	}
}

func TestHandler_Updated_NoEnqueueWhenPathUnchanged(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	vm := &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "vm1", Path: "/a/b/vm1"}}}
	h.Updated(libweb.Event{Resource: vm, Updated: &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "vm1", Path: "/a/b/vm1"}}}})
	if len(ch) != 0 {
		t.Fatalf("expected no enqueue")
	}
}

func TestHandler_Changed_SkipsArchivedAndWrongProvider(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	archived := &api.Plan{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "arch"},
		Spec: api.PlanSpec{
			Archived: true,
			Provider: providerapi.Pair{
				Source: core.ObjectReference{Namespace: "ns", Name: "src"},
			},
			VMs: []planapi.VM{{Ref: refapi.Ref{ID: "vm1"}}},
		},
	}
	otherProv := &api.Plan{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "other"},
		Spec: api.PlanSpec{
			Provider: providerapi.Pair{
				Source: core.ObjectReference{Namespace: "ns", Name: "different"},
			},
			VMs: []planapi.VM{{Ref: refapi.Ref{ID: "vm1"}}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, archived, otherProv).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	vm := &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "vm1", Path: "/a/b/vm1"}}}
	h.changed(vm)
	if len(ch) != 0 {
		t.Fatalf("expected no enqueue")
	}
}

func TestHandler_Changed_BySuffixName(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	plan1 := &api.Plan{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "plan1"},
		Spec: api.PlanSpec{
			Provider: providerapi.Pair{
				Source: core.ObjectReference{Namespace: "ns", Name: "src"},
			},
			VMs: []planapi.VM{{Ref: refapi.Ref{Name: "vm-name"}}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, plan1).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	vm := &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "x", Path: "/a/b/vm-name"}}}
	h.changed(vm)
	if len(ch) != 1 {
		t.Fatalf("expected 1 enqueue, got %d", len(ch))
	}
}

func TestHandler_Changed_ListErrorDoesNotPanic(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	base.Client = listErrClient{Client: base.Client}
	h := &Handler{Handler: base}

	vm := &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "vm1", Path: "/a/b/vm1"}}}
	h.changed(vm)
	if len(ch) != 0 {
		t.Fatalf("expected no enqueue")
	}
}

func TestHandler_Created_IgnoresNonVMResource(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	h.Created(libweb.Event{Resource: &struct{}{}})
	if len(ch) != 0 {
		t.Fatalf("expected no enqueue")
	}
}

func TestHandler_Deleted_IgnoresNonVMResource(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	h.Deleted(libweb.Event{Resource: &struct{}{}})
	if len(ch) != 0 {
		t.Fatalf("expected no enqueue")
	}
}

func TestHandler_Updated_IgnoresNonVMResource(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	h.Updated(libweb.Event{Resource: &struct{}{}, Updated: &struct{}{}})
	if len(ch) != 0 {
		t.Fatalf("expected no enqueue")
	}
}

func TestHandler_Changed_NoVMs_NoEnqueue(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	plan1 := &api.Plan{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "plan1"},
		Spec: api.PlanSpec{
			Provider: providerapi.Pair{Source: core.ObjectReference{Namespace: "ns", Name: "src"}},
			VMs:      []planapi.VM{},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, plan1).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	vm := &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "vm1", Path: "/a/b/vm1"}}}
	h.changed(vm)
	if len(ch) != 0 {
		t.Fatalf("expected no enqueue")
	}
}

func TestHandler_Changed_MultipleModels_EnqueueOnce(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	plan1 := &api.Plan{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "plan1"},
		Spec: api.PlanSpec{
			Provider: providerapi.Pair{Source: core.ObjectReference{Namespace: "ns", Name: "src"}},
			VMs:      []planapi.VM{{Ref: refapi.Ref{ID: "vm2"}}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, plan1).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	vm1 := &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "vm1", Path: "/a/b/vm1"}}}
	vm2 := &webopenstack.VM{VM1: webopenstack.VM1{VM0: webopenstack.Resource{ID: "vm2", Path: "/a/b/vm2"}}}
	h.changed(vm1, vm2)
	if len(ch) != 1 {
		t.Fatalf("expected 1 enqueue, got %d", len(ch))
	}
}
