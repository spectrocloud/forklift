package openstack

import (
	"errors"
	"testing"

	v1beta1 "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	planbase "github.com/kubev2v/forklift/pkg/controller/plan/adapter/base"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	"github.com/kubev2v/forklift/pkg/controller/provider/web"
	webbase "github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	ocpweb "github.com/kubev2v/forklift/pkg/controller/provider/web/ocp"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/openstack"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cnv "kubevirt.io/api/core/v1"
)

type stubInv2 struct {
	findFn func(resource interface{}, rf refapi.Ref) error
	getFn  func(resource interface{}, id string) error
	listFn func(list interface{}, param ...web.Param) error

	listCalls int
}

func (s *stubInv2) Finder() web.Finder { return nil }
func (s *stubInv2) Get(resource interface{}, id string) error {
	if s.getFn != nil {
		return s.getFn(resource, id)
	}
	return nil
}
func (s *stubInv2) List(list interface{}, param ...web.Param) error {
	s.listCalls++
	if s.listFn != nil {
		return s.listFn(list, param...)
	}
	return nil
}
func (s *stubInv2) Watch(resource interface{}, h web.EventHandler) (*web.Watch, error) {
	return nil, nil
}
func (s *stubInv2) Find(resource interface{}, rf refapi.Ref) error {
	if s.findFn != nil {
		return s.findFn(resource, rf)
	}
	return nil
}
func (s *stubInv2) VM(rf *refapi.Ref) (interface{}, error)       { return struct{}{}, nil }
func (s *stubInv2) Workload(rf *refapi.Ref) (interface{}, error) { return struct{}{}, nil }
func (s *stubInv2) Network(rf *refapi.Ref) (interface{}, error)  { return struct{}{}, nil }
func (s *stubInv2) Storage(rf *refapi.Ref) (interface{}, error)  { return struct{}{}, nil }
func (s *stubInv2) Host(rf *refapi.Ref) (interface{}, error)     { return struct{}{}, nil }

func setOCPVMInterfaces(vm *ocpweb.VM, macs ...string) {
	vm.Object.Spec.Template = &cnv.VirtualMachineInstanceTemplateSpec{}
	ifaces := make([]cnv.Interface, 0, len(macs))
	for _, mac := range macs {
		ifaces = append(ifaces, cnv.Interface{MacAddress: mac})
	}
	vm.Object.Spec.Template.Spec.Domain.Devices.Interfaces = ifaces
}

func mkWorkloadForNet(imageID string, netName, netID string, mac string) *model.Workload {
	w := &model.Workload{}
	w.ID = "vm1"
	w.ImageID = imageID
	w.Image.Properties = map[string]interface{}{}
	w.Flavor.ExtraSpecs = map[string]string{}
	w.Networks = []model.Network{
		{Resource: model.Resource{ID: netID, Name: netName}},
	}
	w.Addresses = map[string]interface{}{
		netName: []interface{}{
			map[string]interface{}{
				"OS-EXT-IPS-MAC:mac_addr": mac,
				"OS-EXT-IPS:type":         "fixed",
			},
		},
	}
	return w
}

func TestBuilder_macConflicts_CachesListAndFindsConflict(t *testing.T) {
	b := createBuilder()

	dst := &stubInv2{}
	dst.listFn = func(list interface{}, param ...web.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa")
		*vms = append(*vms, kvm)
		return nil
	}
	b.Destination.Inventory = dst

	w := &model.Workload{}
	w.Addresses = map[string]interface{}{
		"net": []interface{}{
			map[string]interface{}{"OS-EXT-IPS-MAC:mac_addr": "aa"},
		},
	}

	conf, err := b.macConflicts(w)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(conf) != 1 || conf[0] != "ns/vm" {
		t.Fatalf("unexpected conflicts: %#v", conf)
	}

	// cached map => List should not be called again
	_, _ = b.macConflicts(w)
	if dst.listCalls != 1 {
		t.Fatalf("expected listCalls=1 got %d", dst.listCalls)
	}
}

func TestBuilder_macConflicts_IgnoresUnexpectedAddressShapes(t *testing.T) {
	b := createBuilder()
	b.Destination.Inventory = &stubInv2{listFn: func(list interface{}, param ...web.Param) error { return nil }}

	w := &model.Workload{}
	w.Addresses = map[string]interface{}{
		"net": map[string]any{"not": "a-slice"},
	}
	conf, err := b.macConflicts(w)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(conf) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_mapNetworks_ErrWhenNoNetworkMap(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{Spec: v1beta1.NetworkMapSpec{Map: []v1beta1.NetworkPair{}}}

	w := mkWorkloadForNet("img", "net1", "nid1", "aa")
	spec := newVMSpec()
	if err := b.mapNetworks(w, spec); err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapNetworks_PodMultusIgnoredFloatingAndMultiqueue(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{
		Spec: v1beta1.NetworkMapSpec{
			Map: []v1beta1.NetworkPair{
				{Source: refapi.Ref{ID: "nid1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}},
				{Source: refapi.Ref{ID: "nid2"}, Destination: v1beta1.DestinationNetwork{Type: Multus, Namespace: "ns", Name: "nad"}},
				{Source: refapi.Ref{ID: "nid3"}, Destination: v1beta1.DestinationNetwork{Type: Ignored}},
			},
		},
	}

	w := &model.Workload{}
	w.ImageID = "img"
	w.Image.Properties = map[string]interface{}{
		VifModel:             VifModelVirtualE1000,
		VifMultiQueueEnabled: "true",
	}
	w.Flavor.ExtraSpecs = map[string]string{}
	w.Networks = []model.Network{
		{Resource: model.Resource{ID: "nid1", Name: "net1"}},
		{Resource: model.Resource{ID: "nid2", Name: "net2"}},
		{Resource: model.Resource{ID: "nid3", Name: "net3"}},
	}
	w.Addresses = map[string]interface{}{
		"net1": []interface{}{
			map[string]interface{}{"OS-EXT-IPS-MAC:mac_addr": "aa", "OS-EXT-IPS:type": "fixed"},
			map[string]interface{}{"OS-EXT-IPS-MAC:mac_addr": "ff", "OS-EXT-IPS:type": "floating"}, // should be skipped
		},
		"net2": []interface{}{
			map[string]interface{}{"OS-EXT-IPS-MAC:mac_addr": "bb", "OS-EXT-IPS:type": "fixed"},
		},
		"net3": []interface{}{
			map[string]interface{}{"OS-EXT-IPS-MAC:mac_addr": "cc", "OS-EXT-IPS:type": "fixed"}, // ignored map => skipped
		},
	}

	spec := newVMSpec()
	if err := b.mapNetworks(w, spec); err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	if len(spec.Template.Spec.Networks) != 2 || len(spec.Template.Spec.Domain.Devices.Interfaces) != 2 {
		t.Fatalf("expected 2 networks/interfaces")
	}
	// verify multi-queue enabled
	if spec.Template.Spec.Domain.Devices.NetworkInterfaceMultiQueue == nil || *spec.Template.Spec.Domain.Devices.NetworkInterfaceMultiQueue != true {
		t.Fatalf("expected multiqueue enabled")
	}

	// Order depends on map iteration; check membership.
	seen := map[string]cnv.Interface{}
	for _, itf := range spec.Template.Spec.Domain.Devices.Interfaces {
		seen[itf.MacAddress] = itf
	}
	if seen["aa"].Masquerade == nil {
		t.Fatalf("expected pod masquerade for aa")
	}
	if seen["aa"].Model != VifModelE1000 {
		t.Fatalf("expected e1000 model mapping, got %q", seen["aa"].Model)
	}
	if seen["bb"].Bridge == nil {
		t.Fatalf("expected multus bridge for bb")
	}
}

func TestBuilder_mapDisks_FallsBackBootOrderToImagePVC(t *testing.T) {
	b := createBuilder()
	b.Source.Inventory = &stubInv2{
		findFn: func(resource interface{}, rf refapi.Ref) error {
			switch r := resource.(type) {
			case *model.Image:
				// Inventory Find(image, Ref{ID: pvc.Labels["imageID"]})
				r.DiskFormat = QCOW2
				r.Properties = map[string]interface{}{
					forkliftPropertyOriginalImageID: "vmimg",
				}
				return nil
			default:
				return nil
			}
		},
	}

	w := &model.Workload{}
	w.ImageID = "vmimg"
	w.Image.Properties = map[string]interface{}{DiskBus: VirtioBus}
	w.Volumes = nil

	spec := newVMSpec()
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = "pvc1"
	pvc.Labels = map[string]string{"imageID": "img1"}
	pvc.Annotations = map[string]string{planbase.AnnDiskSource: "disk0"}

	b.mapDisks(w, []*corev1.PersistentVolumeClaim{pvc}, spec)
	if len(spec.Template.Spec.Domain.Devices.Disks) != 1 {
		t.Fatalf("expected 1 disk")
	}
	if spec.Template.Spec.Domain.Devices.Disks[0].BootOrder == nil || *spec.Template.Spec.Domain.Devices.Disks[0].BootOrder != 1 {
		t.Fatalf("expected bootorder=1")
	}
}

func TestBuilder_mapDisks_BootableVolumeSetsBootOrder(t *testing.T) {
	b := createBuilder()
	b.Source.Inventory = &stubInv2{
		findFn: func(resource interface{}, rf refapi.Ref) error {
			switch r := resource.(type) {
			case *model.Image:
				r.DiskFormat = RAW
				r.Properties = map[string]interface{}{
					forkliftPropertyOriginalVolumeID: "vol-1",
				}
				return nil
			default:
				return nil
			}
		},
		getFn: func(resource interface{}, id string) error {
			vol := resource.(*model.Volume)
			vol.Bootable = "true"
			return nil
		},
	}

	w := &model.Workload{}
	w.ImageID = "vmimg"
	w.Image.Properties = map[string]interface{}{}

	spec := newVMSpec()
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = "pvc1"
	pvc.Labels = map[string]string{"imageID": "img1"}
	pvc.Annotations = map[string]string{planbase.AnnDiskSource: "disk0"}

	b.mapDisks(w, []*corev1.PersistentVolumeClaim{pvc}, spec)
	if len(spec.Template.Spec.Domain.Devices.Disks) != 1 {
		t.Fatalf("expected 1 disk")
	}
	if spec.Template.Spec.Domain.Devices.Disks[0].BootOrder == nil || *spec.Template.Spec.Domain.Devices.Disks[0].BootOrder != 1 {
		t.Fatalf("expected bootorder=1")
	}
}

func TestBuilder_mapDisks_ISOUsesCDRom(t *testing.T) {
	b := createBuilder()
	b.Source.Inventory = &stubInv2{
		findFn: func(resource interface{}, rf refapi.Ref) error {
			img := resource.(*model.Image)
			img.DiskFormat = ISO
			img.Properties = map[string]interface{}{
				forkliftPropertyOriginalImageID: "vmimg",
			}
			return nil
		},
	}

	w := &model.Workload{}
	w.ImageID = "vmimg"
	w.Image.Properties = map[string]interface{}{CdromBus: IdeBus}

	spec := newVMSpec()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pvc1",
			Labels:      map[string]string{"imageID": "img1"},
			Annotations: map[string]string{planbase.AnnDiskSource: "disk0"},
		},
	}
	b.mapDisks(w, []*corev1.PersistentVolumeClaim{pvc}, spec)
	if spec.Template.Spec.Domain.Devices.Disks[0].CDRom == nil {
		t.Fatalf("expected cdrom")
	}
}

func TestBuilder_Tasks_CreatesImageAndVolumeTasks(t *testing.T) {
	b := createBuilder()
	src := &stubInv2{}
	src.findFn = func(resource interface{}, rf refapi.Ref) error {
		w := resource.(*model.Workload)
		w.ID = "vm1"
		w.ImageID = "img"
		w.Image.SizeBytes = 10 * 1024 * 1024
		w.Volumes = []model.Volume{{Resource: model.Resource{ID: "vol1"}, Size: 2}}
		return nil
	}
	b.Source.Inventory = src

	tasks, err := b.Tasks(refapi.Ref{ID: "vm1"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks got %d", len(tasks))
	}
	seen := map[string]int64{}
	for _, tk := range tasks {
		seen[tk.Name] = tk.Progress.Total
	}
	if _, ok := seen[getVmSnapshotName(b.Context, "vm1")]; !ok {
		t.Fatalf("missing image task")
	}
	if _, ok := seen[getImageFromVolumeName(b.Context, "vm1", "vol1")]; !ok {
		t.Fatalf("missing volume task")
	}
}

func TestBuilder_VirtualMachine_HappyPath_CreatesDisksAndNetworks(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{
		Spec: v1beta1.NetworkMapSpec{
			Map: []v1beta1.NetworkPair{
				{Source: refapi.Ref{ID: "nid1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}},
			},
		},
	}
	b.Destination.Inventory = &stubInv2{listFn: func(list interface{}, param ...web.Param) error { return nil }}

	src := &stubInv2{}
	src.findFn = func(resource interface{}, rf refapi.Ref) error {
		switch r := resource.(type) {
		case *model.Workload:
			*r = *mkWorkloadForNet("vmimg", "net1", "nid1", "aa")
			r.Flavor.VCPUs = 2
			r.Flavor.RAM = 1024
			r.ImageID = "vmimg"
			r.Image.Properties = map[string]interface{}{DiskBus: VirtioBus}
			return nil
		case *model.Image:
			r.DiskFormat = QCOW2
			r.Properties = map[string]interface{}{
				forkliftPropertyOriginalImageID: "vmimg",
			}
			return nil
		default:
			return nil
		}
	}
	b.Source.Inventory = src

	spec := &cnv.VirtualMachineSpec{} // Template nil should be created
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pvc1",
			Labels:      map[string]string{"imageID": "img1"},
			Annotations: map[string]string{planbase.AnnDiskSource: "disk0"},
		},
	}
	err := b.VirtualMachine(refapi.Ref{ID: "vm1"}, spec, []*corev1.PersistentVolumeClaim{pvc}, false, false)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if spec.Template == nil {
		t.Fatalf("expected template")
	}
	if len(spec.Template.Spec.Volumes) != 1 || len(spec.Template.Spec.Domain.Devices.Disks) != 1 {
		t.Fatalf("expected 1 volume/disk")
	}
	if len(spec.Template.Spec.Networks) != 1 || len(spec.Template.Spec.Domain.Devices.Interfaces) != 1 {
		t.Fatalf("expected 1 network/interface")
	}
}

func TestBuilder_VirtualMachine_ErrOnMacConflict(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{Spec: v1beta1.NetworkMapSpec{Map: []v1beta1.NetworkPair{}}}

	dst := &stubInv2{}
	dst.listFn = func(list interface{}, param ...web.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa")
		*vms = append(*vms, kvm)
		return nil
	}
	b.Destination.Inventory = dst

	src := &stubInv2{}
	src.findFn = func(resource interface{}, rf refapi.Ref) error {
		switch r := resource.(type) {
		case *model.Workload:
			r.Addresses = map[string]interface{}{"net": []interface{}{map[string]interface{}{"OS-EXT-IPS-MAC:mac_addr": "aa"}}}
			r.Image.Properties = map[string]interface{}{}
			return nil
		default:
			return nil
		}
	}
	b.Source.Inventory = src

	err := b.VirtualMachine(refapi.Ref{ID: "vm1"}, newVMSpec(), nil, false, false)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapNetworks_DoesNotSetMultiqueueOnBadBool(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{
		Spec: v1beta1.NetworkMapSpec{
			Map: []v1beta1.NetworkPair{{Source: refapi.Ref{ID: "nid1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}}},
		},
	}
	w := mkWorkloadForNet("img", "net1", "nid1", "aa")
	w.Image.Properties[VifMultiQueueEnabled] = "not-bool"
	spec := newVMSpec()
	_ = b.mapNetworks(w, spec)
	if spec.Template.Spec.Domain.Devices.NetworkInterfaceMultiQueue != nil {
		t.Fatalf("expected nil multiqueue")
	}
}

func TestBuilder_Tasks_WrapsFindError(t *testing.T) {
	b := createBuilder()
	b.Source.Inventory = &stubInv2{findFn: func(resource interface{}, rf refapi.Ref) error { return errors.New("boom") }}
	_, err := b.Tasks(refapi.Ref{ID: "vm1"})
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapNetworks_MultiQueueFromFlavorExtra(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{
		Spec: v1beta1.NetworkMapSpec{
			Map: []v1beta1.NetworkPair{{Source: refapi.Ref{ID: "nid1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}}},
		},
	}
	w := mkWorkloadForNet("img", "net1", "nid1", "aa")
	w.Flavor.ExtraSpecs[FlavorVifMultiQueueEnabled] = "true"
	spec := newVMSpec()
	_ = b.mapNetworks(w, spec)
	if spec.Template.Spec.Domain.Devices.NetworkInterfaceMultiQueue == nil || *spec.Template.Spec.Domain.Devices.NetworkInterfaceMultiQueue != true {
		t.Fatalf("expected enabled")
	}
}

func TestBuilder_macConflicts_NoDestinationInventoryErrorPropagated(t *testing.T) {
	b := createBuilder()
	b.Destination.Inventory = &stubInv2{listFn: func(list interface{}, param ...web.Param) error { return errors.New("boom") }}
	w := &model.Workload{}
	w.Addresses = map[string]interface{}{}
	_, err := b.macConflicts(w)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapNetworks_FloatingOnlyResultsInEmpty(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{
		Spec: v1beta1.NetworkMapSpec{
			Map: []v1beta1.NetworkPair{{Source: refapi.Ref{ID: "nid1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}}},
		},
	}
	w := mkWorkloadForNet("img", "net1", "nid1", "aa")
	w.Addresses["net1"] = []interface{}{
		map[string]interface{}{"OS-EXT-IPS-MAC:mac_addr": "aa", "OS-EXT-IPS:type": "floating"},
	}
	spec := newVMSpec()
	_ = b.mapNetworks(w, spec)
	if len(spec.Template.Spec.Networks) != 0 || len(spec.Template.Spec.Domain.Devices.Interfaces) != 0 {
		t.Fatalf("expected empty for floating only")
	}
}

func TestBuilder_mapDisks_UnsupportedFormatStillAppendsVolume(t *testing.T) {
	b := createBuilder()
	b.Source.Inventory = &stubInv2{
		findFn: func(resource interface{}, rf refapi.Ref) error {
			img := resource.(*model.Image)
			img.DiskFormat = "weird"
			img.Properties = map[string]interface{}{forkliftPropertyOriginalImageID: "vmimg"}
			return nil
		},
	}
	w := &model.Workload{}
	w.ImageID = "vmimg"
	w.Image.Properties = map[string]interface{}{}
	spec := newVMSpec()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pvc1",
			Labels:      map[string]string{"imageID": "img1"},
			Annotations: map[string]string{planbase.AnnDiskSource: "disk0"},
		},
	}
	b.mapDisks(w, []*corev1.PersistentVolumeClaim{pvc}, spec)
	if len(spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected volume appended")
	}
}

func TestBuilder_mapNetworks_DefaultVifModelFallback(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{
		Spec: v1beta1.NetworkMapSpec{
			Map: []v1beta1.NetworkPair{{Source: refapi.Ref{ID: "nid1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}}},
		},
	}
	w := mkWorkloadForNet("img", "net1", "nid1", "aa")
	w.Image.Properties[VifModel] = "unknown"
	spec := newVMSpec()
	_ = b.mapNetworks(w, spec)
	if len(spec.Template.Spec.Domain.Devices.Interfaces) != 1 {
		t.Fatalf("expected 1 iface")
	}
	if spec.Template.Spec.Domain.Devices.Interfaces[0].Model != DefaultProperties[VifModel] {
		t.Fatalf("expected default model fallback")
	}
}

func TestBuilder_mapDisks_ImageFindError_ReturnsNoDisks(t *testing.T) {
	b := createBuilder()
	b.Source.Inventory = &stubInv2{findFn: func(resource interface{}, rf refapi.Ref) error { return errors.New("boom") }}
	w := &model.Workload{}
	w.ImageID = "vmimg"
	w.Image.Properties = map[string]interface{}{}
	spec := newVMSpec()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pvc1",
			Labels:      map[string]string{"imageID": "img1"},
			Annotations: map[string]string{planbase.AnnDiskSource: "disk0"},
		},
	}
	b.mapDisks(w, []*corev1.PersistentVolumeClaim{pvc}, spec)
	if len(spec.Template.Spec.Domain.Devices.Disks) != 0 {
		t.Fatalf("expected early return")
	}
}

func TestBuilder_mapDisks_VolumeGetError_ReturnsNoDisks(t *testing.T) {
	b := createBuilder()
	b.Source.Inventory = &stubInv2{
		findFn: func(resource interface{}, rf refapi.Ref) error {
			img := resource.(*model.Image)
			img.DiskFormat = RAW
			img.Properties = map[string]interface{}{forkliftPropertyOriginalVolumeID: "vol-1"}
			return nil
		},
		getFn: func(resource interface{}, id string) error { return errors.New("boom") },
	}
	w := &model.Workload{}
	w.ImageID = "vmimg"
	w.Image.Properties = map[string]interface{}{}
	spec := newVMSpec()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pvc1",
			Labels:      map[string]string{"imageID": "img1"},
			Annotations: map[string]string{planbase.AnnDiskSource: "disk0"},
		},
	}
	b.mapDisks(w, []*corev1.PersistentVolumeClaim{pvc}, spec)
	if len(spec.Template.Spec.Domain.Devices.Disks) != 0 {
		t.Fatalf("expected early return")
	}
}

func TestBuilder_mapNetworks_UsesVMNetworksLookupByName(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{
		Spec: v1beta1.NetworkMapSpec{
			Map: []v1beta1.NetworkPair{{Source: refapi.Ref{ID: "nid1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}}},
		},
	}
	w := mkWorkloadForNet("img", "net1", "nid1", "aa")
	// Addresses name doesn't exist in Networks => vmNetworkID remains "" and lookup fails.
	w.Addresses = map[string]interface{}{"unknown": []interface{}{map[string]interface{}{"OS-EXT-IPS-MAC:mac_addr": "aa"}}}
	spec := newVMSpec()
	if err := b.mapNetworks(w, spec); err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_Tasks_EmptyWhenNoImageAndNoVolumes(t *testing.T) {
	b := createBuilder()
	b.Source.Inventory = &stubInv2{findFn: func(resource interface{}, rf refapi.Ref) error {
		w := resource.(*model.Workload)
		w.ID = "vm1"
		w.ImageID = ""
		w.Volumes = nil
		return nil
	}}
	tasks, err := b.Tasks(refapi.Ref{ID: "vm1"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_macConflicts_DestinationListUsesDetailAll(t *testing.T) {
	b := createBuilder()
	dst := &stubInv2{}
	dst.listFn = func(list interface{}, param ...web.Param) error {
		if len(param) == 0 || param[0].Key != webbase.DetailParam || param[0].Value != "all" {
			t.Fatalf("expected detail=all param")
		}
		return nil
	}
	b.Destination.Inventory = dst
	w := &model.Workload{}
	w.Addresses = map[string]interface{}{}
	_, _ = b.macConflicts(w)
}

func TestBuilder_mapNetworks_IgnoredSkipsEverything(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{
		Spec: v1beta1.NetworkMapSpec{
			Map: []v1beta1.NetworkPair{{Source: refapi.Ref{ID: "nid1"}, Destination: v1beta1.DestinationNetwork{Type: Ignored}}},
		},
	}
	w := mkWorkloadForNet("img", "net1", "nid1", "aa")
	spec := newVMSpec()
	if err := b.mapNetworks(w, spec); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(spec.Template.Spec.Networks) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_mapDisks_IdeBusMapsToSataForDisk(t *testing.T) {
	b := createBuilder()
	b.Source.Inventory = &stubInv2{
		findFn: func(resource interface{}, rf refapi.Ref) error {
			img := resource.(*model.Image)
			img.DiskFormat = RAW
			img.Properties = map[string]interface{}{forkliftPropertyOriginalImageID: "vmimg"}
			return nil
		},
	}
	w := &model.Workload{}
	w.ImageID = "vmimg"
	w.Image.Properties = map[string]interface{}{DiskBus: IdeBus}
	spec := newVMSpec()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pvc1",
			Labels:      map[string]string{"imageID": "img1"},
			Annotations: map[string]string{planbase.AnnDiskSource: "disk0"},
		},
	}
	b.mapDisks(w, []*corev1.PersistentVolumeClaim{pvc}, spec)
	if spec.Template.Spec.Domain.Devices.Disks[0].Disk.Bus != cnv.DiskBus(SataBus) {
		t.Fatalf("expected sata bus mapping")
	}
}

func TestBuilder_mapNetworks_MultusNetworkNamePathJoin(t *testing.T) {
	b := createBuilder()
	b.Context.Map.Network = &v1beta1.NetworkMap{
		Spec: v1beta1.NetworkMapSpec{
			Map: []v1beta1.NetworkPair{{Source: refapi.Ref{ID: "nid1"}, Destination: v1beta1.DestinationNetwork{Type: Multus, Namespace: "ns", Name: "nad"}}},
		},
	}
	w := mkWorkloadForNet("img", "net1", "nid1", "aa")
	spec := newVMSpec()
	if err := b.mapNetworks(w, spec); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if spec.Template.Spec.Networks[0].Multus.NetworkName != "ns/nad" {
		t.Fatalf("unexpected multus name: %q", spec.Template.Spec.Networks[0].Multus.NetworkName)
	}
}

func TestBuilder_Tasks_UsesContextPlanPointer(t *testing.T) {
	b := createBuilder()
	if b.Context == nil || b.Context.Plan == nil || b.Context.Log == nil {
		t.Fatalf("expected builder context initialized")
	}
	_ = plancontext.Context{}
}
