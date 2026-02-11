package handler

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestNew_ReturnsHandlerForSupportedTypes(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	cl := fake.NewClientBuilder().WithScheme(s).Build()
	ch := make(chan event.GenericEvent, 1)

	cases := []api.ProviderType{api.OpenShift, api.VSphere, api.OVirt, api.OpenStack, api.Ova}
	for _, pt := range cases {
		pt := pt
		p := &api.Provider{}
		p.Spec.Type = &pt
		h, err := New(cl, ch, p)
		if err != nil {
			t.Fatalf("unexpected err for %s: %v", pt, err)
		}
		if h == nil {
			t.Fatalf("expected handler for %s", pt)
		}
	}
}

func TestNew_ProviderNotSupported(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	cl := fake.NewClientBuilder().WithScheme(s).Build()
	ch := make(chan event.GenericEvent, 1)

	p := &api.Provider{} // Type() => Undefined
	if _, err := New(cl, ch, p); err == nil {
		t.Fatalf("expected error for undefined provider type")
	}
}
