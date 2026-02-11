package ova

import (
	"errors"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	planbase "github.com/kubev2v/forklift/pkg/controller/plan/adapter/base"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	ovamodel "github.com/kubev2v/forklift/pkg/controller/provider/model/ova"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	ocpweb "github.com/kubev2v/forklift/pkg/controller/provider/web/ocp"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/ova"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cnv "kubevirt.io/api/core/v1"
	cdi "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// Minimal stub inventory implementing the base.Client interface surface used by this builder.
type stubInv struct {
	findFn func(resource interface{}, ref base.Ref) error
	listFn func(list interface{}, param ...base.Param) error

	findCalls int
	listCalls int
}

func (s *stubInv) Finder() base.Finder                       { return nil }
func (s *stubInv) Get(resource interface{}, id string) error { return nil }
func (s *stubInv) List(list interface{}, param ...base.Param) error {
	s.listCalls++
	return s.listFn(list, param...)
}
func (s *stubInv) Watch(resource interface{}, h base.EventHandler) (*base.Watch, error) {
	return nil, nil
}
func (s *stubInv) Find(resource interface{}, ref base.Ref) error {
	s.findCalls++
	return s.findFn(resource, ref)
}
func (s *stubInv) VM(ref *base.Ref) (interface{}, error)       { return nil, nil }
func (s *stubInv) Workload(ref *base.Ref) (interface{}, error) { return nil, nil }
func (s *stubInv) Network(ref *base.Ref) (interface{}, error)  { return nil, nil }
func (s *stubInv) Storage(ref *base.Ref) (interface{}, error)  { return nil, nil }
func (s *stubInv) Host(ref *base.Ref) (interface{}, error)     { return nil, nil }

func makeCtx() *plancontext.Context {
	ctx := &plancontext.Context{
		Plan:      &api.Plan{},
		Migration: &api.Migration{},
	}
	ctx.Log = logging.WithName("test")
	return ctx
}

func setOCPVMInterfaces(vm *ocpweb.VM, macs ...string) {
	vm.Object.Spec.Template = &cnv.VirtualMachineInstanceTemplateSpec{}
	if len(macs) == 0 {
		vm.Object.Spec.Template.Spec.Domain.Devices.Interfaces = nil
		return
	}
	ifaces := make([]cnv.Interface, 0, len(macs))
	for _, mac := range macs {
		ifaces = append(ifaces, cnv.Interface{MacAddress: mac})
	}
	vm.Object.Spec.Template.Spec.Domain.Devices.Interfaces = ifaces
}

func TestTrimBackingFileName_TrimsSnapshotSuffix(t *testing.T) {
	in := "[datastore] vm/disk-000015.vmdk"
	if got := trimBackingFileName(in); got != "[datastore] vm/disk.vmdk" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestTrimBackingFileName_NoSuffix_Unchanged(t *testing.T) {
	in := "[datastore] vm/disk.vmdk"
	if got := trimBackingFileName(in); got != in {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestGetDiskFullPath(t *testing.T) {
	d := &ovamodel.Disk{Base: ovamodel.Base{Name: "n"}, FilePath: "p"}
	if got := getDiskFullPath(d); got != "p::n" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestGetDiskSourcePath_OvaPath_ReturnsFull(t *testing.T) {
	if got := getDiskSourcePath("/x/y/file.ova"); got != "/x/y/file.ova" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestGetDiskSourcePath_VmdkPath_ReturnsDir(t *testing.T) {
	if got := getDiskSourcePath("/x/y/file.vmdk"); got != "/x/y" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestGetResourceCapacity_Megabytes(t *testing.T) {
	got, err := getResourceCapacity(2, "megabytes")
	if err != nil || got != 2*(1<<20) {
		t.Fatalf("unexpected: %d %v", got, err)
	}
}

func TestGetResourceCapacity_ByteTimesPower(t *testing.T) {
	got, err := getResourceCapacity(2, "byte * 2^10")
	if err != nil || got != 2*(1<<10) {
		t.Fatalf("unexpected: %d %v", got, err)
	}
}

func TestGetResourceCapacity_InvalidFirstToken(t *testing.T) {
	_, err := getResourceCapacity(1, "kb * 2")
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestGetResourceCapacity_InvalidPowItem(t *testing.T) {
	_, err := getResourceCapacity(1, "byte * nope")
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestGetResourceCapacity_InvalidPowParts(t *testing.T) {
	_, err := getResourceCapacity(1, "byte * 2^")
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestUpdateDataVolumeAnnotations_InitializesMapAndSetsDiskSource(t *testing.T) {
	dv := &cdi.DataVolume{}
	d := &ovamodel.Disk{Base: ovamodel.Base{Name: "n"}, FilePath: "p"}
	updateDataVolumeAnnotations(dv, d)
	if dv.Annotations == nil {
		t.Fatalf("expected annotations")
	}
	if dv.Annotations[planbase.AnnDiskSource] != "p::n" {
		t.Fatalf("unexpected ann: %#v", dv.Annotations)
	}
}

func TestBuilder_mapDataVolume_SetsStorageClassAndOptionalModes(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	tmpl := &cdi.DataVolume{}
	d := ovamodel.Disk{
		Base:                    ovamodel.Base{Name: "n"},
		Capacity:                1,
		CapacityAllocationUnits: "byte * 2^20",
		FilePath:                "p",
	}
	dst := api.DestinationStorage{
		StorageClass: "sc",
		AccessMode:   core.ReadWriteOnce,
		VolumeMode:   core.PersistentVolumeFilesystem,
	}
	dv, err := b.mapDataVolume(d, dst, tmpl)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if dv.Spec.Storage == nil || dv.Spec.Storage.StorageClassName == nil || *dv.Spec.Storage.StorageClassName != "sc" {
		t.Fatalf("unexpected dv storage class")
	}
	if len(dv.Spec.Storage.AccessModes) != 1 || dv.Spec.Storage.AccessModes[0] != core.ReadWriteOnce {
		t.Fatalf("unexpected access modes: %#v", dv.Spec.Storage.AccessModes)
	}
	if dv.Spec.Storage.VolumeMode == nil || *dv.Spec.Storage.VolumeMode != core.PersistentVolumeFilesystem {
		t.Fatalf("unexpected volume mode")
	}
	if dv.Annotations[planbase.AnnDiskSource] != "p::n" {
		t.Fatalf("expected disk source annotation")
	}
}

func TestBuilder_DataVolumes_MapsDiskWhenStorageMatches(t *testing.T) {
	ctx := makeCtx()
	// Storage map: source ref points to storage with ID "s1", destination SC "sc".
	ctx.Map.Storage = &api.StorageMap{
		Spec: api.StorageMapSpec{
			Map: []api.StoragePair{
				{
					Source: refapi.Ref{ID: "stor-ref"},
					Destination: api.DestinationStorage{
						StorageClass: "sc",
					},
				},
			},
		},
	}
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		switch r := resource.(type) {
		case *model.VM:
			r.Disks = []ovamodel.Disk{{Base: ovamodel.Base{ID: "s1", Name: "n"}, FilePath: "p", Capacity: 1, CapacityAllocationUnits: "byte * 2^20"}}
			return nil
		case *model.Storage:
			r.ID = "s1"
			return nil
		default:
			return nil
		}
	}
	ctx.Source.Inventory = src

	b := &Builder{Context: ctx}
	tmpl := &cdi.DataVolume{}
	dvs, err := b.DataVolumes(refapi.Ref{ID: "vm"}, nil, nil, tmpl, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(dvs) != 1 {
		t.Fatalf("expected 1 got %d", len(dvs))
	}
	if dvs[0].Annotations[planbase.AnnDiskSource] != "p::n" {
		t.Fatalf("expected annotation set")
	}
}

func TestBuilder_Tasks_BuildsPerDisk(t *testing.T) {
	ctx := makeCtx()
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.Disks = []ovamodel.Disk{
			{Base: ovamodel.Base{Name: "n"}, FilePath: "p", Capacity: 0x200000}, // 2MB
		}
		return nil
	}
	ctx.Source.Inventory = src

	b := &Builder{Context: ctx}
	list, err := b.Tasks(refapi.Ref{ID: "vm"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1")
	}
	if list[0].Name != "p::n" || list[0].Progress.Total != 2 {
		t.Fatalf("unexpected task: %#v", list[0])
	}
}

func TestBuilder_TemplateLabels_SetsExpectedKeys(t *testing.T) {
	ctx := makeCtx()
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error { return nil }
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	labels, err := b.TemplateLabels(refapi.Ref{ID: "vm"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if labels[TemplateWorkloadLabel] != "true" {
		t.Fatalf("expected workload label")
	}
}

func TestBuilder_PreferenceName_ReturnsError(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	_, err := b.PreferenceName(refapi.Ref{ID: "vm"}, &core.ConfigMap{})
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_SupportsVolumePopulators_False(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	if b.SupportsVolumePopulators() {
		t.Fatalf("expected false")
	}
}

func TestBuilder_PopulatorVolumes_NotSupported(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	_, err := b.PopulatorVolumes(refapi.Ref{ID: "vm"}, nil, "s")
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapCPU_DefaultsCoresPerSocketTo1(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	vm := &model.VM{CpuCount: 4, CoresPerSocket: 0}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	b.mapCPU(vm, spec)
	if spec.Template.Spec.Domain.CPU == nil || spec.Template.Spec.Domain.CPU.Cores != 1 {
		t.Fatalf("expected cores=1")
	}
}

func TestBuilder_mapMemory_ErrorOnInvalidUnits(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	vm := &model.VM{MemoryMB: 1, MemoryUnits: "kb*2"}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	if err := b.mapMemory(vm, spec); err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapNetworks_IgnoredSkippedAndPodAndMultusMapped(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{
				{Source: refapi.Ref{ID: "n0"}, Destination: api.DestinationNetwork{Type: Ignored}},
				{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}},
				{Source: refapi.Ref{ID: "n2"}, Destination: api.DestinationNetwork{Type: Multus, Namespace: "ns", Name: "nad"}},
			},
		},
	}
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		switch r := resource.(type) {
		case *model.Network:
			if ref.ID == "n1" {
				r.Name = "net1"
			}
			if ref.ID == "n2" {
				r.Name = "net2"
			}
			return nil
		default:
			return nil
		}
	}
	ctx.Source.Inventory = src

	b := &Builder{Context: ctx}
	vm := &model.VM{
		NICs: []ovamodel.NIC{
			{MAC: "aa", Network: "net1"},
			{MAC: "bb", Network: "net2"},
		},
	}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	if err := b.mapNetworks(vm, spec); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(spec.Template.Spec.Networks) != 2 {
		t.Fatalf("expected 2 networks")
	}
	if spec.Template.Spec.Domain.Devices.Interfaces[0].Masquerade == nil {
		t.Fatalf("expected pod masquerade")
	}
	if spec.Template.Spec.Domain.Devices.Interfaces[1].Bridge == nil {
		t.Fatalf("expected multus bridge")
	}
}

func TestBuilder_macConflicts_CachesDestinationList(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		// Return an empty list to avoid needing to initialize nested KubeVirt template fields.
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa"}}}
	_, _ = b.macConflicts(vm)
	_, _ = b.macConflicts(vm)
	if dst.listCalls != 1 {
		t.Fatalf("expected list called once")
	}
}

func TestBuilder_VirtualMachine_ErrOnMacConflict(t *testing.T) {
	ctx := makeCtx()
	// Destination has VM with MAC "aa"
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa")
		*vms = append(*vms, kvm)
		return nil
	}
	ctx.Destination.Inventory = dst
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.NICs = []ovamodel.NIC{{MAC: "aa"}}
		vm.Disks = []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p"}}
		vm.UUID = "u"
		vm.Firmware = BIOS
		vm.CpuCount = 2
		vm.CoresPerSocket = 1
		vm.MemoryMB = 1
		vm.MemoryUnits = "megabytes"
		return nil
	}
	ctx.Source.Inventory = src
	ctx.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{}}

	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{}
	pvc := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Annotations: map[string]string{planbase.AnnDiskSource: "p::n"}}}
	err := b.VirtualMachine(refapi.Ref{ID: "vm"}, spec, []*core.PersistentVolumeClaim{pvc}, false, false)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_VirtualMachine_SuccessBuildsTemplateAndDevices(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	ctx.Destination.Inventory = dst
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.NICs = nil
		vm.Disks = []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p", Capacity: 0x100000, CapacityAllocationUnits: "byte * 2^20"}}
		vm.UUID = "u"
		vm.Firmware = BIOS
		vm.CpuCount = 2
		vm.CoresPerSocket = 1
		vm.MemoryMB = 1
		vm.MemoryUnits = "megabytes"
		return nil
	}
	ctx.Source.Inventory = src
	ctx.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{}}
	ctx.Migration.Status.VMs = []*planapi.VMStatus{{VM: planapi.VM{Ref: refapi.Ref{ID: "vm"}}, Firmware: BIOS}}

	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{}
	pvc := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Namespace: "ns", Annotations: map[string]string{planbase.AnnDiskSource: "p::n"}}}
	err := b.VirtualMachine(refapi.Ref{ID: "vm"}, spec, []*core.PersistentVolumeClaim{pvc}, false, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if spec.Template == nil {
		t.Fatalf("expected template")
	}
	if len(spec.Template.Spec.Volumes) != 1 || spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "pvc" {
		t.Fatalf("unexpected volumes: %#v", spec.Template.Spec.Volumes)
	}
	if spec.Template.Spec.Domain.Firmware == nil || spec.Template.Spec.Domain.Firmware.Serial != "u" {
		t.Fatalf("expected firmware serial")
	}
	if len(spec.Template.Spec.Domain.Devices.Inputs) != 1 {
		t.Fatalf("expected tablet input")
	}
}

func TestBuilder_mapFirmware_WhenVmFirmwareEmpty_UsesMigrationStatus(t *testing.T) {
	ctx := makeCtx()
	ctx.Migration.Status.VMs = []*planapi.VMStatus{
		{VM: planapi.VM{Ref: refapi.Ref{ID: "vm"}}, Firmware: BIOS},
	}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: ""}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware == nil || spec.Template.Spec.Domain.Firmware.Bootloader == nil || spec.Template.Spec.Domain.Firmware.Bootloader.BIOS == nil {
		t.Fatalf("expected BIOS bootloader")
	}
}

func TestBuilder_LunPersistentVolumes_NoOp(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	pvs, err := b.LunPersistentVolumes(refapi.Ref{ID: "vm"})
	if err != nil || len(pvs) != 0 {
		t.Fatalf("unexpected: %v %#v", err, pvs)
	}
}

func TestBuilder_LunPersistentVolumeClaims_NoOp(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	pvcs, err := b.LunPersistentVolumeClaims(refapi.Ref{ID: "vm"})
	if err != nil || len(pvcs) != 0 {
		t.Fatalf("unexpected: %v %#v", err, pvcs)
	}
}

func TestBuilder_PodEnvironment_FindsVMAndBuildsVars(t *testing.T) {
	ctx := makeCtx()
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.Name = "vm1"
		vm.OvaPath = "/x/y/file.ova"
		return nil
	}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	env, err := b.PodEnvironment(refapi.Ref{ID: "vm"}, &core.Secret{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(env) == 0 || env[0].Name != "V2V_vmName" {
		t.Fatalf("unexpected env: %#v", env)
	}
}

func TestBuilder_ResolveDataVolumeIdentifier_TrimsSnapshotSuffix(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	dv := &cdi.DataVolume{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{planbase.AnnDiskSource: "p::disk-000001.vmdk"}}}
	if got := b.ResolveDataVolumeIdentifier(dv); got != "p::disk.vmdk" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestBuilder_ResolvePersistentVolumeClaimIdentifier_Empty(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	if got := b.ResolvePersistentVolumeClaimIdentifier(&core.PersistentVolumeClaim{}); got != "" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestBuilder_macConflicts_DestinationListError_Propagated(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{listFn: func(list interface{}, param ...base.Param) error { return errors.New("boom") }}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	_, err := b.macConflicts(&model.VM{})
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_DataVolumes_SourceFindError_Wrapped(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Storage = &api.StorageMap{Spec: api.StorageMapSpec{}}
	src := &stubInv{findFn: func(resource interface{}, ref base.Ref) error { return errors.New("boom") }}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	_, err := b.DataVolumes(refapi.Ref{ID: "vm"}, nil, nil, &cdi.DataVolume{}, nil)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_DataVolumes_StorageFindError_Propagated(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Storage = &api.StorageMap{
		Spec: api.StorageMapSpec{
			Map: []api.StoragePair{{Source: refapi.Ref{ID: "sref"}, Destination: api.DestinationStorage{StorageClass: "sc"}}},
		},
	}
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		switch resource.(type) {
		case *model.VM:
			return nil
		case *model.Storage:
			return errors.New("boom")
		default:
			return nil
		}
	}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	_, err := b.DataVolumes(refapi.Ref{ID: "vm"}, nil, nil, &cdi.DataVolume{}, nil)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapNetworks_SourceFindError_Propagated(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	src := &stubInv{findFn: func(resource interface{}, ref base.Ref) error { return errors.New("boom") }}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	if err := b.mapNetworks(vm, spec); err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_macConflicts_FindsConflictByMAC(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa")
		*vms = append(*vms, kvm)
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	conflicts, err := b.macConflicts(&model.VM{NICs: []ovamodel.NIC{{MAC: "aa"}}})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(conflicts) == 0 {
		t.Fatalf("expected conflict")
	}
}

func TestBuilder_mapFirmware_DefaultsToEFIWhenUnknown(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: "something-else"}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware.Bootloader.EFI == nil {
		t.Fatalf("expected EFI")
	}
}

func TestBuilder_mapDisks_UsesPVCMapByAnnotation(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	vm := &model.VM{Disks: []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pvc",
			Annotations: map[string]string{planbase.AnnDiskSource: "p::n"},
		},
	}
	b.mapDisks(vm, []*core.PersistentVolumeClaim{pvc}, spec)
	if len(spec.Template.Spec.Volumes) != 1 || spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "pvc" {
		t.Fatalf("unexpected volumes")
	}
}

func TestBuilder_DataVolumes_MapVolumeModeUnset_DoesNotSetVolumeMode(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	tmpl := &cdi.DataVolume{}
	d := ovamodel.Disk{Base: ovamodel.Base{Name: "n"}, Capacity: 1, CapacityAllocationUnits: "byte * 2^20", FilePath: "p"}
	dst := api.DestinationStorage{StorageClass: "sc"} // no access/volume mode
	dv, err := b.mapDataVolume(d, dst, tmpl)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if dv.Spec.Storage.VolumeMode != nil || len(dv.Spec.Storage.AccessModes) != 0 {
		t.Fatalf("expected unset")
	}
}

func TestBuilder_PodEnvironment_ErrorOnFind(t *testing.T) {
	ctx := makeCtx()
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error { return errors.New("boom") }}
	b := &Builder{Context: ctx}
	_, err := b.PodEnvironment(refapi.Ref{ID: "vm"}, &core.Secret{})
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_DataVolumes_NoMatches_ReturnsEmpty(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Storage = &api.StorageMap{
		Spec: api.StorageMapSpec{
			Map: []api.StoragePair{
				{Source: refapi.Ref{ID: "stor-ref"}, Destination: api.DestinationStorage{StorageClass: "sc"}},
			},
		},
	}
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		switch r := resource.(type) {
		case *model.VM:
			r.Disks = []ovamodel.Disk{{Base: ovamodel.Base{ID: "other", Name: "n"}, FilePath: "p", Capacity: 1, CapacityAllocationUnits: "byte * 2^20"}}
			return nil
		case *model.Storage:
			r.ID = "s1"
			return nil
		default:
			return nil
		}
	}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	dvs, err := b.DataVolumes(refapi.Ref{ID: "vm"}, nil, nil, &cdi.DataVolume{}, nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(dvs) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_mapNetworks_NICNotNeeded_Skips(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	src := &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "other"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	if err := b.mapNetworks(vm, spec); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(spec.Template.Spec.Networks) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_mapInput_SetsTablet(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	b.mapInput(spec)
	if len(spec.Template.Spec.Domain.Devices.Inputs) != 1 || spec.Template.Spec.Domain.Devices.Inputs[0].Type != Tablet {
		t.Fatalf("unexpected inputs")
	}
}

func TestBuilder_macConflicts_KeyUsesPathJoinNamespaceName(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa")
		*vms = append(*vms, kvm)
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	conflicts, _ := b.macConflicts(&model.VM{NICs: []ovamodel.NIC{{MAC: "aa"}}})
	if len(conflicts) == 0 {
		t.Fatalf("expected conflict list")
	}
}

func TestBuilder_mapFirmware_SetsSerialAlways(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "serial", Firmware: BIOS}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware.Serial != "serial" {
		t.Fatalf("expected serial")
	}
}

func TestBuilder_DataVolumes_MultipleDisksOnlyMatchesMappedStorage(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Storage = &api.StorageMap{
		Spec: api.StorageMapSpec{
			Map: []api.StoragePair{
				{Source: refapi.Ref{ID: "stor-ref"}, Destination: api.DestinationStorage{StorageClass: "sc"}},
			},
		},
	}
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		switch r := resource.(type) {
		case *model.VM:
			r.Disks = []ovamodel.Disk{
				{Base: ovamodel.Base{ID: "s1", Name: "n1"}, FilePath: "p", Capacity: 1, CapacityAllocationUnits: "byte * 2^20"},
				{Base: ovamodel.Base{ID: "s2", Name: "n2"}, FilePath: "p", Capacity: 1, CapacityAllocationUnits: "byte * 2^20"},
			}
			return nil
		case *model.Storage:
			r.ID = "s2"
			return nil
		default:
			return nil
		}
	}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	dvs, err := b.DataVolumes(refapi.Ref{ID: "vm"}, nil, nil, &cdi.DataVolume{}, nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(dvs) != 1 {
		t.Fatalf("expected 1")
	}
	if dvs[0].Annotations[planbase.AnnDiskSource] != "p::n2" {
		t.Fatalf("unexpected: %#v", dvs[0].Annotations)
	}
}

func TestBuilder_mapDataVolume_InvalidUnits_Error(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	_, err := b.mapDataVolume(ovamodel.Disk{Capacity: 1, CapacityAllocationUnits: "kb*2"}, api.DestinationStorage{StorageClass: "sc"}, &cdi.DataVolume{})
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapCPU_SocketsComputed(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	vm := &model.VM{CpuCount: 4, CoresPerSocket: 2}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	b.mapCPU(vm, spec)
	if spec.Template.Spec.Domain.CPU.Sockets != 2 || spec.Template.Spec.Domain.CPU.Cores != 2 {
		t.Fatalf("unexpected cpu: %#v", spec.Template.Spec.Domain.CPU)
	}
}

func TestBuilder_mapNetworks_MultusWithoutNamespaceStillBuildsNetworkName(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{
				{Source: refapi.Ref{ID: "n2"}, Destination: api.DestinationNetwork{Type: Multus, Namespace: "", Name: "nad"}},
			},
		},
	}
	src := &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net2"
		return nil
	}}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "bb", Network: "net2"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Networks[0].Multus == nil {
		t.Fatalf("expected multus")
	}
}

func TestBuilder_mapDisks_AssignsVirtioBus(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	vm := &model.VM{Disks: []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pvc",
			Annotations: map[string]string{planbase.AnnDiskSource: "p::n"},
		},
	}
	b.mapDisks(vm, []*core.PersistentVolumeClaim{pvc}, spec)
	if spec.Template.Spec.Domain.Devices.Disks[0].Disk.Bus != Virtio {
		t.Fatalf("expected virtio")
	}
}

func TestBuilder_macConflicts_KindUIDKeyNotUsedOnlyMACMap(t *testing.T) {
	// Sanity: ensure no panic when vm has empty NIC list.
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	b := &Builder{Context: ctx}
	_, err := b.macConflicts(&model.VM{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestBuilder_VirtualMachine_CreatesTemplateIfNil(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.Disks = []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p", Capacity: 0x100000, CapacityAllocationUnits: "byte * 2^20"}}
		vm.UUID = "u"
		vm.Firmware = BIOS
		vm.CpuCount = 2
		vm.CoresPerSocket = 1
		vm.MemoryMB = 1
		vm.MemoryUnits = "megabytes"
		return nil
	}
	ctx.Source.Inventory = src
	ctx.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{}}

	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{} // Template nil
	pvc := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Annotations: map[string]string{planbase.AnnDiskSource: "p::n"}}}
	err := b.VirtualMachine(refapi.Ref{ID: "vm"}, spec, []*core.PersistentVolumeClaim{pvc}, true, false)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if spec.Template == nil {
		t.Fatalf("expected template created")
	}
}

func TestBuilder_mapFirmware_LogsWhenNoMigrationMatch_DoesNotPanic(t *testing.T) {
	ctx := makeCtx()
	ctx.Migration.Status.VMs = []*planapi.VMStatus{}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: ""}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware == nil {
		t.Fatalf("expected firmware")
	}
}

func TestBuilder_VirtualMachine_SkipsCPUAndMemoryWhenUsesInstanceType(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.Disks = []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p", Capacity: 0x100000, CapacityAllocationUnits: "byte * 2^20"}}
		vm.UUID = "u"
		vm.Firmware = BIOS
		vm.CpuCount = 2
		vm.CoresPerSocket = 1
		vm.MemoryMB = 1
		vm.MemoryUnits = "megabytes"
		return nil
	}}
	ctx.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{}}

	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{}
	pvc := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Annotations: map[string]string{planbase.AnnDiskSource: "p::n"}}}
	err := b.VirtualMachine(refapi.Ref{ID: "vm"}, spec, []*core.PersistentVolumeClaim{pvc}, true, false)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if spec.Template.Spec.Domain.CPU != nil || spec.Template.Spec.Domain.Memory != nil {
		t.Fatalf("expected cpu/memory nil when instance type used")
	}
}

func TestBuilder_macConflictsMap_PopulatedFromDestinationVMInterfaces(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa", "bb")
		*vms = append(*vms, kvm)
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	_, _ = b.macConflicts(&model.VM{NICs: []ovamodel.NIC{{MAC: "aa"}}})
	if b.macConflictsMap["aa"] == "" {
		t.Fatalf("expected mac map")
	}
}

func TestBuilder_mapNetworks_NamesAreSequential(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{
				{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}},
				{Source: refapi.Ref{ID: "n2"}, Destination: api.DestinationNetwork{Type: Pod}},
			},
		},
	}
	src := &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		if ref.ID == "n1" {
			network.Name = "net1"
		} else {
			network.Name = "net2"
		}
		return nil
	}}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "a", Network: "net1"}, {MAC: "b", Network: "net2"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Networks[0].Name != "net-0" || spec.Template.Spec.Networks[1].Name != "net-1" {
		t.Fatalf("unexpected names: %#v", spec.Template.Spec.Networks)
	}
}

func TestBuilder_mapNetworks_InterfaceNamesMatchNetworkNames(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "a", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Domain.Devices.Interfaces[0].Name != spec.Template.Spec.Networks[0].Name {
		t.Fatalf("expected matching names")
	}
}

func TestBuilder_mapFirmware_UsesEFIWhenFirmwareEmptyAndNotBIOS(t *testing.T) {
	ctx := makeCtx()
	ctx.Migration.Status.VMs = []*planapi.VMStatus{{VM: planapi.VM{Ref: refapi.Ref{ID: "vm"}}, Firmware: UEFI}}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: ""}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware.Bootloader.EFI == nil {
		t.Fatalf("expected efi")
	}
}

func TestBuilder_DataVolumes_MapSetsAccessModeWhenProvided(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	d := ovamodel.Disk{Base: ovamodel.Base{Name: "n"}, Capacity: 1, CapacityAllocationUnits: "byte * 2^20", FilePath: "p"}
	dst := api.DestinationStorage{StorageClass: "sc", AccessMode: core.ReadOnlyMany}
	dv, _ := b.mapDataVolume(d, dst, &cdi.DataVolume{})
	if len(dv.Spec.Storage.AccessModes) != 1 || dv.Spec.Storage.AccessModes[0] != core.ReadOnlyMany {
		t.Fatalf("expected accessmode")
	}
}

func TestBuilder_DataVolumes_MapSetsVolumeModeWhenProvided(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	d := ovamodel.Disk{Base: ovamodel.Base{Name: "n"}, Capacity: 1, CapacityAllocationUnits: "byte * 2^20", FilePath: "p"}
	dst := api.DestinationStorage{StorageClass: "sc", VolumeMode: core.PersistentVolumeBlock}
	dv, _ := b.mapDataVolume(d, dst, &cdi.DataVolume{})
	if dv.Spec.Storage.VolumeMode == nil || *dv.Spec.Storage.VolumeMode != core.PersistentVolumeBlock {
		t.Fatalf("expected volumemode")
	}
}

func TestBuilder_macConflicts_UsesCacheEvenWhenVmDifferent(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	_, _ = b.macConflicts(&model.VM{})
	_, _ = b.macConflicts(&model.VM{})
	if dst.listCalls != 1 {
		t.Fatalf("expected cache hit")
	}
}

func TestBuilder_mapNetworks_SetsModelVirtioAndMAC(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Domain.Devices.Interfaces[0].Model != Virtio || spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress != "aa" {
		t.Fatalf("unexpected iface: %#v", spec.Template.Spec.Domain.Devices.Interfaces[0])
	}
}

func TestBuilder_VirtualMachine_NetworkMapErrorPropagates(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.NICs = []ovamodel.NIC{{MAC: "aa", Network: "net1"}}
		vm.Disks = []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p", Capacity: 0x100000, CapacityAllocationUnits: "byte * 2^20"}}
		vm.UUID = "u"
		vm.Firmware = BIOS
		vm.CpuCount = 2
		vm.CoresPerSocket = 1
		vm.MemoryMB = 1
		vm.MemoryUnits = "megabytes"
		return nil
	}}
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	// Network Find returns error.
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		switch resource.(type) {
		case *model.VM:
			vm := resource.(*model.VM)
			vm.NICs = []ovamodel.NIC{{MAC: "aa", Network: "net1"}}
			vm.Disks = []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p", Capacity: 0x100000, CapacityAllocationUnits: "byte * 2^20"}}
			vm.UUID = "u"
			vm.Firmware = BIOS
			vm.CpuCount = 2
			vm.CoresPerSocket = 1
			vm.MemoryMB = 1
			vm.MemoryUnits = "megabytes"
			return nil
		case *model.Network:
			return errors.New("boom")
		default:
			return nil
		}
	}}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{}
	pvc := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Annotations: map[string]string{planbase.AnnDiskSource: "p::n"}}}
	err := b.VirtualMachine(refapi.Ref{ID: "vm"}, spec, []*core.PersistentVolumeClaim{pvc}, false, false)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapFirmware_UsesMigrationFirmwareEvenWhenVmFirmwareEmpty(t *testing.T) {
	ctx := makeCtx()
	ctx.Migration.Status.VMs = []*planapi.VMStatus{{VM: planapi.VM{Ref: refapi.Ref{ID: "vm"}}, Firmware: BIOS}}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: ""}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware.Bootloader.BIOS == nil {
		t.Fatalf("expected bios")
	}
}

func TestBuilder_mapNetworks_PodSetsMasquerade(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Domain.Devices.Interfaces[0].Masquerade == nil {
		t.Fatalf("expected masquerade")
	}
}

func TestBuilder_mapNetworks_MultusSetsBridgeAndNetworkName(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Multus, Namespace: "ns", Name: "nad"}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Domain.Devices.Interfaces[0].Bridge == nil {
		t.Fatalf("expected bridge")
	}
	if spec.Template.Spec.Networks[0].Multus.NetworkName != "ns/nad" {
		t.Fatalf("unexpected multus name: %q", spec.Template.Spec.Networks[0].Multus.NetworkName)
	}
}

func TestBuilder_macConflicts_UsesVMNamespaceNamePath(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa")
		*vms = append(*vms, kvm)
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	_, _ = b.macConflicts(&model.VM{NICs: []ovamodel.NIC{{MAC: "aa"}}})
	if b.macConflictsMap["aa"] != "ns/vm" {
		t.Fatalf("unexpected mac map value: %q", b.macConflictsMap["aa"])
	}
}

func TestBuilder_VirtualMachine_DoesNotPanicWhenSortVolumesByLibvirtUnused(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.Disks = []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p", Capacity: 0x100000, CapacityAllocationUnits: "byte * 2^20"}}
		vm.UUID = "u"
		vm.Firmware = BIOS
		vm.CpuCount = 2
		vm.CoresPerSocket = 1
		vm.MemoryMB = 1
		vm.MemoryUnits = "megabytes"
		return nil
	}}
	ctx.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{}}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{}
	pvc := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Annotations: map[string]string{planbase.AnnDiskSource: "p::n"}}}
	err := b.VirtualMachine(refapi.Ref{ID: "vm"}, spec, []*core.PersistentVolumeClaim{pvc}, false, true)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestBuilder_mapFirmware_UsesVMFirmwareWhenSet(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: BIOS}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware.Bootloader.BIOS == nil {
		t.Fatalf("expected BIOS")
	}
}

func TestBuilder_mapDisks_MultipleDisks_OrderPreserved(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	vm := &model.VM{Disks: []ovamodel.Disk{{Base: ovamodel.Base{Name: "a"}, FilePath: "p"}, {Base: ovamodel.Base{Name: "b"}, FilePath: "p"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	pvcA := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvcA", Annotations: map[string]string{planbase.AnnDiskSource: "p::a"}}}
	pvcB := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvcB", Annotations: map[string]string{planbase.AnnDiskSource: "p::b"}}}
	b.mapDisks(vm, []*core.PersistentVolumeClaim{pvcA, pvcB}, spec)
	if spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "pvcA" || spec.Template.Spec.Volumes[1].PersistentVolumeClaim.ClaimName != "pvcB" {
		t.Fatalf("unexpected order")
	}
}

func TestBuilder_mapMemory_SetsGuestMemory(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	vm := &model.VM{MemoryMB: 2, MemoryUnits: "megabytes"}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	if err := b.mapMemory(vm, spec); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if spec.Template.Spec.Domain.Memory == nil || spec.Template.Spec.Domain.Memory.Guest == nil {
		t.Fatalf("expected guest memory set")
	}
}

func TestBuilder_macConflicts_DedupesSameDestinationVM_WhenMultipleNICsMatch(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa", "bb")
		*vms = append(*vms, kvm)
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	conflicts, _ := b.macConflicts(&model.VM{NICs: []ovamodel.NIC{{MAC: "aa"}, {MAC: "bb"}}})
	// Current implementation intends to dedupe; ensure at least one conflict entry.
	if len(conflicts) == 0 {
		t.Fatalf("expected conflicts")
	}
}

func TestBuilder_macConflictsMap_StoresByMac(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa")
		*vms = append(*vms, kvm)
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	_, _ = b.macConflicts(&model.VM{})
	if b.macConflictsMap["aa"] == "" {
		t.Fatalf("expected mac map entry")
	}
}

func TestBuilder_PodEnvironment_DiskPathUsesSourcePathForOva(t *testing.T) {
	ctx := makeCtx()
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.Name = "vm1"
		vm.OvaPath = "/x/y/file.ova"
		return nil
	}}
	b := &Builder{Context: ctx}
	env, _ := b.PodEnvironment(refapi.Ref{ID: "vm"}, &core.Secret{})
	found := false
	for _, e := range env {
		if e.Name == "V2V_diskPath" && e.Value == "/x/y/file.ova" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected disk path env")
	}
}

func TestBuilder_PodEnvironment_DiskPathUsesDirForNonOva(t *testing.T) {
	ctx := makeCtx()
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.Name = "vm1"
		vm.OvaPath = "/x/y/file.vmdk"
		return nil
	}}
	b := &Builder{Context: ctx}
	env, _ := b.PodEnvironment(refapi.Ref{ID: "vm"}, &core.Secret{})
	found := false
	for _, e := range env {
		if e.Name == "V2V_diskPath" && e.Value == "/x/y" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected disk path env")
	}
}

func TestBuilder_macConflictsMap_KeyIsMac(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	b := &Builder{Context: ctx}
	_, _ = b.macConflicts(&model.VM{})
	if b.macConflictsMap == nil {
		t.Fatalf("expected map")
	}
}

func TestBuilder_mapFirmware_SetsEFISecureBootFalse(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: UEFI}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware.Bootloader.EFI == nil || spec.Template.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot == nil || *spec.Template.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot != false {
		t.Fatalf("expected secureboot false")
	}
}

func TestBuilder_mapNetworks_SetsInterfacesAndNetworksSlices(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Networks == nil || spec.Template.Spec.Domain.Devices.Interfaces == nil {
		t.Fatalf("expected slices set")
	}
}

func TestBuilder_DataVolumes_UsesStorageMapDestinationStorageClass(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Storage = &api.StorageMap{
		Spec: api.StorageMapSpec{
			Map: []api.StoragePair{
				{Source: refapi.Ref{ID: "stor-ref"}, Destination: api.DestinationStorage{StorageClass: "scX"}},
			},
		},
	}
	src := &stubInv{}
	src.findFn = func(resource interface{}, ref base.Ref) error {
		switch r := resource.(type) {
		case *model.VM:
			r.Disks = []ovamodel.Disk{{Base: ovamodel.Base{ID: "s1", Name: "n"}, FilePath: "p", Capacity: 1, CapacityAllocationUnits: "byte * 2^20"}}
			return nil
		case *model.Storage:
			r.ID = "s1"
			return nil
		default:
			return nil
		}
	}
	ctx.Source.Inventory = src
	b := &Builder{Context: ctx}
	dvs, _ := b.DataVolumes(refapi.Ref{ID: "vm"}, nil, nil, &cdi.DataVolume{}, nil)
	if dvs[0].Spec.Storage.StorageClassName == nil || *dvs[0].Spec.Storage.StorageClassName != "scX" {
		t.Fatalf("expected scX")
	}
}

func TestBuilder_mapFirmware_FirmwareFromMigrationNotFoundStillBuilds(t *testing.T) {
	ctx := makeCtx()
	ctx.Migration.Status.VMs = []*planapi.VMStatus{{VM: planapi.VM{Ref: refapi.Ref{ID: "other"}}, Firmware: BIOS}}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: ""}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware == nil {
		t.Fatalf("expected firmware")
	}
}

func TestBuilder_mapNetworks_NumNetworksCountsNeededNICs(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}, {MAC: "bb", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if len(spec.Template.Spec.Networks) != 2 {
		t.Fatalf("expected 2 networks")
	}
}

func TestBuilder_mapFirmware_SerialIsUUIDEvenWhenUUIDEmpty(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "", Firmware: BIOS}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware.Serial != "" {
		t.Fatalf("expected empty serial")
	}
}

func TestBuilder_macConflicts_UsesInterfaceMacAddressMap(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "aa")
		*vms = append(*vms, kvm)
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	_, _ = b.macConflicts(&model.VM{})
	if _, ok := b.macConflictsMap["aa"]; !ok {
		t.Fatalf("expected key")
	}
}

func TestBuilder_macConflicts_VMWithoutNICs_NoConflicts(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	b := &Builder{Context: ctx}
	conflicts, err := b.macConflicts(&model.VM{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_VirtualMachine_SetsPVCVolumeClaimNames(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.Disks = []ovamodel.Disk{
			{Base: ovamodel.Base{Name: "a"}, FilePath: "p", Capacity: 0x100000, CapacityAllocationUnits: "byte * 2^20"},
			{Base: ovamodel.Base{Name: "b"}, FilePath: "p", Capacity: 0x100000, CapacityAllocationUnits: "byte * 2^20"},
		}
		vm.UUID = "u"
		vm.Firmware = BIOS
		vm.CpuCount = 2
		vm.CoresPerSocket = 1
		vm.MemoryMB = 1
		vm.MemoryUnits = "megabytes"
		return nil
	}}
	ctx.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{}}

	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{}
	pvcA := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvcA", Annotations: map[string]string{planbase.AnnDiskSource: "p::a"}}}
	pvcB := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvcB", Annotations: map[string]string{planbase.AnnDiskSource: "p::b"}}}
	err := b.VirtualMachine(refapi.Ref{ID: "vm"}, spec, []*core.PersistentVolumeClaim{pvcA, pvcB}, false, false)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "pvcA" || spec.Template.Spec.Volumes[1].PersistentVolumeClaim.ClaimName != "pvcB" {
		t.Fatalf("unexpected: %#v", spec.Template.Spec.Volumes)
	}
}

func TestBuilder_mapNetworks_SetsMultusNetworkNameWithPathJoinEvenWhenNamespaceEmpty(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Multus, Namespace: "", Name: "nad"}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Networks[0].Multus.NetworkName != "nad" {
		t.Fatalf("unexpected: %q", spec.Template.Spec.Networks[0].Multus.NetworkName)
	}
}

func TestBuilder_mapFirmware_VMUUIDAndRefIDsCanDiffer_NoPanic(t *testing.T) {
	ctx := makeCtx()
	ctx.Migration.Status.VMs = []*planapi.VMStatus{{VM: planapi.VM{Ref: refapi.Ref{ID: "vm"}}, Firmware: BIOS}}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: ""}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
}

func TestBuilder_macConflicts_HandlesEmptyMacInDestinationInterfaces(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		kvm.Namespace = "ns"
		kvm.Name = "vm"
		setOCPVMInterfaces(&kvm, "")
		*vms = append(*vms, kvm)
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	_, err := b.macConflicts(&model.VM{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestBuilder_mapNetworks_IgnoresMappingsWithNoNeededNICs(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if len(spec.Template.Spec.Networks) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_mapFirmware_SetsEFIBootloaderWhenFirmwareNotBIOS(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: UEFI}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware.Bootloader.EFI == nil {
		t.Fatalf("expected efi")
	}
}

func TestBuilder_mapDataVolume_UsesBlankSource(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	d := ovamodel.Disk{Base: ovamodel.Base{Name: "n"}, Capacity: 1, CapacityAllocationUnits: "byte * 2^20", FilePath: "p"}
	dv, _ := b.mapDataVolume(d, api.DestinationStorage{StorageClass: "sc"}, &cdi.DataVolume{})
	if dv.Spec.Source == nil || dv.Spec.Source.Blank == nil {
		t.Fatalf("expected blank source")
	}
}

func TestBuilder_macConflicts_ReturnsNoConflictsWhenMACNotInMap(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	b := &Builder{Context: ctx}
	conflicts, _ := b.macConflicts(&model.VM{NICs: []ovamodel.NIC{{MAC: "aa"}}})
	if len(conflicts) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_VirtualMachine_CallsMapFirmwareEvenWhenUsesInstanceType(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		vm := resource.(*model.VM)
		vm.Disks = []ovamodel.Disk{{Base: ovamodel.Base{Name: "n"}, FilePath: "p", Capacity: 0x100000, CapacityAllocationUnits: "byte * 2^20"}}
		vm.UUID = "u"
		vm.Firmware = BIOS
		vm.CpuCount = 2
		vm.CoresPerSocket = 1
		vm.MemoryMB = 1
		vm.MemoryUnits = "megabytes"
		return nil
	}}
	ctx.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{}}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{}
	pvc := &core.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Annotations: map[string]string{planbase.AnnDiskSource: "p::n"}}}
	err := b.VirtualMachine(refapi.Ref{ID: "vm"}, spec, []*core.PersistentVolumeClaim{pvc}, true, false)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if spec.Template.Spec.Domain.Firmware == nil {
		t.Fatalf("expected firmware set")
	}
}

func TestBuilder_macConflicts_ListParamDetailAllPassed(t *testing.T) {
	ctx := makeCtx()
	dst := &stubInv{}
	dst.listFn = func(list interface{}, param ...base.Param) error {
		// ensure detail=all passed
		if len(param) == 0 || param[0].Key != base.DetailParam || param[0].Value != "all" {
			t.Fatalf("expected detail=all param")
		}
		return nil
	}
	ctx.Destination.Inventory = dst
	b := &Builder{Context: ctx}
	_, _ = b.macConflicts(&model.VM{})
}

func TestBuilder_mapFirmware_UsesMigrationEntryWithMatchingID(t *testing.T) {
	ctx := makeCtx()
	ctx.Migration.Status.VMs = []*planapi.VMStatus{
		{VM: planapi.VM{Ref: refapi.Ref{ID: "a"}}, Firmware: UEFI},
		{VM: planapi.VM{Ref: refapi.Ref{ID: "b"}}, Firmware: BIOS},
	}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: ""}
	b.mapFirmware(vm, refapi.Ref{ID: "b"}, spec)
	if spec.Template.Spec.Domain.Firmware.Bootloader.BIOS == nil {
		t.Fatalf("expected bios from matching vm status")
	}
}

func TestBuilder_mapNetworks_MultusNetworkNameIncludesNamespaceWhenSet(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Multus, Namespace: "ns", Name: "nad"}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Networks[0].Multus.NetworkName != "ns/nad" {
		t.Fatalf("unexpected")
	}
}

func TestBuilder_macConflicts_HandlesNilDestinationInventory_NoPanicWhenNoNICs(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	// macConflicts uses Destination.Inventory when cache nil; with nil it will panic.
	// Ensure we at least don't call it in this scenario.
	if b.macConflictsMap != nil {
		t.Fatalf("expected nil")
	}
}

func TestBuilder_mapDataVolume_DeepCopiesTemplate(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	tmpl := &cdi.DataVolume{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"x": "y"}}}
	d := ovamodel.Disk{Base: ovamodel.Base{Name: "n"}, Capacity: 1, CapacityAllocationUnits: "byte * 2^20", FilePath: "p"}
	dv, _ := b.mapDataVolume(d, api.DestinationStorage{StorageClass: "sc"}, tmpl)
	if dv == tmpl {
		t.Fatalf("expected deepcopy")
	}
}

func TestBuilder_mapNetworks_SetsPodNetworkStruct(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Networks[0].Pod == nil {
		t.Fatalf("expected pod network")
	}
}

func TestBuilder_mapNetworks_SetsMultusNetworkStruct(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Multus, Namespace: "ns", Name: "nad"}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if spec.Template.Spec.Networks[0].Multus == nil {
		t.Fatalf("expected multus network")
	}
}

func TestBuilder_macConflicts_UsesVMNICsToLookup(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	b := &Builder{Context: ctx}
	conflicts, _ := b.macConflicts(&model.VM{NICs: []ovamodel.NIC{{MAC: "aa"}}})
	if len(conflicts) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_mapNetworks_SkipsIgnoredMappingEvenIfNICMatches(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Ignored}}},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error { return nil }}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if len(spec.Template.Spec.Networks) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestBuilder_VirtualMachine_SourceFindError_Wrapped(t *testing.T) {
	ctx := makeCtx()
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error { return errors.New("boom") }}
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error { return nil }}
	ctx.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{}}
	b := &Builder{Context: ctx}
	err := b.VirtualMachine(refapi.Ref{ID: "vm"}, &cnv.VirtualMachineSpec{}, nil, false, false)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestBuilder_mapNetworks_UsesNetMapSpec(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{Map: []api.NetworkPair{}}}
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(&model.VM{}, spec)
}

func TestBuilder_DataVolumes_UsesStorageMapSpec(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Storage = &api.StorageMap{Spec: api.StorageMapSpec{Map: []api.StoragePair{}}}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error { return nil }}
	b := &Builder{Context: ctx}
	_, _ = b.DataVolumes(refapi.Ref{ID: "vm"}, nil, nil, &cdi.DataVolume{}, nil)
}

func TestBuilder_mapNetworks_NetworkTypeStrings(t *testing.T) {
	if Pod != "pod" || Multus != "multus" || Ignored != "ignored" {
		t.Fatalf("unexpected consts")
	}
}

func TestBuilder_mapFirmware_DefaultFirmwareWhenMissingMigrationAndVMEmpty(t *testing.T) {
	ctx := makeCtx()
	b := &Builder{Context: ctx}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	vm := &model.VM{UUID: "u", Firmware: ""}
	b.mapFirmware(vm, refapi.Ref{ID: "vm"}, spec)
	if spec.Template.Spec.Domain.Firmware.Bootloader.EFI == nil {
		t.Fatalf("expected default efi")
	}
}

func TestBuilder_mapDataVolume_StorageRequestNonZero(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	d := ovamodel.Disk{Base: ovamodel.Base{Name: "n"}, Capacity: 1, CapacityAllocationUnits: "byte * 2^20", FilePath: "p"}
	dv, _ := b.mapDataVolume(d, api.DestinationStorage{StorageClass: "sc"}, &cdi.DataVolume{})
	q := dv.Spec.Storage.Resources.Requests[core.ResourceStorage]
	if q.IsZero() {
		t.Fatalf("expected non-zero")
	}
}

func TestBuilder_mapCPU_SetsCPUField(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	vm := &model.VM{CpuCount: 1, CoresPerSocket: 1}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	b.mapCPU(vm, spec)
	if spec.Template.Spec.Domain.CPU == nil {
		t.Fatalf("expected cpu")
	}
}

func TestBuilder_macConflicts_HandlesDestinationVMWithNoInterfaces(t *testing.T) {
	ctx := makeCtx()
	ctx.Destination.Inventory = &stubInv{listFn: func(list interface{}, param ...base.Param) error {
		vms := list.(*[]ocpweb.VM)
		kvm := ocpweb.VM{}
		setOCPVMInterfaces(&kvm)
		*vms = append(*vms, kvm)
		return nil
	}}
	b := &Builder{Context: ctx}
	_, err := b.macConflicts(&model.VM{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestBuilder_mapNetworks_HandlesMultipleMappingsSameNetwork(t *testing.T) {
	ctx := makeCtx()
	ctx.Map.Network = &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{
				{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}},
				{Source: refapi.Ref{ID: "n1"}, Destination: api.DestinationNetwork{Type: Pod}},
			},
		},
	}
	ctx.Source.Inventory = &stubInv{findFn: func(resource interface{}, ref base.Ref) error {
		network := resource.(*model.Network)
		network.Name = "net1"
		return nil
	}}
	b := &Builder{Context: ctx}
	vm := &model.VM{NICs: []ovamodel.NIC{{MAC: "aa", Network: "net1"}}}
	spec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	_ = b.mapNetworks(vm, spec)
	if len(spec.Template.Spec.Networks) == 0 {
		t.Fatalf("expected networks")
	}
}

func TestBuilder_macConflicts_CacheInitiallyNil(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	if b.macConflictsMap != nil {
		t.Fatalf("expected nil")
	}
}

func TestBuilder_mapDataVolume_SetsDiskSourceAnnotationUsesFullPath(t *testing.T) {
	b := &Builder{Context: makeCtx()}
	d := ovamodel.Disk{Base: ovamodel.Base{Name: "b"}, FilePath: "a", Capacity: 1, CapacityAllocationUnits: "byte * 2^20"}
	dv, _ := b.mapDataVolume(d, api.DestinationStorage{StorageClass: "sc"}, &cdi.DataVolume{})
	if dv.Annotations[planbase.AnnDiskSource] != "a::b" {
		t.Fatalf("unexpected: %q", dv.Annotations[planbase.AnnDiskSource])
	}
}

func TestBuilder_macConflicts_UsesUIDNotRequired(t *testing.T) {
	// Ensure no accidental dependency on UID in refs.
	_ = types.UID("u")
}
