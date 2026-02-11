package openstack

import (
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumetypes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/projects"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/regions"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	model "github.com/kubev2v/forklift/pkg/controller/provider/model/openstack"
	libclient "github.com/kubev2v/forklift/pkg/lib/client/openstack"
)

func TestResourceMappings_RegionProjectFlavorVolumeTypeFault(t *testing.T) {
	region := &Region{Region: libclient.Region{Region: regions.Region{ID: "r1", Description: "d", ParentRegionID: "p"}}}
	rm := &model.Region{}
	region.ApplyTo(rm)
	if !region.equalsTo(rm) {
		t.Fatalf("expected equals after ApplyTo")
	}
	rm.Description = "x"
	if region.equalsTo(rm) {
		t.Fatalf("expected not equals after change")
	}

	project := &Project{Project: libclient.Project{Project: projects.Project{Name: "n", Description: "d", Enabled: true, IsDomain: false, DomainID: "dom", ParentID: "par"}}}
	pm := &model.Project{}
	project.ApplyTo(pm)
	if !project.equalsTo(pm) {
		t.Fatalf("expected project equals after ApplyTo")
	}

	flavor := &Flavor{
		Flavor:     libclient.Flavor{Flavor: flavors.Flavor{Name: "f1", Disk: 10, RAM: 2048, VCPUs: 2, RxTxFactor: 1.5, Swap: 0, IsPublic: true, Ephemeral: 0, Description: "desc"}},
		ExtraSpecs: map[string]string{"k": "v"},
	}
	fm := &model.Flavor{}
	flavor.ApplyTo(fm)
	if !flavor.equalsTo(fm) {
		t.Fatalf("expected flavor equals after ApplyTo")
	}

	vt := &VolumeType{VolumeType: libclient.VolumeType{VolumeType: volumetypes.VolumeType{ID: "id", Name: "n", Description: "d", ExtraSpecs: map[string]string{"x": "y"}, IsPublic: true, QosSpecID: "q", PublicAccess: true}}}
	vtm := &model.VolumeType{}
	vt.ApplyTo(vtm)
	if !vt.equalsTo(vtm) {
		t.Fatalf("expected volumetype equals after ApplyTo")
	}

	fault := &Fault{Fault: libclient.Fault{Fault: servers.Fault{Code: 1, Details: "det", Message: "msg", Created: time.Now()}}}
	fm2 := &model.Fault{}
	fault.ApplyTo(fm2)
	if !fault.equalsTo(fm2) {
		t.Fatalf("expected fault equals after ApplyTo")
	}
}

func TestResourceMappings_ImageSnapshotVolumeNetworkSubnetVM(t *testing.T) {
	now := time.Now()
	img := &Image{Image: libclient.Image{Image: images.Image{
		Name:             "img",
		Status:           images.ImageStatusActive,
		Tags:             []string{"t"},
		ContainerFormat:  "bare",
		DiskFormat:       "qcow2",
		MinDiskGigabytes: 1,
		MinRAMMegabytes:  2,
		Owner:            "o",
		Protected:        false,
		Visibility:       images.ImageVisibilityPublic,
		Hidden:           false,
		Checksum:         "c",
		SizeBytes:        123,
		Metadata:         map[string]string{"m": "v"},
		Properties:       map[string]interface{}{"p": "v"},
		CreatedAt:        now.Add(-time.Hour),
		UpdatedAt:        now,
	}}}
	im := &model.Image{}
	img.ApplyTo(im)
	if !img.updatedAfter(&model.Image{UpdatedAt: now.Add(-time.Minute)}) {
		t.Fatalf("expected updatedAfter true")
	}

	snap := &Snapshot{Snapshot: libclient.Snapshot{Snapshot: snapshots.Snapshot{Name: "s", Description: "d", VolumeID: "v", Status: "available", Size: 10, Metadata: map[string]string{"k": "v"}, CreatedAt: now.Add(-time.Hour), UpdatedAt: now}}}
	sm := &model.Snapshot{}
	snap.ApplyTo(sm)
	if !snap.updatedAfter(&model.Snapshot{UpdatedAt: now.Add(-time.Minute)}) {
		t.Fatalf("expected snapshot updatedAfter true")
	}

	vol := &Volume{Volume: libclient.Volume{Volume: volumes.Volume{
		Name:             "vol",
		Status:           "available",
		Size:             1,
		AvailabilityZone: "az",
		Description:      "d",
		VolumeType:       "vt",
		Metadata:         map[string]string{"k": "v"},
		Attachments:      []volumes.Attachment{{ID: "a1"}},
		CreatedAt:        now.Add(-time.Hour),
		UpdatedAt:        now,
	}}}
	vm := &model.Volume{}
	vol.ApplyTo(vm)
	if len(vm.Attachments) != 1 || vm.Attachments[0].ID != "a1" {
		t.Fatalf("unexpected attachments: %#v", vm.Attachments)
	}
	if !vol.updatedAfter(&model.Volume{UpdatedAt: now.Add(-time.Minute)}) {
		t.Fatalf("expected volume updatedAfter true")
	}

	net := &Network{Network: libclient.Network{Network: networks.Network{
		Name:                  "n",
		Description:           "d",
		AdminStateUp:          true,
		Status:                "ACTIVE",
		Subnets:               []string{"s1"},
		TenantID:              "t",
		ProjectID:             "p",
		Shared:                true,
		AvailabilityZoneHints: []string{"h"},
		Tags:                  []string{"t"},
		RevisionNumber:        1,
		CreatedAt:             now.Add(-time.Hour),
		UpdatedAt:             now,
	}}}
	nm := &model.Network{}
	net.ApplyTo(nm)
	if !net.updatedAfter(&model.Network{UpdatedAt: now.Add(-time.Minute)}) {
		t.Fatalf("expected network updatedAfter true")
	}

	sub := &Subnet{Subnet: libclient.Subnet{Subnet: subnets.Subnet{
		ID: "id", NetworkID: "nid", Name: "sn", Description: "d", IPVersion: 4, CIDR: "10.0.0.0/24", GatewayIP: "10.0.0.1",
		DNSNameservers: []string{"1.1.1.1"}, ServiceTypes: []string{"foo"}, EnableDHCP: true, TenantID: "t", ProjectID: "p",
		AllocationPools: []subnets.AllocationPool{{Start: "10.0.0.10", End: "10.0.0.20"}},
		HostRoutes:      []subnets.HostRoute{{DestinationCIDR: "0.0.0.0/0", NextHop: "10.0.0.1"}},
		Tags:            []string{"tag"}, RevisionNumber: 2,
	}}}
	subm := &model.Subnet{}
	sub.ApplyTo(subm)
	if !sub.equalsTo(subm) {
		t.Fatalf("expected subnet equals after ApplyTo")
	}

	// VM: exercise addImageID/addFlavorID/fault/attached volumes and equality.
	vmr := &VM{VM: libclient.VM{Server: servers.Server{ID: "vm1", Name: "vm", TenantID: "t", UserID: "u", HostID: "h", Status: "ACTIVE", Progress: 50}}}
	vmr.Image = map[string]interface{}{"id": "img1"}
	vmr.Flavor = map[string]interface{}{"id": "fl1"}
	vmr.Fault = servers.Fault{Code: 1, Message: "m", Details: "d", Created: now}
	vmr.AttachedVolumes = []servers.AttachedVolume{{ID: "av1"}}
	vmModel := &model.VM{}
	vmr.ApplyTo(vmModel)
	if vmModel.ImageID != "img1" || vmModel.FlavorID != "fl1" || len(vmModel.AttachedVolumes) != 1 {
		t.Fatalf("unexpected vm model after ApplyTo: %#v", vmModel)
	}
	if !vmr.equalsTo(vmModel) {
		t.Fatalf("expected vm equals after ApplyTo")
	}
	vmModel.AttachedVolumes = append(vmModel.AttachedVolumes, model.AttachedVolume{ID: "av2"})
	if vmr.equalsTo(vmModel) {
		t.Fatalf("expected not equals when attached volumes differ")
	}
}
