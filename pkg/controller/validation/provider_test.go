package validation

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	providerapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/provider"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestProvider_Validate_NotSetNotFoundNotReadyReady(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)

	pv := &Provider{Client: fake.NewClientBuilder().WithScheme(s).Build()}

	// NotSet.
	cnds, err := pv.Validate(core.ObjectReference{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !cnds.HasCondition(ProviderNotValid) {
		t.Fatalf("expected ProviderNotValid")
	}

	// NotFound.
	cnds, err = pv.Validate(core.ObjectReference{Namespace: "ns", Name: "missing"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !cnds.HasCondition(ProviderNotValid) {
		t.Fatalf("expected ProviderNotValid")
	}

	// NotReady.
	tp := api.VSphere
	p := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec:       api.ProviderSpec{Type: &tp},
	}
	pv2 := &Provider{Client: fake.NewClientBuilder().WithScheme(s).WithObjects(p).Build()}
	cnds, err = pv2.Validate(core.ObjectReference{Namespace: "ns", Name: "p"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !cnds.HasCondition(ProviderNotReady) {
		t.Fatalf("expected ProviderNotReady")
	}

	// Ready => no ProviderNotReady.
	p.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True, Category: libcnd.Required})
	pv3 := &Provider{Client: fake.NewClientBuilder().WithScheme(s).WithObjects(p).Build()}
	cnds, err = pv3.Validate(core.ObjectReference{Namespace: "ns", Name: "p"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cnds.HasCondition(ProviderNotReady) {
		t.Fatalf("did not expect ProviderNotReady when ready")
	}
}

func TestProviderPair_Validate_SourceAndDestinationTypeRules(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)

	vs := api.VSphere
	ocp := api.OpenShift

	src := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "src"},
		Spec:       api.ProviderSpec{Type: &vs},
	}
	src.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True, Category: libcnd.Required})

	dstBad := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dstbad"},
		Spec:       api.ProviderSpec{Type: &vs},
	}
	dstBad.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True, Category: libcnd.Required})

	dstOk := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "dstok"},
		Spec:       api.ProviderSpec{Type: &ocp},
	}
	dstOk.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True, Category: libcnd.Required})

	pv := &ProviderPair{Client: fake.NewClientBuilder().WithScheme(s).WithObjects(src, dstBad, dstOk).Build()}

	// Destination not OpenShift => DestinationProviderNotValid set (type rule).
	cnds, err := pv.Validate(providerapi.Pair{
		Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
		Destination: core.ObjectReference{Namespace: "ns", Name: "dstbad"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !cnds.HasCondition(DestinationProviderNotValid) {
		t.Fatalf("expected DestinationProviderNotValid for non-OpenShift destination")
	}

	// Destination OpenShift => no type-not-valid condition.
	cnds, err = pv.Validate(providerapi.Pair{
		Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
		Destination: core.ObjectReference{Namespace: "ns", Name: "dstok"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cnds.HasCondition(DestinationProviderNotValid) && cnds.FindCondition(DestinationProviderNotValid).Reason == TypeNotValid {
		t.Fatalf("did not expect DestinationProviderNotValid(TypeNotValid) for OpenShift destination")
	}
}
