package openstack

import (
	"context"
	"errors"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
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

func TestHandler_Changed_EnqueuesReferencedMaps(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenStack
	prov := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"},
		Spec:       api.ProviderSpec{Type: &tp},
	}

	// Referenced by ID.
	mp1 := &api.NetworkMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "mp1"},
		Spec: api.NetworkMapSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
			},
			Map: []api.NetworkPair{
				{Source: refapi.Ref{ID: "n1"}},
			},
		},
	}
	// Not referenced.
	mp2 := &api.NetworkMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "mp2"},
		Spec: api.NetworkMapSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
			},
			Map: []api.NetworkPair{
				{Source: refapi.Ref{ID: "other"}},
			},
		},
	}
	// Different provider.
	mp3 := &api.NetworkMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "mp3"},
		Spec: api.NetworkMapSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "different"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
			},
			Map: []api.NetworkPair{
				{Source: refapi.Ref{ID: "n1"}},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, mp1, mp2, mp3).Build()
	ch := make(chan event.GenericEvent, 10)
	base, err := watchhandler.New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	h := &Handler{Handler: base}
	n := &webopenstack.Network{Resource: webopenstack.Resource{ID: "n1", Path: "/a/b/net1"}}
	h.changed(n)

	select {
	case ev := <-ch:
		if ev.Object.GetName() != "mp1" {
			t.Fatalf("expected mp1 enqueued, got %s", ev.Object.GetName())
		}
	default:
		t.Fatalf("expected one event enqueued")
	}
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

func TestHandler_CreatedAndDeleted_Enqueue(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	mp := &api.NetworkMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "mp1"},
		Spec: api.NetworkMapSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
			},
			Map: []api.NetworkPair{
				{Source: refapi.Ref{ID: "n1"}},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, mp).Build()
	ch := make(chan event.GenericEvent, 10)
	h, err := New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	n := &webopenstack.Network{Resource: webopenstack.Resource{ID: "n1", Path: "/a/b/net1"}}
	h.Created(libweb.Event{Resource: n})
	h.Deleted(libweb.Event{Resource: n})
	if len(ch) != 2 {
		t.Fatalf("expected 2 events enqueued, got %d", len(ch))
	}
}

func TestHandler_Updated_OnlyWhenPathChanges(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenStack
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	n1 := &webopenstack.Network{Resource: webopenstack.Resource{ID: "n1", Path: "/a/b/net"}}
	n2 := &webopenstack.Network{Resource: webopenstack.Resource{ID: "n1", Path: "/a/b/net"}} // same
	h.Updated(libweb.Event{Resource: n1, Updated: n2})
	if len(ch) != 0 {
		t.Fatalf("expected no enqueue when path unchanged")
	}

	n3 := &webopenstack.Network{Resource: webopenstack.Resource{ID: "n1", Path: "/a/b/net2"}} // changed
	h.Updated(libweb.Event{Resource: n1, Updated: n3})
	// May enqueue only if there are NetworkMaps; here none, but we cover branch.
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
	n := &webopenstack.Network{Resource: webopenstack.Resource{ID: "n1", Path: "/a/b/net1"}}
	h.changed(n)
	if len(ch) != 0 {
		t.Fatalf("expected no events on list error")
	}
}
