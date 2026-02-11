package main

import (
	"archive/tar"
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
)

func TestSuffixHelpers(t *testing.T) {
	if !hasSuffixIgnoreCase("X.OVA", ".ova") {
		t.Fatalf("expected suffix match")
	}
	if !isOva("a.OvA") {
		t.Fatalf("expected isOva true")
	}
	if !isOvf("a.OVF") {
		t.Fatalf("expected isOvf true")
	}
	if hasSuffixIgnoreCase("a.ovf", ".ova") {
		t.Fatalf("expected suffix mismatch")
	}
}

func TestFindOVAFiles_DepthFiltering(t *testing.T) {
	dir := t.TempDir()

	mustWrite := func(p string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("writefile: %v", err)
		}
	}

	// OVA max depth is 2.
	mustWrite(filepath.Join(dir, "root.ova"))                           // depth 1 => include
	mustWrite(filepath.Join(dir, "d1", "two.ova"))                      // depth 2 => include
	mustWrite(filepath.Join(dir, "d1", "d2", "three.ova"))              // depth 3 => exclude (OVA)
	mustWrite(filepath.Join(dir, "d1", "d2", "three.ovf"))              // depth 3 => include (OVF)
	mustWrite(filepath.Join(dir, "d1", "d2", "d3", "four.ovf"))         // depth 4 => exclude (OVF)
	mustWrite(filepath.Join(dir, "d1", "d2", "d3", "four.not-ovf-ova")) // ignore

	ovas, ovfs, err := findOVAFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ovas) != 2 {
		t.Fatalf("expected 2 ova files, got %d: %#v", len(ovas), ovas)
	}
	if len(ovfs) != 1 {
		t.Fatalf("expected 1 ovf file, got %d: %#v", len(ovfs), ovfs)
	}
}

func TestReadOVF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.ovf")
	if err := os.WriteFile(p, []byte(`<Envelope></Envelope>`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	env, err := readOVF(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env == nil {
		t.Fatalf("expected envelope")
	}
}

func TestReadOVFFromOVA(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.ova")

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: "test.ovf",
		Mode: 0o644,
		Size: int64(len(`<Envelope></Envelope>`)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte(`<Envelope></Envelope>`)); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write ova: %v", err)
	}

	env, err := readOVFFromOVA(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env == nil {
		t.Fatalf("expected envelope")
	}
}

func TestGuessOvaSource(t *testing.T) {
	env := Envelope{}

	// Direct map hits.
	env.Attributes = []xml.Attr{{Value: "http://www.ovirt.org/ovf"}}
	if got := guessOvaSource(env); got != Ovirt {
		t.Fatalf("expected %s, got %s", Ovirt, got)
	}

	// VMware fallback.
	env.Attributes = []xml.Attr{{Value: "http://www.vmware.com/schema/ovf/whatever"}}
	if got := guessOvaSource(env); got != VMware {
		t.Fatalf("expected %s, got %s", VMware, got)
	}

	// Unknown.
	env.Attributes = []xml.Attr{{Value: "http://example.invalid"}}
	if got := guessOvaSource(env); got != Unknown {
		t.Fatalf("expected %s, got %s", Unknown, got)
	}
}

func TestConvertStructsAndUUIDMap(t *testing.T) {
	// Initialize globals used by convert helpers.
	vmIDMap = NewUUIDMap()
	diskIDMap = NewUUIDMap()
	networkIDMap = NewUUIDMap()

	env := Envelope{
		// Trigger VMware fallback.
		Attributes: []xml.Attr{{Value: "http://www.vmware.com/schema/ovf/1"}},
		VirtualSystem: []VirtualSystem{
			{
				ID:   "not-a-uuid",
				Name: "vm1",
				OperatingSystemSection: struct {
					Info        string `xml:"Info"`
					Description string `xml:"Description"`
					OsType      string `xml:"osType,attr"`
				}{OsType: "linux"},
				HardwareSection: VirtualHardwareSection{
					Items: []Item{
						{
							ElementName:     "Network adapter 1",
							InstanceID:      "3",
							ResourceType:    "10",
							VirtualQuantity: 1,
							Address:         "aa:bb",
							Connection:      "net1",
						},
						{
							ElementName:     "CPU",
							InstanceID:      "1",
							ResourceType:    "3",
							Description:     "Number of Virtual CPUs",
							VirtualQuantity: 4,
							AllocationUnits: "count",
							CoresPerSocket:  "2",
							ResourceSubType: "x",
							Parent:          "y",
							HostResource:    "z",
						},
						{
							ElementName:     "Memory",
							InstanceID:      "2",
							ResourceType:    "4",
							Description:     "Memory Size",
							VirtualQuantity: 1024,
							AllocationUnits: "MB",
						},
						{
							ElementName:     "Hard Disk 1",
							InstanceID:      "4",
							ResourceType:    "17",
							Description:     "Hard Disk Device",
							VirtualQuantity: 1,
						},
						{
							ElementName:     "",
							InstanceID:      "5",
							ResourceType:    "0",
							Description:     "Some Device",
							VirtualQuantity: 0,
						},
						{
							ElementName:     "",
							InstanceID:      "6",
							ResourceType:    "0",
							Description:     "",
							VirtualQuantity: 0,
						},
					},
				},
			},
		},
		DiskSection: DiskSection{
			Disks: []Disk{
				{
					Capacity:                10,
					CapacityAllocationUnits: "byte",
					DiskId:                  "d1",
					FileRef:                 "file1",
					Format:                  "raw",
					PopulatedSize:           5,
				},
			},
		},
		NetworkSection: NetworkSection{
			Networks: []Network{{Name: "net1", Description: "n1"}},
		},
		References: References{
			File: []struct {
				Href string `xml:"href,attr"`
			}{{Href: "disk1.vmdk"}},
		},
	}

	applyConfiguration := []VirtualConfig{
		{Key: "firmware", Value: "efi"},
		{Key: "memoryHotAddEnabled", Value: "true"},
		{Key: "cpuHotAddEnabled", Value: "true"},
		{Key: "cpuHotRemoveEnabled", Value: "false"},
	}
	applyExtra := []ExtraVirtualConfig{
		{Key: "cpuHotRemoveEnabled", Value: "true"},
	}
	env.VirtualSystem[0].HardwareSection.Configs = applyConfiguration
	env.VirtualSystem[0].HardwareSection.ExtraConfig = applyExtra

	paths := []string{filepath.Join(t.TempDir(), "a.ovf")}

	vms, err := convertToVmStruct([]Envelope{env}, paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vms) != 1 {
		t.Fatalf("expected 1 VM, got %d", len(vms))
	}
	if vms[0].Name != "vm1" || vms[0].OsType != "linux" {
		t.Fatalf("unexpected vm: %#v", vms[0])
	}
	if vms[0].OvaSource != VMware {
		t.Fatalf("expected OvaSource %s, got %s", VMware, vms[0].OvaSource)
	}
	if vms[0].UUID == "" || isValidUUID(vms[0].UUID) {
		// This path uses vmIDMap hash, so it should not be a valid UUID.
		t.Fatalf("expected hashed UUID, got %q", vms[0].UUID)
	}
	if vms[0].Firmware != "efi" || !vms[0].MemoryHotAddEnabled || !vms[0].CpuHotAddEnabled || !vms[0].CpuHotRemoveEnabled {
		t.Fatalf("unexpected config application: %#v", vms[0])
	}
	if len(vms[0].Disks) != 1 || vms[0].Disks[0].Name != "disk1.vmdk" || vms[0].Disks[0].ID == "" {
		t.Fatalf("unexpected disks: %#v", vms[0].Disks)
	}
	if len(vms[0].Networks) != 1 || vms[0].Networks[0].Name != "net1" || vms[0].Networks[0].ID == "" {
		t.Fatalf("unexpected networks: %#v", vms[0].Networks)
	}

	// Also cover standalone converters.
	disks, err := convertToDiskStruct([]Envelope{env}, paths)
	if err != nil || len(disks) != 1 || disks[0].ID == "" {
		t.Fatalf("unexpected disks: err=%v disks=%#v", err, disks)
	}
	nets, err := convertToNetworkStruct([]Envelope{env})
	if err != nil || len(nets) != 1 || nets[0].ID == "" {
		t.Fatalf("unexpected nets: err=%v nets=%#v", err, nets)
	}

	// UUIDMap caching and truncation.
	um := NewUUIDMap()
	id1 := um.GetUUID(struct{ A string }{A: "x"}, "k")
	id2 := um.GetUUID(struct{ A string }{A: "y"}, "k")
	if id1 != id2 {
		t.Fatalf("expected cached ID for same key")
	}
	if len(id1) != 36 {
		t.Fatalf("expected 36-char id, got %d: %q", len(id1), id1)
	}
}

func TestGetDiskPath(t *testing.T) {
	if got := getDiskPath("/tmp/a.ova"); got != "/tmp/a.ova" {
		t.Fatalf("unexpected path: %s", got)
	}
	if got := getDiskPath("/tmp/a.ovf"); got != "/tmp/" {
		t.Fatalf("unexpected path: %s", got)
	}
	if got := getDiskPath("a.ovf"); got != "a.ovf" {
		t.Fatalf("unexpected path: %s", got)
	}
}

func TestIsValidUUID(t *testing.T) {
	if !isValidUUID("550e8400-e29b-41d4-a716-446655440000") {
		t.Fatalf("expected valid uuid")
	}
	if isValidUUID("not-a-uuid") {
		t.Fatalf("expected invalid uuid")
	}
}
