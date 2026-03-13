package vsphere

import (
	"errors"
	v1beta1 "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	planbase "github.com/kubev2v/forklift/pkg/controller/plan/adapter/base"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	container "github.com/kubev2v/forklift/pkg/controller/provider/container/vsphere"
	"github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	"github.com/kubev2v/forklift/pkg/controller/provider/web"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25/types"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	cnv "kubevirt.io/api/core/v1"
	cdi "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var builderLogSpectro = logging.WithName("vsphere-builder-test")

const ManualOriginSpectro = string(types.NetIpConfigInfoIpAddressOriginManual)

// vsphereNetworkInventory is a minimal mock inventory that supports
// Builder.findNetworkMapping() by returning a deterministic Network.
type vsphereNetworkInventory struct{}

func (vsphereNetworkInventory) Find(resource interface{}, rf refapi.Ref) error {
	switch r := resource.(type) {
	case *model.Network:
		*r = model.Network{
			Resource: model.Resource{
				ID: rf.ID,
			},
			Key:     "key-" + rf.ID,
			Variant: vsphere.NetDvPortGroup,
		}
		return nil
	default:
		return nil
	}
}

func (vsphereNetworkInventory) Finder() web.Finder                              { return nil }
func (vsphereNetworkInventory) Get(resource interface{}, id string) error       { return nil }
func (vsphereNetworkInventory) Host(ref *refapi.Ref) (interface{}, error)       { return nil, nil }
func (vsphereNetworkInventory) List(list interface{}, param ...web.Param) error { return nil }
func (vsphereNetworkInventory) Network(ref *refapi.Ref) (interface{}, error)    { return nil, nil }
func (vsphereNetworkInventory) Storage(ref *refapi.Ref) (interface{}, error)    { return nil, nil }
func (vsphereNetworkInventory) VM(ref *refapi.Ref) (interface{}, error)         { return nil, nil }
func (vsphereNetworkInventory) Watch(resource interface{}, h web.EventHandler) (*web.Watch, error) {
	return nil, nil
}
func (vsphereNetworkInventory) Workload(ref *refapi.Ref) (interface{}, error) { return nil, nil }

type failingInventory struct{ vsphereNetworkInventory }

func (failingInventory) Find(resource interface{}, rf refapi.Ref) error { return errors.New("boom") }

var _ = Describe("vSphere builder", func() {
	builder := createBuilderSpectro()
	DescribeTable("should", func(vm *model.VM, outputMap string) {
		Expect(builder.mapMacStaticIps(vm)).Should(Equal(outputMap))
	},
		Entry("no static ips", &model.VM{GuestID: "windows9Guest"}, ""),
		Entry("single static ip", &model.VM{
			GuestID: "windows9Guest",
			GuestNetworks: []vsphere.GuestNetwork{
				{
					MAC:          "00:50:56:83:25:47",
					IP:           "172.29.3.193",
					Origin:       ManualOriginSpectro,
					PrefixLength: 16,
					DNS:          []string{"8.8.8.8"},
				}},
			GuestIpStacks: []vsphere.GuestIpStack{
				{
					Gateway: "172.29.3.1",
					Network: "0.0.0.0",
				}},
		}, "00:50:56:83:25:47:ip:172.29.3.193,172.29.3.1,16,8.8.8.8"),
		Entry("multiple static ips", &model.VM{
			GuestID: "windows9Guest",
			GuestNetworks: []vsphere.GuestNetwork{
				{
					MAC:          "00:50:56:83:25:47",
					IP:           "172.29.3.193",
					Origin:       ManualOriginSpectro,
					PrefixLength: 16,
					DNS:          []string{"8.8.8.8"},
				},
				{
					MAC:          "00:50:56:83:25:47",
					IP:           "fe80::5da:b7a5:e0a2:a097",
					Origin:       ManualOriginSpectro,
					PrefixLength: 64,
					DNS:          []string{"fec0:0:0:ffff::1", "fec0:0:0:ffff::2", "fec0:0:0:ffff::3"},
				},
			},
			GuestIpStacks: []vsphere.GuestIpStack{
				{
					Gateway: "172.29.3.1",
					Network: "0.0.0.0",
				},
				{
					Gateway: "fe80::5da:b7a5:e0a2:a095",
					Network: "0.0.0.0",
				},
			},
		}, "00:50:56:83:25:47:ip:172.29.3.193,172.29.3.1,16,8.8.8.8_00:50:56:83:25:47:ip:fe80::5da:b7a5:e0a2:a097,fe80::5da:b7a5:e0a2:a095,64,fec0:0:0:ffff::1,fec0:0:0:ffff::2,fec0:0:0:ffff::3"),
		Entry("non-static ip", &model.VM{GuestID: "windows9Guest", GuestNetworks: []vsphere.GuestNetwork{{MAC: "00:50:56:83:25:47", IP: "172.29.3.193", Origin: string(types.NetIpConfigInfoIpAddressOriginDhcp)}}}, ""),
		Entry("non windows vm", &model.VM{GuestID: "other", GuestNetworks: []vsphere.GuestNetwork{{MAC: "00:50:56:83:25:47", IP: "172.29.3.193", Origin: ManualOriginSpectro}}}, "00:50:56:83:25:47:ip:172.29.3.193,,0"),
		Entry("no OS vm", &model.VM{GuestNetworks: []vsphere.GuestNetwork{{MAC: "00:50:56:83:25:47", IP: "172.29.3.193", Origin: ManualOriginSpectro}}}, "00:50:56:83:25:47:ip:172.29.3.193,,0"),
		Entry("multiple nics static ips", &model.VM{
			GuestID: "windows9Guest",
			GuestNetworks: []vsphere.GuestNetwork{
				{
					MAC:          "00:50:56:83:25:47",
					IP:           "172.29.3.193",
					Origin:       ManualOriginSpectro,
					PrefixLength: 16,
					DNS:          []string{"8.8.8.8"},
				},
				{
					MAC:          "00:50:56:83:25:47",
					IP:           "fe80::5da:b7a5:e0a2:a097",
					Origin:       ManualOriginSpectro,
					PrefixLength: 64,
					DNS:          []string{"fec0:0:0:ffff::1", "fec0:0:0:ffff::2", "fec0:0:0:ffff::3"},
				},
				{
					MAC:          "00:50:56:83:25:48",
					IP:           "172.29.3.192",
					Origin:       ManualOriginSpectro,
					PrefixLength: 24,
					DNS:          []string{"4.4.4.4"},
				},
				{
					MAC:          "00:50:56:83:25:48",
					IP:           "fe80::5da:b7a5:e0a2:a090",
					Origin:       ManualOriginSpectro,
					PrefixLength: 32,
					DNS:          []string{"fec0:0:0:ffff::4", "fec0:0:0:ffff::5", "fec0:0:0:ffff::6"},
				},
			},
			GuestIpStacks: []vsphere.GuestIpStack{
				{
					Gateway: "172.29.3.2",
					Network: "0.0.0.0",
				},
				{
					Gateway: "fe80::5da:b7a5:e0a2:a098",
					Network: "0.0.0.0",
				},
				{
					Gateway: "172.29.3.1",
					Network: "0.0.0.0",
				},
				{
					Gateway: "fe80::5da:b7a5:e0a2:a095",
					Network: "0.0.0.0",
				},
			},
		}, "00:50:56:83:25:47:ip:172.29.3.193,172.29.3.1,16,8.8.8.8_00:50:56:83:25:47:ip:fe80::5da:b7a5:e0a2:a097,fe80::5da:b7a5:e0a2:a095,64,fec0:0:0:ffff::1,fec0:0:0:ffff::2,fec0:0:0:ffff::3_00:50:56:83:25:48:ip:172.29.3.192,172.29.3.1,24,4.4.4.4_00:50:56:83:25:48:ip:fe80::5da:b7a5:e0a2:a090,fe80::5da:b7a5:e0a2:a095,32,fec0:0:0:ffff::4,fec0:0:0:ffff::5,fec0:0:0:ffff::6"),
		Entry("single static ip without DNS", &model.VM{
			GuestID: "windows9Guest",
			GuestNetworks: []vsphere.GuestNetwork{
				{
					MAC:          "00:50:56:83:25:47",
					IP:           "172.29.3.193",
					Origin:       ManualOriginSpectro,
					PrefixLength: 16,
				}},
			GuestIpStacks: []vsphere.GuestIpStack{
				{
					Gateway: "172.29.3.1",
					Network: "0.0.0.0",
				}},
		}, "00:50:56:83:25:47:ip:172.29.3.193,172.29.3.1,16"),
		Entry("gateway from different subnet", &model.VM{
			GuestID: "windows9Guest",
			GuestNetworks: []vsphere.GuestNetwork{
				{
					MAC:          "00:50:56:83:25:47",
					IP:           "172.29.3.193",
					Origin:       ManualOriginSpectro,
					PrefixLength: 24,
					DNS:          []string{"8.8.8.8"},
				}},
			GuestIpStacks: []vsphere.GuestIpStack{
				{
					Gateway: "172.29.4.1",
					Network: "0.0.0.0",
				}},
		}, "00:50:56:83:25:47:ip:172.29.3.193,172.29.4.1,24,8.8.8.8"),
		Entry("multiple gateways with different networks", &model.VM{
			GuestID: "windows9Guest",
			GuestNetworks: []vsphere.GuestNetwork{
				{
					MAC:          "00:50:56:83:25:47",
					IP:           "172.29.3.193",
					Origin:       ManualOriginSpectro,
					PrefixLength: 24,
					DNS:          []string{"8.8.8.8"},
				}},
			GuestIpStacks: []vsphere.GuestIpStack{
				{
					Gateway: "10.10.10.2",
					Network: "10.10.10.1",
				},
				{
					Gateway: "172.29.3.1",
					Network: "0.0.0.0",
				},
				{
					Gateway: "10.10.10.1",
					Network: "10.10.10.0",
				}},
		}, "00:50:56:83:25:47:ip:172.29.3.193,172.29.3.1,24,8.8.8.8"),
	)

	DescribeTable("should", func(disks []vsphere.Disk, output []vsphere.Disk) {
		Expect(builder.sortedDisksAsLibvirt(disks)).Should(Equal(output))
	},
		Entry("sort all disks by buses",
			[]vsphere.Disk{
				{Key: 1, Bus: container.IDE},
				{Key: 1, Bus: container.SATA},
				{Key: 1, Bus: container.SCSI},
				{Key: 2, Bus: container.SCSI},
			},
			[]vsphere.Disk{
				{Key: 1, Bus: container.SCSI},
				{Key: 2, Bus: container.SCSI},
				{Key: 1, Bus: container.SATA},
				{Key: 1, Bus: container.IDE},
			},
		),
		Entry("sort IDE and SATA disks by buses",
			[]vsphere.Disk{
				{Key: 1, Bus: container.IDE},
				{Key: 1, Bus: container.SATA},
			},
			[]vsphere.Disk{
				{Key: 1, Bus: container.SATA},
				{Key: 1, Bus: container.IDE},
			},
		),
		Entry("sort multiple SATA disks by buses",
			[]vsphere.Disk{
				{Key: 3, Bus: container.SATA},
				{Key: 1, Bus: container.SATA},
				{Key: 2, Bus: container.SATA},
			},
			[]vsphere.Disk{
				{Key: 1, Bus: container.SATA},
				{Key: 2, Bus: container.SATA},
				{Key: 3, Bus: container.SATA},
			},
		),
		Entry("sort multiple SATA and multiple SCSI disks by buses",
			[]vsphere.Disk{
				{Key: 3, Bus: container.SATA},
				{Key: 3, Bus: container.SCSI},
				{Key: 2, Bus: container.SCSI},
				{Key: 1, Bus: container.SATA},
				{Key: 2, Bus: container.SATA},
				{Key: 1, Bus: container.SCSI},
			},
			[]vsphere.Disk{
				{Key: 1, Bus: container.SCSI},
				{Key: 2, Bus: container.SCSI},
				{Key: 3, Bus: container.SCSI},
				{Key: 1, Bus: container.SATA},
				{Key: 2, Bus: container.SATA},
				{Key: 3, Bus: container.SATA},
			},
		),
		Entry("sort multiple all disks by buses",
			[]vsphere.Disk{
				{Key: 2, Bus: container.IDE},
				{Key: 3, Bus: container.SATA},
				{Key: 3, Bus: container.SCSI},
				{Key: 2, Bus: container.SCSI},
				{Key: 3, Bus: container.IDE},
				{Key: 1, Bus: container.SATA},
				{Key: 2, Bus: container.SATA},
				{Key: 1, Bus: container.SCSI},
				{Key: 1, Bus: container.IDE},
			},
			[]vsphere.Disk{
				{Key: 1, Bus: container.SCSI},
				{Key: 2, Bus: container.SCSI},
				{Key: 3, Bus: container.SCSI},
				{Key: 1, Bus: container.SATA},
				{Key: 2, Bus: container.SATA},
				{Key: 3, Bus: container.SATA},
				{Key: 1, Bus: container.IDE},
				{Key: 2, Bus: container.IDE},
				{Key: 3, Bus: container.IDE},
			},
		),
	)

	DescribeTable("should sort disks as vmware ordering", func(disks []vsphere.Disk, output []vsphere.Disk) {
		Expect(builder.sortedDisksAsVmware(disks)).Should(Equal(output))
	},
		Entry("sort all disks by buses (SATA, NVME, IDE, SCSI)",
			[]vsphere.Disk{
				{Key: 2, Bus: container.SCSI},
				{Key: 1, Bus: container.SATA},
				{Key: 3, Bus: container.IDE},
				{Key: 1, Bus: container.SCSI},
				{Key: 2, Bus: container.SATA},
				{Key: 1, Bus: container.IDE},
				{Key: 1, Bus: container.NVME},
			},
			[]vsphere.Disk{
				{Key: 1, Bus: container.SATA},
				{Key: 2, Bus: container.SATA},
				{Key: 1, Bus: container.NVME},
				{Key: 1, Bus: container.IDE},
				{Key: 3, Bus: container.IDE},
				{Key: 1, Bus: container.SCSI},
				{Key: 2, Bus: container.SCSI},
			},
		),
	)

	DescribeTable("IsLegacyWindows", func(vm *model.VM, want bool) {
		Expect(IsLegacyWindows(vm)).To(Equal(want))
	},
		Entry("legacy guest id mixed-case identifier does not match (legacyIdentifiers entry is mixed case)", &model.VM{GuestID: "windows7Guest"}, false),
		Entry("legacy guest name matches", &model.VM{GuestName: "Windows XP Professional"}, true),
		Entry("legacy guest name matches server family", &model.VM{GuestName: "Server 2008 R2"}, true),
		Entry("non legacy windows", &model.VM{GuestID: "windows9Guest", GuestName: "Windows 10"}, false),
		Entry("non windows", &model.VM{GuestID: "rhel8_64Guest", GuestName: "Red Hat Enterprise Linux"}, false),
	)

	DescribeTable("isWindows", func(vm *model.VM, want bool) {
		Expect(isWindows(vm)).To(Equal(want))
	},
		Entry("windows guest id", &model.VM{GuestID: "windows9Guest"}, true),
		Entry("windows guest name lower", &model.VM{GuestName: "win10"}, true),
		Entry("windows guest name mixed case does not match", &model.VM{GuestName: "Win10"}, false),
		Entry("non windows", &model.VM{GuestID: "rhel8_64Guest"}, false),
	)

	Describe("getHostAddress", func() {
		It("should return the management network IP when valid", func() {
			host := &model.Host{
				Resource: model.Resource{
					Name: "host.example",
				},
				Network: vsphere.HostNetwork{
					VNICs: []vsphere.VNIC{
						{PortGroup: ManagementNetwork, IpAddress: "not-an-ip"},
						{PortGroup: "Other", IpAddress: "10.0.0.2"},
						{PortGroup: ManagementNetwork, IpAddress: "10.0.0.1"},
					},
				},
			}
			Expect(getHostAddress(host)).To(Equal("10.0.0.1"))
		})

		It("should fall back to host name when management IP missing/invalid", func() {
			host := &model.Host{
				Resource: model.Resource{
					Name: "host.example",
				},
				Network: vsphere.HostNetwork{
					VNICs: []vsphere.VNIC{
						{PortGroup: ManagementNetwork, IpAddress: ""},
						{PortGroup: ManagementNetwork, IpAddress: "bad"},
					},
				},
			}
			Expect(getHostAddress(host)).To(Equal("host.example"))
		})
	})

	Describe("findNetworkMapping", func() {
		It("should match by dvPortGroup key", func() {
			builder.Source.Inventory = vsphereNetworkInventory{}
			netMap := []v1beta1.NetworkPair{
				{Source: refapi.Ref{ID: "n1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}},
			}
			nic := vsphere.NIC{Network: vsphere.Ref{ID: "key-n1"}}
			m := builder.findNetworkMapping(nic, netMap)
			Expect(m).ToNot(BeNil())
			Expect(m.Source.ID).To(Equal("n1"))
		})

		It("should match by network ID", func() {
			builder.Source.Inventory = vsphereNetworkInventory{}
			netMap := []v1beta1.NetworkPair{
				{Source: refapi.Ref{ID: "n1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}},
			}
			nic := vsphere.NIC{Network: vsphere.Ref{ID: "n1"}}
			m := builder.findNetworkMapping(nic, netMap)
			Expect(m).ToNot(BeNil())
		})

		It("should return nil when inventory find fails or doesn't match", func() {
			builder.Source.Inventory = failingInventory{}
			netMap := []v1beta1.NetworkPair{
				{Source: refapi.Ref{ID: "n1"}, Destination: v1beta1.DestinationNetwork{Type: Pod}},
			}
			nic := vsphere.NIC{Network: vsphere.Ref{ID: "key-n1"}}
			Expect(builder.findNetworkMapping(nic, netMap)).To(BeNil())
		})
	})

	Describe("map helpers", func() {
		newVMSpec := func() *cnv.VirtualMachineSpec {
			return &cnv.VirtualMachineSpec{
				Template: &cnv.VirtualMachineInstanceTemplateSpec{
					Spec: cnv.VirtualMachineInstanceSpec{
						Domain: cnv.DomainSpec{},
					},
				},
			}
		}

		It("mapInput should set tablet with virtio bus by default", func() {
			builder.Plan.Spec.SkipGuestConversion = false
			builder.Plan.Spec.UseCompatibilityMode = false

			spec := newVMSpec()
			builder.mapInput(spec)
			Expect(spec.Template.Spec.Domain.Devices.Inputs).To(HaveLen(1))
			Expect(spec.Template.Spec.Domain.Devices.Inputs[0].Bus).To(Equal(cnv.InputBusVirtio))
		})

		It("mapInput should use USB bus under compatibility mode", func() {
			builder.Plan.Spec.SkipGuestConversion = true
			builder.Plan.Spec.UseCompatibilityMode = true

			spec := newVMSpec()
			builder.mapInput(spec)
			Expect(spec.Template.Spec.Domain.Devices.Inputs[0].Bus).To(Equal(cnv.InputBusUSB))
		})

		It("mapClock should set timezone when provided", func() {
			host := &model.Host{Timezone: "UTC"}
			spec := newVMSpec()
			builder.mapClock(host, spec)
			Expect(spec.Template.Spec.Domain.Clock).ToNot(BeNil())
			Expect(spec.Template.Spec.Domain.Clock.ClockOffset.Timezone).ToNot(BeNil())
		})

		It("mapMemory should set guest memory from MB", func() {
			vm := &model.VM{MemoryMB: 512}
			spec := newVMSpec()
			builder.mapMemory(vm, spec)
			Expect(spec.Template.Spec.Domain.Memory).ToNot(BeNil())
			want := resource.MustParse("512Mi")
			Expect(spec.Template.Spec.Domain.Memory.Guest.String()).To(Equal(want.String()))
		})

		It("mapCPU should set sockets/cores and add nested features when enabled", func() {
			vm := &model.VM{CpuCount: 4, CoresPerSocket: 2, NestedHVEnabled: true}
			spec := newVMSpec()
			builder.mapCPU(vm, spec)
			Expect(spec.Template.Spec.Domain.CPU).ToNot(BeNil())
			Expect(spec.Template.Spec.Domain.CPU.Sockets).To(Equal(uint32(2)))
			Expect(spec.Template.Spec.Domain.CPU.Cores).To(Equal(uint32(2)))
			Expect(spec.Template.Spec.Domain.CPU.Features).To(HaveLen(2))
		})

		It("mapFirmware should configure EFI secureboot + SMM", func() {
			vm := &model.VM{UUID: "u", Firmware: Efi, SecureBoot: true}
			spec := newVMSpec()
			builder.mapFirmware(vm, spec)
			Expect(spec.Template.Spec.Domain.Firmware).ToNot(BeNil())
			Expect(spec.Template.Spec.Domain.Firmware.Bootloader).ToNot(BeNil())
			Expect(spec.Template.Spec.Domain.Firmware.Bootloader.EFI).ToNot(BeNil())
			Expect(spec.Template.Spec.Domain.Features).ToNot(BeNil())
			Expect(spec.Template.Spec.Domain.Features.SMM).ToNot(BeNil())
		})

		It("mapFirmware should default to BIOS", func() {
			vm := &model.VM{UUID: "u", Firmware: "other"}
			spec := newVMSpec()
			builder.mapFirmware(vm, spec)
			Expect(spec.Template.Spec.Domain.Firmware.Bootloader.BIOS).ToNot(BeNil())
		})
	})

	Describe("identifier helpers", func() {
		It("ResolveDataVolumeIdentifier/ResolvePersistentVolumeClaimIdentifier should honor warm/cold baseVolume behavior", func() {
			builder.Plan.Spec.Warm = true
			dv := &cdi.DataVolume{}
			dv.Annotations = map[string]string{planbase.AnnDiskSource: "[ds] vm/disk-000015.vmdk"}
			Expect(builder.ResolveDataVolumeIdentifier(dv)).To(Equal("[ds] vm/disk.vmdk"))

			builder.Plan.Spec.Warm = false
			pvc := &core.PersistentVolumeClaim{}
			pvc.Annotations = map[string]string{AnnImportBackingFile: "[ds] vm/disk-000015.vmdk"}
			Expect(builder.ResolvePersistentVolumeClaimIdentifier(pvc)).To(Equal("[ds] vm/disk-000015.vmdk"))
		})
	})
})

//nolint:errcheck
func createBuilderSpectro(objs ...runtime.Object) *Builder {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	v1beta1.SchemeBuilder.AddToScheme(scheme)
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()
	return &Builder{
		Context: &plancontext.Context{
			Destination: plancontext.Destination{
				Client: client,
			},
			Plan: createPlan(),
			Log:  builderLogSpectro,

			// To make sure r.Scheme is not nil
			Client: client,
		},
	}
}
