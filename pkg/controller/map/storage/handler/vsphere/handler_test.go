package vsphere

import (
	"context"
	"errors"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	providerapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/provider"
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

func TestHandler_Changed_EnqueuesReferencedMaps(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.VSphere
	prov := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"},
		Spec:       api.ProviderSpec{Type: &tp},
	}

	mp1 := &api.StorageMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "mp1"},
		Spec: api.StorageMapSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
			},
			Map: []api.StoragePair{
				{Source: refapi.Ref{Name: "ds1"}},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, mp1).Build()
	ch := make(chan event.GenericEvent, 10)
	base, err := watchhandler.New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	h := &Handler{Handler: base}
	ds := &webvsphere.Datastore{Resource: webvsphere.Resource{ID: "dsid", Path: "/a/b/ds1"}}
	h.changed(ds)

	select {
	case ev := <-ch:
		if ev.Object.GetName() != "mp1" {
			t.Fatalf("expected mp1 enqueued, got %s", ev.Object.GetName())
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
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	a := &webvsphere.Datastore{Resource: webvsphere.Resource{ID: "ds", Path: "/p"}}
	b := &webvsphere.Datastore{Resource: webvsphere.Resource{ID: "ds", Path: "/p"}}
	h.Updated(libweb.Event{Resource: a, Updated: b})

	c := &webvsphere.Datastore{Resource: webvsphere.Resource{ID: "ds", Path: "/p2"}}
	h.Updated(libweb.Event{Resource: a, Updated: c})
}

func TestHandler_Changed_ListErrorDoesNotPanic(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.VSphere
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	base.Client = listErrClient{Client: base.Client}

	h := &Handler{Handler: base}
	ds := &webvsphere.Datastore{Resource: webvsphere.Resource{ID: "ds", Path: "/p"}}
	h.changed(ds)
	if len(ch) != 0 {
		t.Fatalf("expected no events on list error")
	}
}

func TestNew_ReturnsHandler(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	tp := api.VSphere
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
	tp := api.VSphere
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

func TestHandler_CreatedAndDeleted_Enqueue(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)
	tp := api.VSphere
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	mp := &api.StorageMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "mp1"},
		Spec: api.StorageMapSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
			},
			Map: []api.StoragePair{
				{Source: refapi.Ref{Name: "ds1"}},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, mp).Build()
	ch := make(chan event.GenericEvent, 10)
	h, err := New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	ds := &webvsphere.Datastore{Resource: webvsphere.Resource{ID: "dsid", Path: "/a/b/ds1"}}
	h.Created(libweb.Event{Resource: ds})
	h.Deleted(libweb.Event{Resource: ds})
	if len(ch) != 2 {
		t.Fatalf("expected 2 events enqueued, got %d", len(ch))
	}
}
