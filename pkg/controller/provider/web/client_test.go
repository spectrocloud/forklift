package web

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
)

func TestNewClient_UnsupportedProviderType(t *testing.T) {
	p := &api.Provider{} // Spec.Type nil => Undefined
	_, err := NewClient(p)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewClient_SupportedProviderTypes(t *testing.T) {
	for _, pt := range []api.ProviderType{api.OpenShift, api.VSphere, api.OVirt, api.OpenStack, api.Ova} {
		pt := pt
		t.Run(pt.String(), func(t *testing.T) {
			p := &api.Provider{}
			p.Spec.Type = &pt
			c, err := NewClient(p)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c == nil {
				t.Fatalf("expected client")
			}
		})
	}
}
