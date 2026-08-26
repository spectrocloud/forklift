package plan

import (
	"context"
	"maps"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	core "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Ensure the namespace exists on the destination.
// spectroPrivilegedPSALabels relax Pod Security Admission on the target
// namespace. The virt-v2v pod runs with CAP_SYS_ADMIN and an unconfined
// seccomp profile outside OpenShift, which the restricted PSA levels reject.
var spectroPrivilegedPSALabels = map[string]string{
	"pod-security.kubernetes.io/enforce": "privileged",
	"pod-security.kubernetes.io/audit":   "privileged",
	"pod-security.kubernetes.io/warn":    "privileged",
}

func ensureNamespace(plan *api.Plan, client client.Client) error {
	ns := &core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Name:   plan.Spec.TargetNamespace,
			Labels: maps.Clone(spectroPrivilegedPSALabels),
		},
	}
	err := client.Create(context.TODO(), ns)
	if err == nil {
		return nil
	}
	if !k8serr.IsAlreadyExists(err) {
		return err
	}

	// Spectro: the namespace may pre-date the plan, or have been created by a
	// prior release without these labels, so bring an existing one up to date.
	existing := &core.Namespace{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: plan.Spec.TargetNamespace}, existing); err != nil {
		// Namespace exists but is unreadable; leave it alone rather than fail
		// the migration on a permissions problem.
		return nil
	}
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	changed := false
	for k, v := range spectroPrivilegedPSALabels {
		if existing.Labels[k] != v {
			existing.Labels[k] = v
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return client.Update(context.TODO(), existing)
}

// Ensure the config map exists on the destination
func ensureConfigMap(cm *core.ConfigMap, name func(plan *api.Plan) string, plan *api.Plan, client client.Client) error {
	cm.ObjectMeta = meta.ObjectMeta{
		Name:      name(plan),
		Namespace: plan.Spec.TargetNamespace,
	}
	err := client.Create(context.TODO(), cm)
	if err != nil && k8serr.IsAlreadyExists(err) {
		err = nil
	}
	return err
}
