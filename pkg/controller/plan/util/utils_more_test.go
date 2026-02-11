package util

import (
	"net/url"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/openstack"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/ovirt"
	"github.com/kubev2v/forklift/pkg/settings"
	core "k8s.io/api/core/v1"
)

func TestRoundUp_MultipleZero_ReturnsRequested(t *testing.T) {
	if got := RoundUp(123, 0); got != 123 {
		t.Fatalf("expected 123 got %d", got)
	}
}

func TestRoundUp_AlreadyMultiple_ReturnsSame(t *testing.T) {
	if got := RoundUp(1024, 512); got != 1024 {
		t.Fatalf("expected 1024 got %d", got)
	}
}

func TestRoundUp_RoundsUp(t *testing.T) {
	if got := RoundUp(513, 512); got != 1024 {
		t.Fatalf("expected 1024 got %d", got)
	}
}

func TestRoundUp_ZeroRequested_ReturnsZero(t *testing.T) {
	if got := RoundUp(0, 512); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
}

func TestRoundUp_RoundsUpLarge(t *testing.T) {
	if got := RoundUp(1001, 1000); got != 2000 {
		t.Fatalf("expected 2000 got %d", got)
	}
}

func TestCalculateSpaceWithOverhead_Filesystem_UsesAlignmentAndPercent(t *testing.T) {
	oldFS := settings.Settings.FileSystemOverhead
	t.Cleanup(func() { settings.Settings.FileSystemOverhead = oldFS })
	settings.Settings.FileSystemOverhead = 10

	mode := core.PersistentVolumeFilesystem
	// requested 1 => aligned to 1MiB, then / (1-0.1) = 1.111..MiB => ceil => 2MiB? Actually ceil on bytes.
	got := CalculateSpaceWithOverhead(1, &mode)
	if got <= DefaultAlignBlockSize {
		t.Fatalf("expected > %d got %d", DefaultAlignBlockSize, got)
	}
}

func TestCalculateSpaceWithOverhead_Block_AddsFixedOverhead(t *testing.T) {
	oldBlock := settings.Settings.BlockOverhead
	t.Cleanup(func() { settings.Settings.BlockOverhead = oldBlock })
	settings.Settings.BlockOverhead = 123

	mode := core.PersistentVolumeBlock
	got := CalculateSpaceWithOverhead(1, &mode)
	if got != DefaultAlignBlockSize+123 {
		t.Fatalf("expected %d got %d", DefaultAlignBlockSize+123, got)
	}
}

// ---- Consolidated from populator_more_test.go ----

func TestOpenstackVolumePopulator_BuildsExpectedObject(t *testing.T) {
	u, _ := url.Parse("https://identity.example.invalid/v3")
	img := &openstack.Image{Resource: openstack.Resource{ID: "img-1", Name: "imgName"}}
	tn := &core.ObjectReference{Name: "net"}
	obj := OpenstackVolumePopulator(img, u, tn, "ns", "sec", "vm1", "mig1")
	if obj.Name != "imgName" || obj.Namespace != "ns" {
		t.Fatalf("unexpected meta: %#v", obj.ObjectMeta)
	}
	if obj.Labels["vmID"] != "vm1" || obj.Labels["migration"] != "mig1" {
		t.Fatalf("unexpected labels: %#v", obj.Labels)
	}
	if obj.Spec.IdentityURL != u.String() || obj.Spec.SecretName != "sec" || obj.Spec.ImageID != "img-1" {
		t.Fatalf("unexpected spec: %#v", obj.Spec)
	}
	if obj.Spec.TransferNetwork == nil || obj.Spec.TransferNetwork.Name != "net" {
		t.Fatalf("unexpected transfer network: %#v", obj.Spec.TransferNetwork)
	}
	if obj.Status.Progress != "0" {
		t.Fatalf("expected progress 0 got %q", obj.Status.Progress)
	}
}

func TestOpenstackVolumePopulator_AllowsNilTransferNetwork(t *testing.T) {
	u, _ := url.Parse("https://identity.example.invalid/v3")
	img := &openstack.Image{Resource: openstack.Resource{ID: "img-1", Name: "imgName"}}
	obj := OpenstackVolumePopulator(img, u, nil, "ns", "sec", "vm1", "mig1")
	if obj.Spec.TransferNetwork != nil {
		t.Fatalf("expected nil transfer network")
	}
}

func TestOvirtVolumePopulator_BuildsExpectedObject(t *testing.T) {
	u, _ := url.Parse("https://engine.example.invalid/ovirt-engine/api")
	tn := &core.ObjectReference{Name: "net"}
	da := ovirt.XDiskAttachment{
		DiskAttachment: ovirt.DiskAttachment{ID: "da-1", Disk: "disk-1"},
		Disk: ovirt.XDisk{
			Disk: ovirt.Disk{
				Resource: ovirt.Resource{ID: "disk-1"},
			},
		},
	}
	obj := OvirtVolumePopulator(da, u, tn, "ns", "sec", "vm1", "mig1")
	if obj.Name != "da-1" || obj.Namespace != "ns" {
		t.Fatalf("unexpected meta: %#v", obj.ObjectMeta)
	}
	if obj.Labels["vmID"] != "vm1" || obj.Labels["migration"] != "mig1" {
		t.Fatalf("unexpected labels: %#v", obj.Labels)
	}
	if obj.Spec.EngineURL != ("https://"+u.Host) || obj.Spec.EngineSecretName != "sec" || obj.Spec.DiskID != "disk-1" {
		t.Fatalf("unexpected spec: %#v", obj.Spec)
	}
	if obj.Spec.TransferNetwork == nil || obj.Spec.TransferNetwork.Name != "net" {
		t.Fatalf("unexpected transfer network: %#v", obj.Spec.TransferNetwork)
	}
	if obj.Status.Progress != "0" {
		t.Fatalf("expected progress 0 got %q", obj.Status.Progress)
	}
}

func TestOvirtVolumePopulator_AllowsNilTransferNetwork(t *testing.T) {
	u, _ := url.Parse("https://engine.example.invalid/ovirt-engine/api")
	da := ovirt.XDiskAttachment{DiskAttachment: ovirt.DiskAttachment{ID: "da-1", Disk: "disk-1"}}
	obj := OvirtVolumePopulator(da, u, nil, "ns", "sec", "vm1", "mig1")
	if obj.Spec.TransferNetwork != nil {
		t.Fatalf("expected nil transfer network")
	}
}

func TestPlanapiVolumePopulatorTypeValuesCompile(t *testing.T) {
	// Just ensure the api types referenced by populators are present.
	_ = &api.OpenstackVolumePopulator{}
	_ = &api.OvirtVolumePopulator{}
}

func TestGetDeviceNumber_InvalidPrefix_ReturnsZero(t *testing.T) {
	if got := GetDeviceNumber("/dev/vda"); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
}

func TestGetDeviceNumber_TooShort_ReturnsZero(t *testing.T) {
	if got := GetDeviceNumber("/dev/sd"); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
}

func TestGetDeviceNumber_Sda_Returns1(t *testing.T) {
	if got := GetDeviceNumber("/dev/sda"); got != 1 {
		t.Fatalf("expected 1 got %d", got)
	}
}

func TestGetDeviceNumber_Sdb_Returns2(t *testing.T) {
	if got := GetDeviceNumber("/dev/sdb"); got != 2 {
		t.Fatalf("expected 2 got %d", got)
	}
}

func TestGetDeviceNumber_Sdz_Returns26(t *testing.T) {
	if got := GetDeviceNumber("/dev/sdz"); got != 26 {
		t.Fatalf("expected 26 got %d", got)
	}
}

func TestGetDeviceNumber_Sda1_Returns1(t *testing.T) {
	if got := GetDeviceNumber("/dev/sda1"); got != 1 {
		t.Fatalf("expected 1 got %d", got)
	}
}

func TestGetDeviceNumber_UppercaseLetter_CurrentBehavior(t *testing.T) {
	// Current implementation treats any letter as a disk suffix and does byte arithmetic
	// against 'a', so uppercase yields a large number.
	if got := GetDeviceNumber("/dev/sdA"); got != 225 {
		t.Fatalf("expected 225 got %d", got)
	}
}

func TestGetDeviceNumber_DigitOnlySuffix_Returns0(t *testing.T) {
	if got := GetDeviceNumber("/dev/sd1"); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
}

func TestGetDeviceNumber_FirstLetterWins(t *testing.T) {
	if got := GetDeviceNumber("/dev/sdab"); got != 1 {
		t.Fatalf("expected 1 got %d", got)
	}
}

func TestGetDeviceNumber_EmptyString_Returns0(t *testing.T) {
	if got := GetDeviceNumber(""); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
}

func TestCalculateSpaceWithOverhead_FilesystemOverhead0_EqualsAligned(t *testing.T) {
	oldFS := settings.Settings.FileSystemOverhead
	t.Cleanup(func() { settings.Settings.FileSystemOverhead = oldFS })
	settings.Settings.FileSystemOverhead = 0

	mode := core.PersistentVolumeFilesystem
	got := CalculateSpaceWithOverhead(1, &mode)
	if got != DefaultAlignBlockSize {
		t.Fatalf("expected %d got %d", DefaultAlignBlockSize, got)
	}
}

func TestCalculateSpaceWithOverhead_Block_AlreadyAlignedStillAddsOverhead(t *testing.T) {
	oldBlock := settings.Settings.BlockOverhead
	t.Cleanup(func() { settings.Settings.BlockOverhead = oldBlock })
	settings.Settings.BlockOverhead = 10

	mode := core.PersistentVolumeBlock
	got := CalculateSpaceWithOverhead(DefaultAlignBlockSize, &mode)
	if got != DefaultAlignBlockSize+10 {
		t.Fatalf("expected %d got %d", DefaultAlignBlockSize+10, got)
	}
}

func TestGetBootDiskNumber_DeviceZero_ReturnsZero(t *testing.T) {
	if got := GetBootDiskNumber("/dev/vda"); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
}

func TestGetBootDiskNumber_Sda_Returns0(t *testing.T) {
	if got := GetBootDiskNumber("/dev/sda"); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
}

func TestGetBootDiskNumber_Sdb_Returns1(t *testing.T) {
	if got := GetBootDiskNumber("/dev/sdb"); got != 1 {
		t.Fatalf("expected 1 got %d", got)
	}
}
