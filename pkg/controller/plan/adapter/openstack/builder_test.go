package openstack

import (
	"context"
	"fmt"
	v1beta1 "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	cnv "kubevirt.io/api/core/v1"
	cdi "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	provweb "github.com/kubev2v/forklift/pkg/controller/provider/web"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/openstack"
)

var _ = Describe("OpenStack builder", func() {
	DescribeTable("should", func(os, version, distro, matchPreferenceName string) {
		Expect(getPreferenceOs(os, version, distro)).Should(Equal(matchPreferenceName))
	},
		Entry("rhel9", RHEL, "9", RHEL, "rhel.9"),
		Entry("centos stream 9", CentOS, "9", CentOS, "centos.stream9"),
		Entry("windows 11", Windows, "11", Windows, "windows.11.virtio"),
		Entry("windows2022", Windows, "2022", Windows, "windows.2k22.virtio"),
		Entry("ubuntu 22", Ubuntu, "22.04.3", Ubuntu, "ubuntu"),
	)
})

func newVMSpec() *cnv.VirtualMachineSpec {
	return &cnv.VirtualMachineSpec{
		Template: &cnv.VirtualMachineInstanceTemplateSpec{
			Spec: cnv.VirtualMachineInstanceSpec{
				Domain: cnv.DomainSpec{
					Devices: cnv.Devices{},
				},
			},
		},
	}
}

// stubInventory is a minimal provider/web client for Builder tests.
// It supports Find() for *model.Workload and returns a static workload.
type stubInventory struct {
	workload *model.Workload
	err      error
}

func (s stubInventory) Finder() provweb.Finder { return nil }
func (s stubInventory) Get(resource interface{}, id string) error {
	return nil
}
func (s stubInventory) List(list interface{}, param ...provweb.Param) error { return nil }
func (s stubInventory) Watch(resource interface{}, h provweb.EventHandler) (*provweb.Watch, error) {
	return nil, nil
}
func (s stubInventory) Find(resource interface{}, rf refapi.Ref) error {
	if s.err != nil {
		return s.err
	}
	switch r := resource.(type) {
	case *model.Workload:
		if s.workload != nil {
			*r = *s.workload
		}
		return nil
	default:
		return nil
	}
}
func (s stubInventory) VM(rf *refapi.Ref) (interface{}, error)       { return struct{}{}, nil }
func (s stubInventory) Workload(rf *refapi.Ref) (interface{}, error) { return struct{}{}, nil }
func (s stubInventory) Network(rf *refapi.Ref) (interface{}, error)  { return struct{}{}, nil }
func (s stubInventory) Storage(rf *refapi.Ref) (interface{}, error)  { return struct{}{}, nil }
func (s stubInventory) Host(rf *refapi.Ref) (interface{}, error)     { return struct{}{}, nil }

var _ = Describe("OpenStack builder mapping helpers", func() {
	It("mapInput should set tablet when pointer model is present", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Image.Properties = map[string]interface{}{PointerModel: "usbtablet"}

		spec := newVMSpec()
		b.mapInput(vm, spec)
		Expect(spec.Template.Spec.Domain.Devices.Inputs).To(HaveLen(1))
		Expect(string(spec.Template.Spec.Domain.Devices.Inputs[0].Bus)).To(Equal(UsbBus))
	})

	It("mapInput should no-op when pointer model is absent", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Image.Properties = map[string]interface{}{}
		spec := newVMSpec()
		b.mapInput(vm, spec)
		Expect(spec.Template.Spec.Domain.Devices.Inputs).To(BeEmpty())
	})

	It("mapVideo should disable autoattach graphics when video model is none", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Image.Properties = map[string]interface{}{VideoModel: VideoNONE}
		spec := newVMSpec()
		b.mapVideo(vm, spec)
		Expect(spec.Template.Spec.Domain.Devices.AutoattachGraphicsDevice).ToNot(BeNil())
		Expect(*spec.Template.Spec.Domain.Devices.AutoattachGraphicsDevice).To(BeFalse())
	})

	It("mapHardwareRng should set rng device only when flavor allows and image has rng model", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Flavor.ExtraSpecs = map[string]string{FlavorHwRng: "true"}
		vm.Image.Properties = map[string]interface{}{HwRngModel: "virtio"}
		spec := newVMSpec()
		b.mapHardwareRng(vm, spec)
		Expect(spec.Template.Spec.Domain.Devices.Rng).ToNot(BeNil())
	})

	It("mapFirmware should prefer image firmware type", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Image.Properties = map[string]interface{}{FirmwareType: EFI}
		spec := newVMSpec()
		b.mapFirmware(vm, spec)
		Expect(spec.Template.Spec.Domain.Firmware).ToNot(BeNil())
		Expect(spec.Template.Spec.Domain.Firmware.Bootloader).ToNot(BeNil())
		Expect(spec.Template.Spec.Domain.Firmware.Bootloader.EFI).ToNot(BeNil())
		Expect(spec.Template.Spec.Domain.Firmware.Bootloader.BIOS).To(BeNil())
	})

	It("mapFirmware should fall back to bootable volume image metadata when image firmware type missing", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Image.Properties = map[string]interface{}{}
		vm.Volumes = []model.Volume{
			{Bootable: "false"},
			{Bootable: "true", VolumeImageMetadata: map[string]string{FirmwareType: EFI}},
		}
		spec := newVMSpec()
		b.mapFirmware(vm, spec)
		Expect(spec.Template.Spec.Domain.Firmware.Bootloader.EFI).ToNot(BeNil())
	})

	It("mapFirmware should default to BIOS when no firmware type provided", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Image.Properties = map[string]interface{}{}
		spec := newVMSpec()
		b.mapFirmware(vm, spec)
		Expect(spec.Template.Spec.Domain.Firmware.Bootloader.BIOS).ToNot(BeNil())
	})

	It("getCpuCount should default based on flavor and allow overrides from image properties and flavor extras", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Flavor.VCPUs = 4
		vm.Flavor.ExtraSpecs = map[string]string{
			FlavorCpuSockets: "2",
		}
		vm.Image.Properties = map[string]interface{}{}

		Expect(b.getCpuCount(vm, CpuSockets)).To(Equal(uint32(2))) // flavor override
		Expect(b.getCpuCount(vm, CpuCores)).To(Equal(uint32(1)))
		Expect(b.getCpuCount(vm, CpuThreads)).To(Equal(uint32(1)))

		vm.Image.Properties[CpuSockets] = "3"
		Expect(b.getCpuCount(vm, CpuSockets)).To(Equal(uint32(3))) // image override wins
		Expect(b.getCpuCount(vm, "unknown")).To(Equal(uint32(0)))
	})

	It("mapResources should no-op when usesInstanceType is true", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Flavor.VCPUs = 2
		vm.Flavor.RAM = 1024
		vm.Image.Properties = map[string]interface{}{}
		spec := newVMSpec()
		b.mapResources(vm, spec, true)
		Expect(spec.Template.Spec.Domain.CPU).To(BeNil())
		Expect(spec.Template.Spec.Domain.Memory).To(BeNil())
	})

	It("mapResources should set cpu policy/threads and memory when not using instance type", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Flavor.VCPUs = 4
		vm.Flavor.RAM = 2048
		vm.Flavor.ExtraSpecs = map[string]string{
			FlavorCpuPolicy:            CpuPolicyDedicated,
			FlavorEmulatorThreadPolicy: CpuThreadPolicyIsolate,
			FlavorCpuCores:             "2",
			FlavorCpuThreads:           "2",
		}
		vm.Image.Properties = map[string]interface{}{}

		spec := newVMSpec()
		b.mapResources(vm, spec, false)
		Expect(spec.Template.Spec.Domain.CPU).ToNot(BeNil())
		Expect(spec.Template.Spec.Domain.CPU.DedicatedCPUPlacement).To(BeTrue())
		Expect(spec.Template.Spec.Domain.CPU.IsolateEmulatorThread).To(BeTrue())
		Expect(spec.Template.Spec.Domain.CPU.Sockets).To(Equal(uint32(4))) // default from VCPUs
		Expect(spec.Template.Spec.Domain.CPU.Cores).To(Equal(uint32(2)))   // flavor override
		Expect(spec.Template.Spec.Domain.CPU.Threads).To(Equal(uint32(2))) // flavor override
		Expect(spec.Template.Spec.Domain.Memory).ToNot(BeNil())
		want := resource.MustParse("2048Mi")
		Expect(spec.Template.Spec.Domain.Memory.Guest.String()).To(Equal(want.String()))
	})

	It("no-op/simple helpers should return expected defaults", func() {
		b := createBuilder()
		Expect(b.SupportsVolumePopulators()).To(BeTrue())
		Expect(b.ResolveDataVolumeIdentifier(&cdi.DataVolume{})).To(Equal(""))
		Expect(b.ResolvePersistentVolumeClaimIdentifier(&corev1.PersistentVolumeClaim{})).To(Equal(""))
	})
})

var _ = Describe("OpenStack builder OS + template label helpers", func() {
	It("getOs should normalize distro families", func() {
		b := createBuilder()
		vm := &model.Workload{}
		vm.Image.Properties = map[string]interface{}{OsDistro: SLED, OsVersion: "15"}
		os, version, distro := b.getOs(vm)
		Expect(os).To(Equal(OpenSUSE))
		Expect(version).To(Equal("15"))
		Expect(distro).To(Equal(SLED))

		vm.Image.Properties = map[string]interface{}{OsDistro: MSDOS, OsVersion: "6.22"}
		os, _, distro = b.getOs(vm)
		Expect(os).To(Equal(Windows))
		Expect(distro).To(Equal(MSDOS))
	})

	It("getTemplateOs/getPreferenceOs should handle CentOS stream and Windows versions", func() {
		Expect(getTemplateOs(CentOS, "9", CentOS)).To(Equal("centos-stream9"))
		Expect(getPreferenceOs(CentOS, "9", CentOS)).To(Equal("centos.stream9"))

		Expect(getTemplateOs(Windows, "2022", Windows)).To(Equal("windows2k22"))
		Expect(getPreferenceOs(Windows, "2022", Windows)).To(Equal("windows.2k22.virtio"))

		// default windows fallback
		Expect(getTemplateOs(Windows, "something-else", Windows)).To(Equal(DefaultWindows))
		Expect(getPreferenceOs(Windows, "something-else", Windows)).To(Equal("windows.10.virtio"))
	})

	It("TemplateLabels should pick flavor/workload based on RAM, pointer model and emulator thread policy", func() {
		b := createBuilder()
		w := &model.Workload{}
		w.Image.Properties = map[string]interface{}{
			OsDistro:     RHEL,
			OsVersion:    "9",
			PointerModel: "usbtablet",
		}
		w.Flavor.RAM = 16384
		w.Flavor.ExtraSpecs = map[string]string{FlavorEmulatorThreadPolicy: CpuThreadPolicyIsolate}

		b.Source.Inventory = stubInventory{workload: w}

		lbls, err := b.TemplateLabels(refapi.Ref{ID: "vm-1"})
		Expect(err).ToNot(HaveOccurred())
		Expect(lbls).To(HaveKeyWithValue(fmt.Sprintf(TemplateOSLabel, "rhel9"), "true"))
		Expect(lbls).To(HaveKeyWithValue(fmt.Sprintf(TemplateFlavorLabel, TemplateFlavorLarge), "true"))
		Expect(lbls).To(HaveKeyWithValue(fmt.Sprintf(TemplateWorkloadLabel, TemplateWorkloadHighPerformance), "true"))
	})

	It("PreferenceName should use inventory workload properties", func() {
		b := createBuilder()
		w := &model.Workload{}
		w.Image.Properties = map[string]interface{}{
			OsDistro:  CentOS,
			OsVersion: "8",
		}
		b.Source.Inventory = stubInventory{workload: w}
		name, err := b.PreferenceName(refapi.Ref{ID: "vm-1"}, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(name).To(Equal("centos.stream8"))
	})
})

var _ = Describe("OpenStack Glance const test", func() {
	It("GlanceSource should be glance, changing it may break the UI", func() {
		Expect(v1beta1.GlanceSource).Should(Equal("glance"))
	})
})

func createBuilder(objs ...runtime.Object) *Builder {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	v1beta1.SchemeBuilder.AddToScheme(scheme)
	_ = cdi.AddToScheme(scheme)

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	// Minimal storage map (can be overridden per-test).
	sm := &v1beta1.StorageMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sm",
			Namespace: "test",
		},
		Spec: v1beta1.StorageMapSpec{
			Map: []v1beta1.StoragePair{},
		},
	}

	vs := v1beta1.VSphere
	return &Builder{
		Context: &plancontext.Context{
			Destination: plancontext.Destination{Client: cl},
			Log:         destinationClientLog,
			Plan:        createPlan(),
			Migration: &v1beta1.Migration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "m",
					Namespace: "test",
					UID:       types.UID("migration1"),
				},
			},
			Source: plancontext.Source{
				Provider: &v1beta1.Provider{Spec: v1beta1.ProviderSpec{Type: &vs, URL: "https://identity.example.invalid"}},
			},
			Client: cl,
			Map: struct {
				Network *v1beta1.NetworkMap
				Storage *v1beta1.StorageMap
			}{
				Storage: sm,
			},
		},
	}
}

var _ = Describe("OpenStack builder storage helpers", func() {
	Describe("getStorageClassName", func() {
		It("should return storage class by volumeType ID", func() {
			b := createBuilder()
			b.Context.Map.Storage.Spec.Map = []v1beta1.StoragePair{
				{Source: refapi.Ref{ID: "vtid"}, Destination: v1beta1.DestinationStorage{StorageClass: "sc1"}},
			}
			w := &model.Workload{}
			w.VolumeTypes = []model.VolumeType{
				{Resource: model.Resource{ID: "vtid", Name: "fast"}},
			}
			sc, err := b.getStorageClassName(w, "fast")
			Expect(err).ToNot(HaveOccurred())
			Expect(sc).To(Equal("sc1"))
		})

		It("should return storage class by volumeType name mapping", func() {
			b := createBuilder()
			b.Context.Map.Storage.Spec.Map = []v1beta1.StoragePair{
				{Source: refapi.Ref{Name: "fast"}, Destination: v1beta1.DestinationStorage{StorageClass: "sc2"}},
			}
			w := &model.Workload{}
			w.VolumeTypes = []model.VolumeType{
				{Resource: model.Resource{ID: "vtid", Name: "fast"}},
			}
			sc, err := b.getStorageClassName(w, "fast")
			Expect(err).ToNot(HaveOccurred())
			Expect(sc).To(Equal("sc2"))
		})

		It("should error when volume type is not found", func() {
			b := createBuilder()
			w := &model.Workload{}
			_, err := b.getStorageClassName(w, "missing")
			Expect(err).To(HaveOccurred())
		})

		It("should error when no storage map exists for volume type", func() {
			b := createBuilder()
			w := &model.Workload{}
			w.VolumeTypes = []model.VolumeType{{Resource: model.Resource{ID: "vtid", Name: "fast"}}}
			_, err := b.getStorageClassName(w, "fast")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("getVolumeAndAccessMode", func() {
		It("should error when StorageProfile is missing", func() {
			b := createBuilder()
			_, _, err := b.getVolumeAndAccessMode("sc-missing")
			Expect(err).To(HaveOccurred())
		})

		It("should default volumeMode to filesystem when omitted", func() {
			b := createBuilder()
			sp := &cdi.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc1"},
				Status: cdi.StorageProfileStatus{
					ClaimPropertySets: []cdi.ClaimPropertySet{
						{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
					},
				},
			}
			Expect(b.Client.Create(context.TODO(), sp)).To(Succeed())
			am, vm, err := b.getVolumeAndAccessMode("sc1")
			Expect(err).ToNot(HaveOccurred())
			Expect(am).To(ContainElement(corev1.ReadWriteOnce))
			Expect(vm).ToNot(BeNil())
			Expect(*vm).To(Equal(corev1.PersistentVolumeFilesystem))
		})

		It("should error when StorageProfile has no access modes", func() {
			b := createBuilder()
			sp := &cdi.StorageProfile{
				ObjectMeta: metav1.ObjectMeta{Name: "sc1"},
				Status:     cdi.StorageProfileStatus{ClaimPropertySets: []cdi.ClaimPropertySet{{}}},
			}
			Expect(b.Client.Create(context.TODO(), sp)).To(Succeed())
			_, _, err := b.getVolumeAndAccessMode("sc1")
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("OpenStack builder populator list helpers", func() {
	It("getVolumePopulatorCR should return NotFound when none exist", func() {
		b := createBuilder()
		_, err := b.getVolumePopulatorCR("img1")
		Expect(k8serr.IsNotFound(err)).To(BeTrue())
	})

	It("getVolumePopulatorCR should error when multiple exist", func() {
		b := createBuilder(
			&v1beta1.OpenstackVolumePopulator{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "test", Labels: map[string]string{"migration": "migration1", "imageID": "img1"}}},
			&v1beta1.OpenstackVolumePopulator{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "test", Labels: map[string]string{"migration": "migration1", "imageID": "img1"}}},
		)
		_, err := b.getVolumePopulatorCR("img1")
		Expect(err).To(HaveOccurred())
	})

	It("getVolumePopulatorCR should return the only match", func() {
		cr := &v1beta1.OpenstackVolumePopulator{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "test", Labels: map[string]string{"migration": "migration1", "imageID": "img1"}}}
		b := createBuilder(cr)
		got, err := b.getVolumePopulatorCR("img1")
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Name).To(Equal("a"))
	})

	It("getVolumePopulatorPVC should return NotFound when none exist", func() {
		b := createBuilder()
		_, err := b.getVolumePopulatorPVC("img1")
		Expect(k8serr.IsNotFound(err)).To(BeTrue())
	})

	It("getVolumePopulatorPVC should error when multiple exist", func() {
		b := createBuilder(
			&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "test", Labels: map[string]string{"migration": "migration1", "imageID": "img1"}}},
			&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "test", Labels: map[string]string{"migration": "migration1", "imageID": "img1"}}},
		)
		_, err := b.getVolumePopulatorPVC("img1")
		Expect(err).To(HaveOccurred())
	})

	It("getVolumePopulatorPVC should return the only match", func() {
		pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "test", Labels: map[string]string{"migration": "migration1", "imageID": "img1"}}}
		b := createBuilder(pvc)
		got, err := b.getVolumePopulatorPVC("img1")
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Name).To(Equal("a"))
	})
})
