package ovirt

import (
	"testing"

	model "github.com/kubev2v/forklift/pkg/controller/provider/model/ovirt"
)

func TestBase_ParseHelpers(t *testing.T) {
	b := &Base{}
	if !b.bool("true") {
		t.Fatalf("expected true")
	}
	if b.bool("not-a-bool") {
		t.Fatalf("expected false for invalid bool")
	}
	if b.int16("7") != 7 {
		t.Fatalf("expected 7")
	}
	if b.int16("nope") != 0 {
		t.Fatalf("expected 0 for invalid int16")
	}
	if b.int32("9") != 9 {
		t.Fatalf("expected 9")
	}
	if b.int32("nope") != 0 {
		t.Fatalf("expected 0 for invalid int32")
	}
	if b.int64("11") != 11 {
		t.Fatalf("expected 11")
	}
	if b.int64("nope") != 0 {
		t.Fatalf("expected 0 for invalid int64")
	}
}

func TestApplyTo_DataCenterClusterHost(t *testing.T) {
	dc := &DataCenter{
		Base: Base{
			ID:          "dc1",
			Name:        "dc-name",
			Description: "dc-desc",
		},
	}
	dcM := &model.DataCenter{}
	dc.ApplyTo(dcM)
	if dcM.Name != "dc-name" || dcM.Description != "dc-desc" {
		t.Fatalf("unexpected datacenter model: %#v", dcM)
	}

	cluster := &Cluster{
		Base: Base{
			ID:          "cl1",
			Name:        "cl-name",
			Description: "cl-desc",
		},
		DataCenter:    Ref{ID: "dc1"},
		HaReservation: "true",
		KSM: struct {
			Enabled string `json:"enabled"`
		}{Enabled: "false"},
		BiosType: "q35",
		CPU: struct {
			Type string `json:"type"`
		}{Type: "Intel"},
		Version: struct {
			Minor string `json:"minor"`
			Major string `json:"major"`
		}{Minor: "6", Major: "4"},
	}
	clM := &model.Cluster{}
	cluster.ApplyTo(clM)
	if clM.Name != "cl-name" || clM.DataCenter != "dc1" || clM.HaReservation != true || clM.KsmEnabled != false {
		t.Fatalf("unexpected cluster model: %#v", clM)
	}
	if clM.BiosType != "q35" || clM.CPU.Type != "Intel" || clM.Version.Minor != "6" || clM.Version.Major != "4" {
		t.Fatalf("unexpected cluster model fields: %#v", clM)
	}

	host := &Host{
		Base: Base{
			ID:          "h1",
			Name:        "host1",
			Description: "desc",
		},
		Cluster: Ref{ID: "cl1"},
		Status:  "maintenance",
		OS: struct {
			Type    string `json:"type"`
			Version struct {
				Full string `json:"full_version"`
			} `json:"version"`
		}{
			Type: "RHEL",
			Version: struct {
				Full string `json:"full_version"`
			}{Full: "9.3"},
		},
		CPU: struct {
			Topology struct {
				Sockets string `json:"sockets"`
				Cores   string `json:"cores"`
			} `json:"topology"`
		}{
			Topology: struct {
				Sockets string `json:"sockets"`
				Cores   string `json:"cores"`
			}{Sockets: "2", Cores: "8"},
		},
	}
	host.Networks.Attachment = []struct {
		ID      string `json:"id"`
		Network Ref    `json:"network"`
	}{
		{ID: "na1", Network: Ref{ID: "net1"}},
	}
	host.NICs.List = []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		LinkSpeed string `json:"speed"`
		MTU       string `json:"mtu"`
		VLan      struct {
			ID string `json:"id"`
		} `json:"vlan"`
	}{
		{ID: "nic1", Name: "eth0", LinkSpeed: "1000", MTU: "1500", VLan: struct {
			ID string `json:"id"`
		}{ID: "10"}},
	}

	hM := &model.Host{}
	host.ApplyTo(hM)
	if hM.Name != "host1" || hM.Cluster != "cl1" || !hM.InMaintenance {
		t.Fatalf("unexpected host model: %#v", hM)
	}
	if hM.CpuSockets != 2 || hM.CpuCores != 8 {
		t.Fatalf("unexpected cpu topology: %#v", hM)
	}
	if len(hM.NetworkAttachments) != 1 || hM.NetworkAttachments[0].Network != "net1" {
		t.Fatalf("unexpected network attachments: %#v", hM.NetworkAttachments)
	}
	if len(hM.NICs) != 1 || hM.NICs[0].LinkSpeed != 1000 || hM.NICs[0].MTU != 1500 || hM.NICs[0].VLan != "10" {
		t.Fatalf("unexpected nics: %#v", hM.NICs)
	}
}
