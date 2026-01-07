package ocp

import (
	"context"
	"fmt"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	"github.com/kubev2v/forklift/pkg/lib/logging"

	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	"github.com/kubev2v/forklift/pkg/controller/provider/web"
	ocpclient "github.com/kubev2v/forklift/pkg/lib/client/openshift"
	core "k8s.io/api/core/v1"
	cnv "kubevirt.io/api/core/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const VM_NOT_FOUND = "VM not found."

// Validator
type Validator struct {
	plan         *api.Plan
	inventory    web.Client
	sourceClient k8sclient.Client
	log          logging.LevelLogger
}

// MaintenanceMode implements base.Validator
func (r *Validator) MaintenanceMode(vmRef ref.Ref) (bool, error) {
	return true, nil
}

// PodNetwork implements base.Validator
func (r *Validator) PodNetwork(vmRef ref.Ref) (ok bool, err error) {
	if r.plan.Referenced.Map.Network == nil {
		return
	}

	vm := &cnv.VirtualMachine{}
	err = r.sourceClient.Get(context.TODO(), k8sclient.ObjectKey{Namespace: vmRef.Namespace, Name: vmRef.Name}, vm)
	if err != nil {
		err = liberr.Wrap(
			err,
			VM_NOT_FOUND,
			"vm",
			vmRef.String())
		return
	}
	mapping := r.plan.Referenced.Map.Network.Spec.Map
	podMapped := 0
	for i := range mapping {
		mapped := &mapping[i]
		if mapped.Destination.Type == Pod {
			podMapped++
		}
	}

	ok = podMapped <= 1
	return
}

// WarmMigration implements base.Validator
func (r *Validator) WarmMigration() bool {
	return false
}

// NOOP
func (r *Validator) InvalidDiskSizes(vmRef ref.Ref) ([]string, error) {
	return []string{}, nil
}

func (r *Validator) MacConflicts(vmRef ref.Ref) ([]planbase.MacConflict, error) {
	// Only check MAC conflicts for live migrations
	// For cold migrations, the source VM is shut down, so no conflicts occur
	if r.Plan.Spec.Type != api.MigrationLive {
		r.log.V(1).Info("Skipping MAC conflict check for non-live migration",
			"migrationType", r.Plan.Spec.Type, "vm", vmRef.String())
		return []planbase.MacConflict{}, nil
	}

	r.log.Info("Checking MAC conflicts for live migration", "vm", vmRef.String())

	// Get source VM using common helper
	vm, err := planbase.FindSourceVM[inventory.VM](r.Source.Inventory, vmRef)
	if err != nil {
		return nil, err
	}

	// Get destination VMs and extract their MACs using common helper
	destinationVMs, err := planbase.GetDestinationVMsFromInventory(r.Destination.Inventory, web.Param{
		Key:   web.DetailParam,
		Value: "all",
	})
	if err != nil {
		return nil, liberr.Wrap(err)
	}

	// Extract source VM MACs
	var sourceMacs []string
	if vm.Object.Spec.Template != nil {
		for _, iface := range vm.Object.Spec.Template.Spec.Domain.Devices.Interfaces {
			// Include all MACs, even empty ones - the helper function will handle filtering
			sourceMacs = append(sourceMacs, iface.MacAddress)
		}
	}

	// Use common helper to detect conflicts
	conflicts := planbase.CheckMacConflicts(sourceMacs, destinationVMs)

	if len(conflicts) > 0 {
		r.log.Info("MAC conflicts detected for live migration",
			"vm", vmRef.String(), "conflicts", len(conflicts))
	} else {
		r.log.V(1).Info("No MAC conflicts detected", "vm", vmRef.String())
	}

	return conflicts, nil
}

func (r *Validator) SharedDisks(vmRef ref.Ref, client k8sclient.Client) (ok bool, s string, s2 string, err error) {
	ok = true
	return
}

// Load.
func (r *Validator) Load() (err error) {
	r.inventory, err = web.NewClient(r.plan.Referenced.Provider.Source)
	return
}

func (r *Validator) StorageMapped(vmRef ref.Ref) (ok bool, err error) {
	if r.plan.Referenced.Map.Storage == nil {
		return
	}

	vm := &cnv.VirtualMachine{}
	err = r.sourceClient.Get(context.TODO(), k8sclient.ObjectKey{Namespace: vmRef.Namespace, Name: vmRef.Name}, vm)
	if err != nil {
		err = liberr.Wrap(
			err,
			VM_NOT_FOUND,
			"vm",
			vmRef.String())
		return
	}

	for _, vol := range vm.Spec.Template.Spec.Volumes {
		var pvcName string
		switch {
		case vol.PersistentVolumeClaim != nil:
			pvcName = vol.PersistentVolumeClaim.ClaimName
		case vol.DataVolume != nil:
			pvcName = vol.DataVolume.Name
		default:
			r.log.Info("Not PVC or DataVolume, skipping volume...", "volume", vol.Name)
			continue
		}

		pvc := &core.PersistentVolumeClaim{}
		err = r.sourceClient.Get(context.TODO(), k8sclient.ObjectKey{
			Namespace: vmRef.Namespace,
			Name:      pvcName,
		}, pvc)
		if err != nil {
			err = liberr.Wrap(
				err,
				"PVC not found.",
				"pvc",
				pvcName)
			return
		}

		storageClass := pvc.Spec.StorageClassName
		if storageClass == nil {
			return false, nil
		}

		_, found := r.plan.Referenced.Map.Storage.FindStorageByName(*storageClass)
		if !found {
			err = liberr.Wrap(
				err,
				"StorageClass not found.",
				"StorageClass",
				*storageClass)

			return false, err
		}
	}

	return true, nil
}

// Validate that a VM's networks have been mapped.
func (r *Validator) NetworksMapped(vmRef ref.Ref) (ok bool, err error) {
	if r.plan.Referenced.Map.Network == nil {
		return
	}

	vm := &cnv.VirtualMachine{}
	err = r.sourceClient.Get(context.TODO(), k8sclient.ObjectKey{Namespace: vmRef.Namespace, Name: vmRef.Name}, vm)
	if err != nil {
		err = liberr.Wrap(
			err,
			VM_NOT_FOUND,
			"vm",
			vmRef.String())
		return
	}

	for _, net := range vm.Spec.Template.Spec.Networks {
		if net.Pod != nil {
			_, found := r.plan.Referenced.Map.Network.FindNetworkByType(Pod)
			if !found {
				err = liberr.Wrap(
					err,
					"Pod network not found.",
					"vm",
					vmRef.String(),
				)
				r.log.Error(err, "Pod network not found.")

				return false, err
			}
		} else if net.Multus != nil {
			name, namespace := ocpclient.GetNetworkNameAndNamespace(net.Multus.NetworkName, &vmRef)
			_, found := r.plan.Referenced.Map.Network.FindNetworkByNameAndNamespace(namespace, name)
			if !found {
				err = liberr.Wrap(
					err,
					"Multus network not found.",
					"network",
					fmt.Sprintf("%s/%s", namespace, name),
				)
				r.log.Error(err, "Multus network not found.")
				return false, err
			}
		}
	}

	return true, nil
}

// NO-OP
func (r *Validator) DirectStorage(vmRef ref.Ref) (bool, error) {
	return true, nil
}

// NO-OP
func (r *Validator) StaticIPs(vmRef ref.Ref) (bool, error) {
	return true, nil
}

// NO-OP
func (r *Validator) ChangeTrackingEnabled(vmRef ref.Ref) (bool, error) {
	return true, nil
}
