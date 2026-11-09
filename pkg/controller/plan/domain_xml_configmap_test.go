package plan

import (
	"context"
	"testing"

	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	v1 "k8s.io/api/core/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The CDI cert ConfigMap is looked up by vmLabels; the domain-XML ConfigMap
// for the same VM must never match that selector.
func TestDomainXMLConfigMapNotSelectedAsCertConfigMap(t *testing.T) {
	kv := createKubeVirtWithProvider(v1beta1.Nutanix)
	vmRef := ref.Ref{ID: "vm-1"}

	created, err := kv.ensureDomainXMLConfigMap(vmRef, "<domain/>")
	if err != nil {
		t.Fatalf("ensureDomainXMLConfigMap: %v", err)
	}

	list := &v1.ConfigMapList{}
	err = kv.Destination.List(context.TODO(), list, &client.ListOptions{
		LabelSelector: k8slabels.SelectorFromSet(kv.vmLabels(vmRef)),
		Namespace:     kv.Plan.Spec.TargetNamespace,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("cert ConfigMap selector matched the domain-XML ConfigMap: %+v", list.Items[0].Labels)
	}

	// It still finds and updates its own ConfigMap.
	updated, err := kv.ensureDomainXMLConfigMap(vmRef, "<domain type='kvm'/>")
	if err != nil {
		t.Fatalf("second ensureDomainXMLConfigMap: %v", err)
	}
	if updated.Name != created.Name {
		t.Fatalf("expected the same ConfigMap, got %q then %q", created.Name, updated.Name)
	}
	if updated.Data["input.xml"] != "<domain type='kvm'/>" {
		t.Fatalf("input.xml not updated: %q", updated.Data["input.xml"])
	}
}
