package vsphere

import (
	"testing"

	model "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	"github.com/vmware/govmomi/vim25/types"
)

func TestBase_Decoded(t *testing.T) {
	b := &Base{}
	if got := b.Decoded("a%2Fb"); got != "a/b" {
		t.Fatalf("expected decoded, got %q", got)
	}
	// invalid escapes should return original string.
	if got := b.Decoded("%zz"); got != "%zz" {
		t.Fatalf("expected original string on decode error, got %q", got)
	}
	// non-string passthrough.
	if got := b.Decoded(types.ManagedObjectReference{}); got != "" {
		t.Fatalf("expected empty string for non-string, got %q", got)
	}
}

func TestBase_Ref_And_RefList(t *testing.T) {
	b := &Base{}
	r := b.Ref(types.ManagedObjectReference{Type: Folder, Value: "f1"})
	if r.ID != "f1" || r.Kind != model.FolderKind {
		t.Fatalf("unexpected ref: %#v", r)
	}
	r2 := b.Ref(types.ManagedObjectReference{Type: Datastore, Value: "ds1"})
	if r2.Kind != model.DsKind {
		t.Fatalf("unexpected kind: %#v", r2)
	}

	list := b.RefList(types.ArrayOfManagedObjectReference{
		ManagedObjectReference: []types.ManagedObjectReference{
			{Type: Network, Value: "n1"},
			{Type: VirtualMachine, Value: "vm1"},
		},
	})
	if len(list) != 2 || list[0].ID != "n1" || list[1].ID != "vm1" {
		t.Fatalf("unexpected list: %#v", list)
	}
}

func TestAdapters_Apply_BaseFieldsAndSpecificFields(t *testing.T) {
	parent := types.ManagedObjectReference{Type: Folder, Value: "parent"}

	t.Run("FolderAdapter", func(t *testing.T) {
		a := &FolderAdapter{}
		a.Apply(types.ObjectUpdate{
			ChangeSet: []types.PropertyChange{
				{Op: Assign, Name: fName, Val: "folder%2Fname"},
				{Op: Assign, Name: fParent, Val: parent},
				{Op: Assign, Name: fChildEntity, Val: types.ArrayOfManagedObjectReference{
					ManagedObjectReference: []types.ManagedObjectReference{
						{Type: Datacenter, Value: "dc1"},
					},
				}},
			},
		})
		m, ok := a.Model().(*model.Folder)
		if !ok {
			t.Fatalf("unexpected model type: %T", a.Model())
		}
		if m.Name != "folder/name" || m.Parent.ID != "parent" || len(m.Children) != 1 || m.Children[0].ID != "dc1" {
			t.Fatalf("unexpected folder model: %#v", m)
		}
	})

	t.Run("DatacenterAdapter", func(t *testing.T) {
		a := &DatacenterAdapter{}
		a.Apply(types.ObjectUpdate{
			ChangeSet: []types.PropertyChange{
				{Op: Assign, Name: fName, Val: "dc"},
				{Op: Assign, Name: fVmFolder, Val: types.ManagedObjectReference{Type: Folder, Value: "vmf"}},
				{Op: Assign, Name: fHostFolder, Val: types.ManagedObjectReference{Type: Folder, Value: "hf"}},
				{Op: Assign, Name: fNetFolder, Val: types.ManagedObjectReference{Type: Folder, Value: "nf"}},
				{Op: Assign, Name: fDsFolder, Val: types.ManagedObjectReference{Type: Folder, Value: "df"}},
			},
		})
		m, ok := a.Model().(*model.Datacenter)
		if !ok {
			t.Fatalf("unexpected model type: %T", a.Model())
		}
		if m.Name != "dc" || m.Vms.ID != "vmf" || m.Clusters.ID != "hf" || m.Networks.ID != "nf" || m.Datastores.ID != "df" {
			t.Fatalf("unexpected datacenter model: %#v", m)
		}
	})
}
