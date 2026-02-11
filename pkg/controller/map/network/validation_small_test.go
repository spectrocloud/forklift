package network

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	providerapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/provider"
	"github.com/kubev2v/forklift/pkg/controller/validation"
	core "k8s.io/api/core/v1"
)

func TestReconciler_validate_EarlyReturnOnInvalidProviders(t *testing.T) {
	// Provider refs not set => ProviderPair validation returns conditions,
	// and validate() returns early (no inventory/web calls).
	r := &Reconciler{}
	mp := &api.NetworkMap{}
	mp.Spec.Provider = providerapi.Pair{
		Source:      core.ObjectReference{},
		Destination: core.ObjectReference{},
	}

	if err := r.validate(mp); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !mp.Status.HasCondition(validation.SourceProviderNotValid) {
		t.Fatalf("expected SourceProviderNotValid condition")
	}
	if !mp.Status.HasCondition(validation.DestinationProviderNotValid) {
		t.Fatalf("expected DestinationProviderNotValid condition")
	}
}
