package vsphere

import (
	"context"
	"errors"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	webvsphere "github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"
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

func TestHandler_Changed_EnqueuesReferencedHosts(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.VSphere
	prov := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec:       api.ProviderSpec{Type: &tp},
	}

	h1 := &api.Host{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h1"},
		Spec: api.HostSpec{
			Provider: core.ObjectReference{Namespace: "ns", Name: "p"},
			Ref:      refapi.Ref{ID: "host-1"},
		},
	}
	h2 := &api.Host{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h2"},
		Spec: api.HostSpec{
			Provider: core.ObjectReference{Namespace: "ns", Name: "p"},
			Ref:      refapi.Ref{ID: "other"},
		},
	}
	hOtherProv := &api.Host{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h3"},
		Spec: api.HostSpec{
			Provider: core.ObjectReference{Namespace: "ns", Name: "other"},
			Ref:      refapi.Ref{ID: "host-1"},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, h1, h2, hOtherProv).Build()
	ch := make(chan event.GenericEvent, 10)
	base, err := watchhandler.New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	h := &Handler{Handler: base}
	hostModel := &webvsphere.Host{Resource: webvsphere.Resource{ID: "host-1", Path: "/dc/cluster/host1"}}
	h.changed(hostModel)

	select {
	case ev := <-ch:
		if ev.Object.GetName() != "h1" {
			t.Fatalf("expected h1 enqueued, got %s", ev.Object.GetName())
		}
	default:
		t.Fatalf("expected one event enqueued")
	}
}

func TestHandler_Updated_BranchCoverage(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.VSphere
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	a := &webvsphere.Host{Resource: webvsphere.Resource{ID: "h", Path: "/p"}}
	b := &webvsphere.Host{Resource: webvsphere.Resource{ID: "h", Path: "/p"}}
	h.Updated(libweb.Event{Resource: a, Updated: b})

	c := &webvsphere.Host{Resource: webvsphere.Resource{ID: "h", Path: "/p2"}}
	h.Updated(libweb.Event{Resource: a, Updated: c})
}

func TestHandler_Changed_ListErrorDoesNotPanic(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.VSphere
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	base.Client = listErrClient{Client: base.Client}

	h := &Handler{Handler: base}
	hostModel := &webvsphere.Host{Resource: webvsphere.Resource{ID: "host-1", Path: "/dc/cluster/host1"}}
	h.changed(hostModel)
	if len(ch) != 0 {
		t.Fatalf("expected no events on list error")
	}
}

func TestNew_ReturnsHandler(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.VSphere
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}, Spec: api.ProviderSpec{Type: &tp}}
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
	tp := api.VSphere
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}, Spec: api.ProviderSpec{Type: &tp}}
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

func TestHandler_CreatedAndDeleted_Enqueue(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.VSphere
	prov := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec:       api.ProviderSpec{Type: &tp},
	}
	h1 := &api.Host{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h1"},
		Spec: api.HostSpec{
			Provider: core.ObjectReference{Namespace: "ns", Name: "p"},
			Ref:      refapi.Ref{ID: "host-1"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, h1).Build()
	ch := make(chan event.GenericEvent, 10)
	h, err := New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	hostModel := &webvsphere.Host{Resource: webvsphere.Resource{ID: "host-1", Path: "/dc/cluster/host1"}}
	h.Created(libweb.Event{Resource: hostModel})
	h.Deleted(libweb.Event{Resource: hostModel})
	if len(ch) != 2 {
		t.Fatalf("expected 2 events enqueued, got %d", len(ch))
	}
}
