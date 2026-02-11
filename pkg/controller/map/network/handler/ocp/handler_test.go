package ocp

import (
	"context"
	"errors"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	providerapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/provider"
	watchhandler "github.com/kubev2v/forklift/pkg/controller/watch/handler"
	libweb "github.com/kubev2v/forklift/pkg/lib/inventory/web"
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

func TestHandler_GenerateEvents_EnqueuesForSourceOrDestinationProvider(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenShift
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}, Spec: api.ProviderSpec{Type: &tp}}

	mpSrc := &api.NetworkMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"},
		Spec: api.NetworkMapSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "p"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "other"},
			},
		},
	}
	mpDst := &api.NetworkMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dst"},
		Spec: api.NetworkMapSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "other"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "p"},
			},
		},
	}
	mpNone := &api.NetworkMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "none"},
		Spec: api.NetworkMapSpec{
			Provider: providerapi.Pair{
				Source:      core.ObjectReference{Namespace: "ns", Name: "other"},
				Destination: core.ObjectReference{Namespace: "ns", Name: "other2"},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov, mpSrc, mpDst, mpNone).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	h := &Handler{Handler: base}

	h.generateEvents()
	if len(ch) != 2 {
		t.Fatalf("expected 2 events enqueued, got %d", len(ch))
	}
}

func TestHandler_GenerateEvents_ListErrorDoesNotPanic(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenShift
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 10)
	base, _ := watchhandler.New(cl, ch, prov)
	base.Client = listErrClient{Client: base.Client}

	h := &Handler{Handler: base}
	h.generateEvents()
	if len(ch) != 0 {
		t.Fatalf("expected no events on list error")
	}
}

func TestNew_ReturnsHandler(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenShift
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

func TestHandler_Watch_EnsurePeriodicEventsAndStop(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenShift
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 1)
	h, err := New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	m := &watchhandler.WatchManager{}
	if err := h.Watch(m); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	m.End()
}

func TestHandler_CreatedAndDeleted_NoPanic(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	tp := api.OpenShift
	prov := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}, Spec: api.ProviderSpec{Type: &tp}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(prov).Build()
	ch := make(chan event.GenericEvent, 1)
	h, err := New(cl, ch, prov)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	h.Created(libweb.Event{})
	h.Deleted(libweb.Event{})
}
