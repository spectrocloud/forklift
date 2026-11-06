package nutanix

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/nutanix"
)

func storageMapFor(containers ...string) *api.StorageMap {
	m := &api.StorageMap{}
	for _, c := range containers {
		m.Spec.Map = append(m.Spec.Map, api.StoragePair{Source: ref.Ref{ID: c}})
	}
	return m
}

func networkMapFor(pairs ...api.NetworkPair) *api.NetworkMap {
	m := &api.NetworkMap{}
	m.Spec.Map = pairs
	return m
}

func TestDisksMapped(t *testing.T) {
	vm := &model.VM{}
	vm.Disks = []model.Disk{
		{UUID: "d0", AdapterType: adapterSCSI, StorageContainerUUID: "sc-a"},
		{UUID: "d1", AdapterType: adapterSCSI, StorageContainerUUID: "sc-b"},
		// CD-ROMs are not migrated, so their container need not be mapped.
		{UUID: "cd", IsCdrom: true, StorageContainerUUID: "sc-iso"},
	}

	if !disksMapped(vm, storageMapFor("sc-a", "sc-b")) {
		t.Error("expected all disks mapped")
	}
	if disksMapped(vm, storageMapFor("sc-a")) {
		t.Error("expected disk on sc-b to be reported unmapped")
	}
}

func TestNICsMapped(t *testing.T) {
	vm := &model.VM{}
	vm.NICs = []model.NIC{
		{MACAddress: "50:6b:8d:00:00:01", SubnetUUID: "subnet-a"},
		{MACAddress: "50:6b:8d:00:00:02", SubnetUUID: "subnet-b"},
	}
	podPair := api.NetworkPair{Source: api.NetworkSourceRef{Ref: ref.Ref{ID: "subnet-a"}}, Destination: api.DestinationNetwork{Type: "pod"}}
	ignoredPair := api.NetworkPair{Source: api.NetworkSourceRef{Ref: ref.Ref{ID: "subnet-b"}}, Destination: api.DestinationNetwork{Type: "ignored"}}

	if !nicsMapped(vm, networkMapFor(podPair, ignoredPair)) {
		t.Error("expected all NICs mapped, including one mapped to ignored")
	}
	if nicsMapped(vm, networkMapFor(podPair)) {
		t.Error("expected NIC on subnet-b to be reported unmapped")
	}
	if !nicsMapped(&model.VM{}, networkMapFor()) {
		t.Error("expected a VM without NICs to be mapped")
	}
}
