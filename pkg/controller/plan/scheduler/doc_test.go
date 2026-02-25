package scheduler

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	"github.com/kubev2v/forklift/pkg/settings"
)

func TestNew_SupportedProviders(t *testing.T) {
	cases := []struct {
		name        string
		provider    api.ProviderType
		maxInFlight int
	}{
		{"VSphere", api.VSphere, 7},
		{"OVirt", api.OVirt, 3},
		{"OpenStack", api.OpenStack, 9},
		{"OpenShift", api.OpenShift, 2},
		{"Ova", api.Ova, 5},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			old := settings.Settings.MaxInFlight
			t.Cleanup(func() { settings.Settings.MaxInFlight = old })
			settings.Settings.MaxInFlight = tc.maxInFlight

			ctx := &plancontext.Context{}
			ctx.Source.Provider = &api.Provider{Spec: api.ProviderSpec{Type: &tc.provider}}
			s, err := New(ctx)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if s == nil {
				t.Fatalf("expected non-nil scheduler for %s", tc.name)
			}
		})
	}
}

func TestNew_UnsupportedProvider_ReturnsError(t *testing.T) {
	tp := api.ProviderType("nope")
	ctx := &plancontext.Context{}
	ctx.Source.Provider = &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	_, err := New(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
}
