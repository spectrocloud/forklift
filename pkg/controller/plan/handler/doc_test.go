package handler

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestNew_SupportedProviderTypes(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	for _, tp := range []api.ProviderType{api.OpenShift, api.VSphere, api.OVirt, api.OpenStack, api.Ova} {
		tp := tp
		t.Run(string(tp), func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(s).Build()
			ch := make(chan event.GenericEvent, 1)
			p := &api.Provider{}
			p.Spec.Type = &tp
			h, err := New(cl, ch, p)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if h == nil {
				t.Fatalf("expected handler")
			}
		})
	}
}

func TestNew_ProviderNotSupported(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	cl := fake.NewClientBuilder().WithScheme(s).Build()
	ch := make(chan event.GenericEvent, 1)
	tp := api.ProviderType("Nope")
	p := &api.Provider{}
	p.Spec.Type = &tp
	if _, err := New(cl, ch, p); err == nil {
		t.Fatalf("expected error")
	}
}
