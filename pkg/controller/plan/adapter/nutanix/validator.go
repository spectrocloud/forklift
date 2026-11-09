package nutanix

import (
	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	planbase "github.com/kubev2v/forklift/pkg/controller/plan/adapter/base"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/nutanix"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Validator struct {
	*plancontext.Context
}

func (r *Validator) WarmMigration() bool {
	return false
}

func (r *Validator) MigrationType() bool {
	switch r.Plan.Spec.Type {
	case api.MigrationCold, "":
		return true
	default:
		return false
	}
}

func (r *Validator) StorageMapped(vmRef ref.Ref) (bool, error) {
	if r.Plan.Referenced.Map.Storage == nil {
		return false, nil
	}
	vm := &model.VM{}
	if err := r.Source.Inventory.Find(vm, vmRef); err != nil {
		return false, liberr.Wrap(err, "vm", vmRef.String())
	}
	return disksMapped(vm, r.Plan.Referenced.Map.Storage), nil
}

// disksMapped reports whether every data disk's storage container has a
// storage map entry. It matches on the map's spec, as DataVolumes does.
func disksMapped(vm *model.VM, storageMap *api.StorageMap) bool {
	mapped := map[string]bool{}
	for _, pair := range storageMap.Spec.Map {
		mapped[pair.Source.ID] = true
	}
	for _, disk := range dataDisks(vm) {
		if !mapped[disk.StorageContainerUUID] {
			return false
		}
	}
	return true
}

func (r *Validator) DirectStorage(_ ref.Ref) (bool, error) {
	return true, nil
}

func (r *Validator) NetworksMapped(vmRef ref.Ref) (bool, error) {
	if r.Plan.Referenced.Map.Network == nil {
		return false, nil
	}
	vm := &model.VM{}
	if err := r.Source.Inventory.Find(vm, vmRef); err != nil {
		return false, liberr.Wrap(err, "vm", vmRef.String())
	}
	return nicsMapped(vm, r.Plan.Referenced.Map.Network), nil
}

// nicsMapped reports whether every NIC's subnet has a network map entry,
// including entries that map it to "ignored". mapNetworks skips NICs
// without one, so an unmapped NIC would otherwise be dropped silently.
func nicsMapped(vm *model.VM, networkMap *api.NetworkMap) bool {
	for _, nic := range vm.NICs {
		if len(planbase.FindAllMappingsForNICRef(ref.Ref{ID: nic.SubnetUUID}, networkMap)) == 0 {
			return false
		}
	}
	return true
}

func (r *Validator) MaintenanceMode(_ ref.Ref) (bool, error) {
	return true, nil
}

func (r *Validator) NICNetworkRefs(_ ref.Ref) ([]ref.Ref, error) {
	// TODO: return one ref per VM NIC subnet
	return nil, nil
}

func (r *Validator) StaticIPs(vmRef ref.Ref) (bool, error) {
	if !r.Plan.Spec.PreserveStaticIPs {
		return true, nil
	}
	vm := &model.VM{}
	if err := r.Source.Inventory.Find(vm, vmRef); err != nil {
		return false, liberr.Wrap(err, "vm", vmRef.String())
	}
	return staticIPsPreserved(vm, r.Context), nil
}

// staticIPsPreserved reports whether the VM's static IPs can be preserved:
// true when a preserved NIC carries them, and true when the VM has no static
// IPs at all (DHCP only), since there is nothing to preserve.
func staticIPsPreserved(vm *model.VM, ctx *plancontext.Context) bool {
	hasStatic := false
	for _, nic := range vm.NICs {
		if len(nic.StaticIPConfigs) == 0 {
			continue
		}
		hasStatic = true
		if nicPreservesStaticIP(nic, ctx) {
			return true
		}
	}
	return !hasStatic
}

func (r *Validator) UdnStaticIPs(_ ref.Ref, _ client.Client) (bool, error) {
	return true, nil
}

func (r *Validator) SharedDisks(_ ref.Ref, _ client.Client) (bool, string, string, error) {
	return true, "", "", nil
}

func (r *Validator) ExcludedDisks(_ ref.Ref) (bool, string, string, error) {
	return true, "", "", nil
}

func (r *Validator) ChangeTrackingEnabled(_ ref.Ref) (bool, error) {
	return true, nil
}

func (r *Validator) HasSnapshot(_ ref.Ref) (bool, string, string, error) {
	return true, "", "", nil
}

func (r *Validator) PowerState(_ ref.Ref) (bool, error) {
	return true, nil
}

func (r *Validator) VMMigrationType(_ ref.Ref) (bool, error) {
	return true, nil
}

func (r *Validator) InvalidDiskSizes(_ ref.Ref) ([]string, error) {
	// TODO: return disk UUIDs with invalid sizes
	return nil, nil
}

func (r *Validator) MacConflicts(_ ref.Ref) ([]planbase.MacConflict, error) {
	// TODO: check source NIC MACs against destination inventory
	return nil, nil
}

func (r *Validator) PVCNameTemplate(_ ref.Ref, _ string) (bool, error) {
	// TODO: validate PVC name template against VM disks
	return true, nil
}

func (r *Validator) GuestToolsInstalled(_ ref.Ref) (bool, error) {
	return true, nil
}

func (r *Validator) ConsolidationNeeded(_ ref.Ref) (bool, error) {
	return false, nil
}
