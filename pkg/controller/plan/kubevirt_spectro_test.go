//nolint:errcheck
package plan

import (
	"context"
	"encoding/json"
	k8snet "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	v1beta1 "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	planbase "github.com/kubev2v/forklift/pkg/controller/plan/adapter/base"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	provweb "github.com/kubev2v/forklift/pkg/controller/provider/web"
	webbase "github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	"github.com/kubev2v/forklift/pkg/settings"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	templatev1 "github.com/openshift/api/template/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	cnv "kubevirt.io/api/core/v1"
	cdi "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"time"
)

var KubeVirtLogSpectro = logging.WithName("kubevirt-test")

// stubInventory satisfies provider/web.Client and is used to avoid nil deref when
// KubeVirt helpers call r.Source.Inventory.VM().
type stubInventory struct{}

func (stubInventory) Finder() provweb.Finder { return nil }
func (stubInventory) Get(resource interface{}, id string) error {
	return nil
}
func (stubInventory) List(list interface{}, param ...provweb.Param) error {
	return nil
}
func (stubInventory) Watch(resource interface{}, h provweb.EventHandler) (*provweb.Watch, error) {
	return nil, nil
}
func (stubInventory) Find(resource interface{}, rf webbase.Ref) error { return nil }
func (stubInventory) VM(rf *webbase.Ref) (interface{}, error)         { return struct{}{}, nil }
func (stubInventory) Workload(rf *webbase.Ref) (interface{}, error)   { return struct{}{}, nil }
func (stubInventory) Network(rf *webbase.Ref) (interface{}, error)    { return struct{}{}, nil }
func (stubInventory) Storage(rf *webbase.Ref) (interface{}, error)    { return struct{}{}, nil }
func (stubInventory) Host(rf *webbase.Ref) (interface{}, error)       { return struct{}{}, nil }

type fakeBuilder struct {
	templateLabels map[string]string
}

func (b fakeBuilder) Secret(vmRef ref.Ref, in, object *v1.Secret) error { return nil }
func (b fakeBuilder) ConfigMap(vmRef ref.Ref, secret *v1.Secret, object *v1.ConfigMap) error {
	return nil
}
func (b fakeBuilder) VirtualMachine(vmRef ref.Ref, object *cnv.VirtualMachineSpec, persistentVolumeClaims []*v1.PersistentVolumeClaim, usesInstanceType bool, sortVolumesByLibvirt bool) error {
	return nil
}
func (b fakeBuilder) DataVolumes(vmRef ref.Ref, secret *v1.Secret, configMap *v1.ConfigMap, dvTemplate *cdi.DataVolume, vddkConfigMap *v1.ConfigMap) (dvs []cdi.DataVolume, err error) {
	return nil, nil
}
func (b fakeBuilder) Tasks(vmRef ref.Ref) ([]*planapi.Task, error) { return nil, nil }
func (b fakeBuilder) TemplateLabels(vmRef ref.Ref) (labels map[string]string, err error) {
	return b.templateLabels, nil
}
func (b fakeBuilder) ResolveDataVolumeIdentifier(dv *cdi.DataVolume) string { return dv.Name }
func (b fakeBuilder) ResolvePersistentVolumeClaimIdentifier(pvc *v1.PersistentVolumeClaim) string {
	return pvc.Name
}
func (b fakeBuilder) PodEnvironment(vmRef ref.Ref, sourceSecret *v1.Secret) (env []v1.EnvVar, err error) {
	return nil, nil
}
func (b fakeBuilder) LunPersistentVolumes(vmRef ref.Ref) (pvs []v1.PersistentVolume, err error) {
	return nil, nil
}
func (b fakeBuilder) LunPersistentVolumeClaims(vmRef ref.Ref) (pvcs []v1.PersistentVolumeClaim, err error) {
	return nil, nil
}
func (b fakeBuilder) SupportsVolumePopulators() bool { return false }
func (b fakeBuilder) PopulatorVolumes(vmRef ref.Ref, annotations map[string]string, secretName string) ([]*v1.PersistentVolumeClaim, error) {
	return nil, nil
}
func (b fakeBuilder) PopulatorTransferredBytes(persistentVolumeClaim *v1.PersistentVolumeClaim) (transferredBytes int64, err error) {
	return 0, nil
}
func (b fakeBuilder) SetPopulatorDataSourceLabels(vmRef ref.Ref, pvcs []*v1.PersistentVolumeClaim) (err error) {
	return nil
}
func (b fakeBuilder) GetPopulatorTaskName(pvc *v1.PersistentVolumeClaim) (taskName string, err error) {
	return "", nil
}
func (b fakeBuilder) PreferenceName(vmRef ref.Ref, configMap *v1.ConfigMap) (name string, err error) {
	return "", nil
}

var _ = ginkgo.Describe("kubevirt tests", func() {
	ginkgo.Describe("getDiskIndex", func() {
		ginkgo.It("should return -1 when annotation missing or invalid", func() {
			pvcMissing := &v1.PersistentVolumeClaim{}
			Expect(getDiskIndex(pvcMissing)).To(Equal(-1))

			pvcInvalid := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						planbase.AnnDiskIndex: "not-an-int",
					},
				},
			}
			Expect(getDiskIndex(pvcInvalid)).To(Equal(-1))
		})

		ginkgo.It("should return the parsed disk index", func() {
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						planbase.AnnDiskIndex: "3",
					},
				},
			}
			Expect(getDiskIndex(pvc)).To(Equal(3))
		})
	})

	ginkgo.Describe("getPVCs", func() {
		ginkgo.It("should return PVCs", func() {
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test",
					Labels: map[string]string{
						"migration": "test",
						"vmID":      "test",
					},
				},
			}
			kubevirt := createKubeVirtSpectro(pvc)
			pvcs, err := kubevirt.getPVCs(ref.Ref{ID: "test"})
			Expect(err).ToNot(HaveOccurred())
			Expect(pvcs).To(HaveLen(1))
		})

		ginkgo.It("should sort PVCs by disk index annotation", func() {
			pvcMissing := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc-missing",
					Namespace: "test",
					Labels: map[string]string{
						"migration": "test",
						"vmID":      "test",
					},
				},
			}
			pvcIndex2 := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc-2",
					Namespace: "test",
					Labels: map[string]string{
						"migration": "test",
						"vmID":      "test",
					},
					Annotations: map[string]string{
						planbase.AnnDiskIndex: "2",
					},
				},
			}
			pvcIndex0 := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc-0",
					Namespace: "test",
					Labels: map[string]string{
						"migration": "test",
						"vmID":      "test",
					},
					Annotations: map[string]string{
						planbase.AnnDiskIndex: "0",
					},
				},
			}

			kubevirt := createKubeVirtSpectro(pvcIndex2, pvcMissing, pvcIndex0)
			pvcs, err := kubevirt.getPVCs(ref.Ref{ID: "test"})
			Expect(err).ToNot(HaveOccurred())
			Expect(pvcs).To(HaveLen(3))

			// Missing annotation => -1, should come first.
			Expect(pvcs[0].Name).To(Equal("pvc-missing"))
			Expect(pvcs[1].Name).To(Equal("pvc-0"))
			Expect(pvcs[2].Name).To(Equal("pvc-2"))
		})
	})

	ginkgo.Describe("VirtualMachineMap", func() {
		ginkgo.It("should map VMs by vmID label", func() {
			vm := &cnv.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-obj",
					Namespace: "test",
					Labels: map[string]string{
						kPlan: "plan-uid",
						kVM:   "vm-1",
					},
				},
			}
			kubevirt := createKubeVirtSpectro(vm)
			mp, err := kubevirt.VirtualMachineMap()
			Expect(err).ToNot(HaveOccurred())
			Expect(mp).To(HaveLen(1))
			_, found := mp["vm-1"]
			Expect(found).To(BeTrue())
		})
	})

	ginkgo.Describe("label helpers", func() {
		ginkgo.It("should build plan/vm labels deterministically", func() {
			kubevirt := createKubeVirtSpectro()

			pl := kubevirt.planLabels()
			Expect(pl).To(HaveKeyWithValue(kMigration, "test"))
			Expect(pl).To(HaveKeyWithValue(kPlan, "plan-uid"))

			vmRef := ref.Ref{ID: "vm-1"}
			vl := kubevirt.vmLabels(vmRef)
			Expect(vl).To(HaveKeyWithValue(kMigration, "test"))
			Expect(vl).To(HaveKeyWithValue(kPlan, "plan-uid"))
			Expect(vl).To(HaveKeyWithValue(kVM, "vm-1"))

			noMig := kubevirt.vmAllButMigrationLabels(vmRef)
			Expect(noMig).ToNot(HaveKey(kMigration))
			Expect(noMig).To(HaveKeyWithValue(kPlan, "plan-uid"))
			Expect(noMig).To(HaveKeyWithValue(kVM, "vm-1"))
		})

		ginkgo.It("should include app label for consumer/conversion pods", func() {
			kubevirt := createKubeVirtSpectro()
			vmRef := ref.Ref{ID: "vm-1"}

			cl := kubevirt.consumerLabels(vmRef, false)
			Expect(cl).To(HaveKeyWithValue(kApp, "consumer"))

			vl := kubevirt.conversionLabels(vmRef, false)
			Expect(vl).To(HaveKeyWithValue(kApp, "virt-v2v"))
		})
	})

	ginkgo.Describe("name helpers", func() {
		ginkgo.It("should generate stable configmap names", func() {
			p := &v1beta1.Plan{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
			Expect(genExtraV2vConfConfigMapName(p)).To(Equal("p-extra-v2v-conf"))
			Expect(genVddkConfConfigMapName(p)).To(Equal("p-vddk-conf-"))
		})

		ginkgo.It("should generate OVA entity name prefixes", func() {
			Expect(getEntityPrefixName("pv", "prov", "plan")).To(Equal("ova-store-pv-prov-plan-"))
		})
	})

	ginkgo.Describe("vmOwnerReference", func() {
		ginkgo.It("should build an OwnerReference with expected fields", func() {
			vm := &cnv.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vm1",
					UID:  types.UID("uid1"),
				},
			}
			oref := vmOwnerReference(vm)
			Expect(oref.APIVersion).To(Equal("kubevirt.io/v1"))
			Expect(oref.Kind).To(Equal("VirtualMachine"))
			Expect(oref.Name).To(Equal("vm1"))
			Expect(oref.UID).To(Equal(types.UID("uid1")))
			Expect(oref.BlockOwnerDeletion).ToNot(BeNil())
			Expect(*oref.BlockOwnerDeletion).To(BeTrue())
			Expect(oref.Controller).ToNot(BeNil())
			Expect(*oref.Controller).To(BeFalse())
		})
	})

	ginkgo.Describe("ExtendedDataVolume", func() {
		ginkgo.It("should parse PercentComplete from status progress", func() {
			edv := &ExtendedDataVolume{DataVolume: &cdi.DataVolume{}}
			edv.Status.Progress = cdi.DataVolumeProgress("50%")
			Expect(edv.PercentComplete()).To(BeNumerically("~", 0.5, 0.0001))

			edv.Status.Progress = cdi.DataVolumeProgress("not-a-percent")
			Expect(edv.PercentComplete()).To(Equal(float64(0)))
		})

		ginkgo.It("should convert DV conditions into forklift conditions", func() {
			edv := &ExtendedDataVolume{
				DataVolume: &cdi.DataVolume{
					Status: cdi.DataVolumeStatus{
						Conditions: []cdi.DataVolumeCondition{
							{
								Type:               cdi.DataVolumeReady,
								Status:             v1.ConditionTrue,
								Reason:             "Ok",
								Message:            "ready",
								LastTransitionTime: metav1.Now(),
							},
						},
					},
				},
			}
			cnd := edv.Conditions()
			Expect(cnd).ToNot(BeNil())
			got := cnd.FindCondition(string(cdi.DataVolumeReady))
			Expect(got).ToNot(BeNil())
			Expect(got.Status).To(Equal("True"))
			Expect(got.Reason).To(Equal("Ok"))
		})
	})

	ginkgo.Describe("VirtualMachine helpers", func() {
		ginkgo.It("Owner should detect matching PVC claim names", func() {
			vm := &VirtualMachine{
				VirtualMachine: &cnv.VirtualMachine{
					Spec: cnv.VirtualMachineSpec{
						Template: &cnv.VirtualMachineInstanceTemplateSpec{
							Spec: cnv.VirtualMachineInstanceSpec{
								Volumes: []cnv.Volume{
									{
										Name: "dvvol",
										VolumeSource: cnv.VolumeSource{
											PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{ClaimName: "dv-1"}},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(vm.Owner(&cdi.DataVolume{ObjectMeta: metav1.ObjectMeta{Name: "dv-1"}})).To(BeTrue())
			Expect(vm.Owner(&cdi.DataVolume{ObjectMeta: metav1.ObjectMeta{Name: "dv-2"}})).To(BeFalse())
		})

		ginkgo.It("Conditions should expose VM status conditions", func() {
			vm := &VirtualMachine{
				VirtualMachine: &cnv.VirtualMachine{
					Status: cnv.VirtualMachineStatus{
						Conditions: []cnv.VirtualMachineCondition{
							{
								Type:   "Ready",
								Status: v1.ConditionTrue,
								Reason: "Ok",
							},
						},
					},
				},
			}
			cnd := vm.Conditions()
			Expect(cnd).ToNot(BeNil())
			got := cnd.FindCondition("Ready")
			Expect(got).ToNot(BeNil())
			Expect(got.Status).To(Equal(libcnd.True))
		})
	})

	ginkgo.Describe("migration helpers", func() {
		ginkgo.It("terminationMessage should return message only for non-zero exits", func() {
			podNoStatus := &v1.Pod{}
			msg, ok := terminationMessage(podNoStatus)
			Expect(ok).To(BeFalse())
			Expect(msg).To(Equal(""))

			podExit0 := &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							LastTerminationState: v1.ContainerState{
								Terminated: &v1.ContainerStateTerminated{ExitCode: 0, Message: "ignored"},
							},
						},
					},
				},
			}
			msg, ok = terminationMessage(podExit0)
			Expect(ok).To(BeFalse())
			Expect(msg).To(Equal(""))

			podExit1 := &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							LastTerminationState: v1.ContainerState{
								Terminated: &v1.ContainerStateTerminated{ExitCode: 1, Message: "boom"},
							},
						},
					},
				},
			}
			msg, ok = terminationMessage(podExit1)
			Expect(ok).To(BeTrue())
			Expect(msg).To(Equal("boom"))
		})

		ginkgo.It("restartLimitExceeded should compare restart count with configured retry limit", func() {
			orig := settings.Settings.ImporterRetry
			settings.Settings.ImporterRetry = 2
			defer func() { settings.Settings.ImporterRetry = orig }()

			podNoStatus := &v1.Pod{}
			Expect(restartLimitExceeded(podNoStatus)).To(BeFalse())

			pod := &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{RestartCount: 2},
					},
				},
			}
			Expect(restartLimitExceeded(pod)).To(BeFalse())

			pod.Status.ContainerStatuses[0].RestartCount = 3
			Expect(restartLimitExceeded(pod)).To(BeTrue())
		})
	})

	ginkgo.Describe("KubeVirt PV/PVC helpers", func() {
		ginkgo.It("setPopulatorPodLabels should patch migration label", func() {
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "p1",
					Namespace: "test",
					Labels:    map[string]string{"a": "b"},
				},
			}
			kubevirt := createKubeVirtSpectro(pod)

			// Pass pod by value as required by the helper.
			err := kubevirt.setPopulatorPodLabels(*pod, "mig-123")
			Expect(err).ToNot(HaveOccurred())

			got := &v1.Pod{}
			Expect(kubevirt.Destination.Get(
				context.TODO(),
				types.NamespacedName{Name: "p1", Namespace: "test"},
				got,
			)).To(Succeed())
			Expect(got.Labels).To(HaveKeyWithValue(kMigration, "mig-123"))
			Expect(got.Labels).To(HaveKeyWithValue("a", "b"))
		})

		ginkgo.It("EnsurePersistentVolumeClaim should create missing PVCs and skip existing ones", func() {
			kubevirt := createKubeVirtSpectro()
			vmRef := ref.Ref{ID: "vm-1"}

			existing := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc-existing",
					Namespace: "test",
					Labels: map[string]string{
						"migration": "test",
						"vmID":      "vm-1",
						"volume":    "vol-1",
					},
					Annotations: map[string]string{planbase.AnnDiskIndex: "0"},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			// Create existing PVC in the fake destination.
			Expect(kubevirt.Destination.Create(context.TODO(), existing)).To(Succeed())

			desired := []v1.PersistentVolumeClaim{
				// This one already exists by matching "volume" label.
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ignored-name",
						Namespace: "test",
						Labels: map[string]string{
							"migration": "test",
							"vmID":      "vm-1",
							"volume":    "vol-1",
						},
					},
				},
				// This one should be created.
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pvc-new",
						Namespace: "test",
						Labels: map[string]string{
							"migration": "test",
							"vmID":      "vm-1",
							"volume":    "vol-2",
						},
					},
					Spec: v1.PersistentVolumeClaimSpec{
						Resources: v1.VolumeResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				},
			}

			Expect(kubevirt.EnsurePersistentVolumeClaim(vmRef, desired)).To(Succeed())

			// Validate the new PVC exists.
			got := &v1.PersistentVolumeClaim{}
			Expect(kubevirt.Destination.Get(
				context.TODO(),
				types.NamespacedName{Name: "pvc-new", Namespace: "test"},
				got,
			)).To(Succeed())
			Expect(got.Labels).To(HaveKeyWithValue("volume", "vol-2"))
		})

		ginkgo.It("EnsurePersistentVolume should create missing PVs and skip existing ones", func() {
			kubevirt := createKubeVirtSpectro()
			vmRef := ref.Ref{ID: "vm-1"}

			existing := &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-existing",
					Labels: map[string]string{
						"migration": "test",
						"plan":      "plan-uid",
						"vmID":      "vm-1",
						"volume":    "vol-1",
					},
				},
				Spec: v1.PersistentVolumeSpec{
					Capacity: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), existing)).To(Succeed())

			desired := []v1.PersistentVolume{
				// matches by volume label => should be skipped
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ignored",
						Labels: map[string]string{
							"migration": "test",
							"plan":      "plan-uid",
							"vmID":      "vm-1",
							"volume":    "vol-1",
						},
					},
					Spec: v1.PersistentVolumeSpec{
						Capacity: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
				// should be created
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pv-new",
						Labels: map[string]string{
							"migration": "test",
							"plan":      "plan-uid",
							"vmID":      "vm-1",
							"volume":    "vol-2",
						},
					},
					Spec: v1.PersistentVolumeSpec{
						Capacity: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}

			Expect(kubevirt.EnsurePersistentVolume(vmRef, desired)).To(Succeed())

			got := &v1.PersistentVolume{}
			Expect(kubevirt.Destination.Get(
				context.TODO(),
				types.NamespacedName{Name: "pv-new"},
				got,
			)).To(Succeed())
			Expect(got.Labels).To(HaveKeyWithValue("volume", "vol-2"))
		})

		ginkgo.It("GetOvaPvListNfs/GetOvaPvcListNfs should list by labels", func() {
			kubevirt := createKubeVirtSpectro()

			pv := &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv1",
					Labels: map[string]string{
						"plan": "plan-uid",
						"ova":  OvaPVLabel,
					},
				},
				Spec: v1.PersistentVolumeSpec{
					Capacity: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
				},
			}
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc1",
					Namespace: "test",
					Labels: map[string]string{
						"plan": "plan-uid",
						"ova":  OvaPVCLabel,
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), pv)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), pvc)).To(Succeed())

			pvs, _, err := GetOvaPvListNfs(kubevirt.Destination.Client, "plan-uid")
			Expect(err).ToNot(HaveOccurred())
			Expect(pvs.Items).To(HaveLen(1))
			Expect(pvs.Items[0].Name).To(Equal("pv1"))

			pvcs, _, err := GetOvaPvcListNfs(kubevirt.Destination.Client, "plan-uid", "test")
			Expect(err).ToNot(HaveOccurred())
			Expect(pvcs.Items).To(HaveLen(1))
			Expect(pvcs.Items[0].Name).To(Equal("pvc1"))
		})

		ginkgo.It("IsCopyOffload should detect annotation key", func() {
			kubevirt := createKubeVirtSpectro()
			pvcs := []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"x": "y"}}},
				{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"copy-offload": "true"}}},
			}
			Expect(kubevirt.IsCopyOffload(pvcs)).To(BeTrue())
			Expect(kubevirt.IsCopyOffload([]*v1.PersistentVolumeClaim{{}})).To(BeFalse())
		})
	})

	ginkgo.Describe("KubeVirt misc helpers", func() {
		ginkgo.It("gen*ConfigMapName helpers should format names", func() {
			p := &v1beta1.Plan{ObjectMeta: metav1.ObjectMeta{Name: "p1"}}
			Expect(genExtraV2vConfConfigMapName(p)).To(Equal("p1-" + ExtraV2vConf))
			Expect(genVddkConfConfigMapName(p)).To(Equal("p1-" + VddkConf + "-"))
		})

		ginkgo.It("GetImporterPod should return not found when annotation missing", func() {
			kubevirt := createKubeVirtSpectro()
			pvc := v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "pvc1",
					Namespace:   "test",
					Annotations: map[string]string{},
				},
			}
			pod, found, err := kubevirt.GetImporterPod(pvc)
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeFalse())
			Expect(pod).ToNot(BeNil())
		})

		ginkgo.It("GetImporterPod should return found when pod exists", func() {
			podObj := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "importer",
					Namespace: "test",
				},
			}
			kubevirt := createKubeVirtSpectro(podObj)
			pvc := v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc1",
					Namespace: "test",
					Annotations: map[string]string{
						AnnImporterPodName: "importer",
					},
				},
			}
			pod, found, err := kubevirt.GetImporterPod(pvc)
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(pod.Name).To(Equal("importer"))
		})

		ginkgo.It("setKvmOnPodSpec should set selector + resources for vSphere/OVA when enabled", func() {
			kubevirt := createKubeVirtSpectro()
			orig := settings.Settings.VirtV2vDontRequestKVM
			settings.Settings.VirtV2vDontRequestKVM = false
			defer func() { settings.Settings.VirtV2vDontRequestKVM = orig }()

			vs := v1beta1.VSphere
			kubevirt.Plan.Provider.Source = &v1beta1.Provider{Spec: v1beta1.ProviderSpec{Type: &vs}}

			ps := &v1.PodSpec{
				Containers: []v1.Container{{}},
			}
			kubevirt.setKvmOnPodSpec(ps)
			Expect(ps.NodeSelector).To(HaveKeyWithValue("kubevirt.io/schedulable", "true"))
			Expect(ps.Containers[0].Resources.Limits).To(HaveKey(v1.ResourceName("devices.kubevirt.io/kvm")))
			Expect(ps.Containers[0].Resources.Requests).To(HaveKey(v1.ResourceName("devices.kubevirt.io/kvm")))
		})

		ginkgo.It("setKvmOnPodSpec should be a no-op when disabled", func() {
			kubevirt := createKubeVirtSpectro()
			orig := settings.Settings.VirtV2vDontRequestKVM
			settings.Settings.VirtV2vDontRequestKVM = true
			defer func() { settings.Settings.VirtV2vDontRequestKVM = orig }()

			vs := v1beta1.VSphere
			kubevirt.Plan.Provider.Source = &v1beta1.Provider{Spec: v1beta1.ProviderSpec{Type: &vs}}

			ps := &v1.PodSpec{Containers: []v1.Container{{}}}
			kubevirt.setKvmOnPodSpec(ps)
			Expect(ps.NodeSelector).To(BeNil())
			Expect(ps.Containers[0].Resources.Limits).To(BeNil())
			Expect(ps.Containers[0].Resources.Requests).To(BeNil())
		})

		ginkgo.It("getListOptionsNamespaced should set namespace", func() {
			kubevirt := createKubeVirtSpectro()
			kubevirt.Plan.Spec.TargetNamespace = "tns"
			opts := kubevirt.getListOptionsNamespaced()
			Expect(opts.Namespace).To(Equal("tns"))
		})

		ginkgo.It("getGeneratedName/getNewVMName should format names", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1", Name: "orig"}}, NewName: ""}
			Expect(kubevirt.getGeneratedName(vm)).To(Equal("plan-vm-1-"))
			Expect(kubevirt.getNewVMName(vm)).To(Equal("orig"))

			vm.NewName = "new"
			Expect(kubevirt.getNewVMName(vm)).To(Equal("new"))
		})

		ginkgo.It("EnsureNamespace should create namespace and set privileged PSA labels (including update on already-exists)", func() {
			// Create an existing namespace without labels to hit the AlreadyExists update path.
			existing := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			}
			kubevirt := createKubeVirtSpectro(existing)
			Expect(kubevirt.EnsureNamespace()).To(Succeed())

			got := &v1.Namespace{}
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "test"}, got)).To(Succeed())
			Expect(got.Labels).To(HaveKeyWithValue("pod-security.kubernetes.io/enforce", "privileged"))
			Expect(got.Labels).To(HaveKeyWithValue("pod-security.kubernetes.io/audit", "privileged"))
			Expect(got.Labels).To(HaveKeyWithValue("pod-security.kubernetes.io/warn", "privileged"))
		})

		ginkgo.It("ListVMs and VirtualMachineMap should list by plan labels and key by vmID label", func() {
			kubevirt := createKubeVirtSpectro()

			labels := kubevirt.planLabels()
			// ListVMs deletes the migration label before listing.
			delete(labels, kMigration)
			labels[kVM] = "vm-1"

			vm := &cnv.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: "test",
					Labels:    labels,
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), vm)).To(Succeed())

			list, err := kubevirt.ListVMs()
			Expect(err).ToNot(HaveOccurred())
			Expect(list).To(HaveLen(1))
			Expect(list[0].Labels[kVM]).To(Equal("vm-1"))

			mp, err := kubevirt.VirtualMachineMap()
			Expect(err).ToNot(HaveOccurred())
			Expect(mp).To(HaveKey("vm-1"))
		})

		ginkgo.It("getImporterPods/DeleteImporterPods should filter and delete matching CDI importer pods", func() {
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc1",
					Namespace: "test",
					Annotations: map[string]string{
						AnnImporterPodName: "any",
					},
				},
			}
			match := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "importer-pvc1-xyz",
					Namespace: "test",
					Labels:    map[string]string{"app": "containerized-data-importer"},
				},
			}
			other := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "something-else",
					Namespace: "test",
					Labels:    map[string]string{"app": "containerized-data-importer"},
				},
			}
			kubevirt := createKubeVirtSpectro(pvc, match, other)

			pods, err := kubevirt.getImporterPods(pvc)
			Expect(err).ToNot(HaveOccurred())
			Expect(pods).To(HaveLen(1))
			Expect(pods[0].Name).To(ContainSubstring("importer-pvc1"))

			Expect(kubevirt.DeleteImporterPods(pvc)).To(Succeed())

			// Matching pod should be gone; other should remain.
			gone := &v1.Pod{}
			err = kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: match.Name, Namespace: "test"}, gone)
			Expect(err).To(HaveOccurred())

			still := &v1.Pod{}
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: other.Name, Namespace: "test"}, still)).To(Succeed())
		})

		ginkgo.It("EnsureExtraV2vConfConfigMap should copy a source configmap into destination with generated name", func() {
			orig := settings.Settings.VirtV2vExtraConfConfigMap
			settings.Settings.VirtV2vExtraConfConfigMap = "extra-src"
			defer func() { settings.Settings.VirtV2vExtraConfConfigMap = orig }()

			scheme := runtime.NewScheme()
			_ = v1.AddToScheme(scheme)
			v1beta1.SchemeBuilder.AddToScheme(scheme)
			_ = cnv.AddToScheme(scheme)
			_ = cdi.AddToScheme(scheme)

			srcCm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "extra-src",
					Namespace: "test",
				},
				Data: map[string]string{"k": "v"},
			}
			srcClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(srcCm).Build()
			dstClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			planObj := &v1beta1.Plan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "plan",
					Namespace: "test",
					UID:       types.UID("plan-uid"),
				},
				Spec: v1beta1.PlanSpec{
					TargetNamespace: "test",
				},
			}

			kubevirt := &KubeVirt{
				Context: &plancontext.Context{
					Destination: plancontext.Destination{Client: dstClient},
					Log:         KubeVirtLogSpectro,
					Migration:   createMigrationSpectro(),
					Plan:        planObj,
					Client:      srcClient,
				},
			}

			Expect(kubevirt.EnsureExtraV2vConfConfigMap()).To(Succeed())

			got := &v1.ConfigMap{}
			Expect(dstClient.Get(context.TODO(), types.NamespacedName{Name: genExtraV2vConfConfigMapName(planObj), Namespace: "test"}, got)).To(Succeed())
			Expect(got.Data).To(HaveKeyWithValue("k", "v"))
		})

		ginkgo.It("pod/job deletion helpers should delete matching resources and ignore not-found", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{
				VM: planapi.VM{
					Ref: ref.Ref{ID: "vm-1", Name: "vm1"},
				},
			}

			// Consumer + conversion pods.
			consumerLabels := kubevirt.consumerLabels(vm.Ref, true)
			conversionLabels := kubevirt.conversionLabels(vm.Ref, true)
			consumerPod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "consumer", Namespace: "test", Labels: consumerLabels}}
			conversionPod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "v2v", Namespace: "test", Labels: conversionLabels}}
			otherPod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "test", Labels: map[string]string{"x": "y"}}}
			Expect(kubevirt.Destination.Create(context.TODO(), consumerPod)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), conversionPod)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), otherPod)).To(Succeed())

			Expect(kubevirt.DeletePVCConsumerPod(vm)).To(Succeed())
			Expect(kubevirt.DeleteGuestConversionPod(vm)).To(Succeed())

			// consumer/conversion deleted; other remains
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "consumer", Namespace: "test"}, &v1.Pod{})).ToNot(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "v2v", Namespace: "test"}, &v1.Pod{})).ToNot(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "other", Namespace: "test"}, &v1.Pod{})).To(Succeed())

			// Hook jobs.
			jobLabels := kubevirt.vmAllButMigrationLabels(vm.Ref)
			job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "test", Labels: jobLabels}}
			otherJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job2", Namespace: "test", Labels: map[string]string{"x": "y"}}}
			Expect(kubevirt.Destination.Create(context.TODO(), job)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), otherJob)).To(Succeed())

			Expect(kubevirt.DeleteHookJobs(vm)).To(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "job1", Namespace: "test"}, &batchv1.Job{})).ToNot(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "job2", Namespace: "test"}, &batchv1.Job{})).To(Succeed())

			// DeleteObject should ignore NotFound.
			missing := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "missing", Namespace: "test"}}
			Expect(kubevirt.DeleteObject(missing, vm, "x", "cm")).To(Succeed())
		})

		ginkgo.It("getPopulatorPods/DeletePopulatorPods should filter by migration label and prefix", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1"}}}

			// Ensure Plan.Status.Migration.ActiveSnapshot().Migration.UID is set.
			kubevirt.Plan.Status.Migration.History = []planapi.Snapshot{
				{Migration: planapi.SnapshotRef{UID: types.UID("miguid")}},
			}

			match := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PopulatorPodPrefix + "abc",
					Namespace: "test",
					Labels:    map[string]string{kMigration: "miguid"},
				},
			}
			nonPrefix := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "not-populator",
					Namespace: "test",
					Labels:    map[string]string{kMigration: "miguid"},
				},
			}
			otherMig := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PopulatorPodPrefix + "other",
					Namespace: "test",
					Labels:    map[string]string{kMigration: "other"},
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), match)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), nonPrefix)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), otherMig)).To(Succeed())

			pods, err := kubevirt.getPopulatorPods()
			Expect(err).ToNot(HaveOccurred())
			Expect(pods).To(HaveLen(1))
			Expect(pods[0].Name).To(Equal(match.Name))

			Expect(kubevirt.DeletePopulatorPods(vm)).To(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: match.Name, Namespace: "test"}, &v1.Pod{})).ToNot(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: nonPrefix.Name, Namespace: "test"}, &v1.Pod{})).To(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: otherMig.Name, Namespace: "test"}, &v1.Pod{})).To(Succeed())
		})
	})

	ginkgo.Describe("cleanup helpers", func() {
		ginkgo.It("DeleteDataVolumes should delete all DVs labeled for the VM", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1"}}}

			labels := kubevirt.vmAllButMigrationLabels(vm.Ref)
			dv := &cdi.DataVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dv-1",
					Namespace: "test",
					Labels:    labels,
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), dv)).To(Succeed())
			Expect(kubevirt.DeleteDataVolumes(vm)).To(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "dv-1", Namespace: "test"}, &cdi.DataVolume{})).ToNot(Succeed())
		})

		ginkgo.It("DeleteJobs should delete jobs and their pods", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1"}}}

			labels := kubevirt.vmAllButMigrationLabels(vm.Ref)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job-1",
					Namespace: "test",
					Labels:    labels,
				},
			}
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job-1-pod",
					Namespace: "test",
					Labels:    map[string]string{"job-name": "job-1"},
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), job)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), pod)).To(Succeed())
			Expect(kubevirt.DeleteJobs(vm)).To(Succeed())

			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "job-1", Namespace: "test"}, &batchv1.Job{})).ToNot(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "job-1-pod", Namespace: "test"}, &v1.Pod{})).ToNot(Succeed())
		})

		ginkgo.It("DeleteSecret/DeleteConfigMap/DeleteVM should delete labeled objects", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1"}}}
			labels := kubevirt.vmAllButMigrationLabels(vm.Ref)

			sec := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "test", Labels: labels}}
			cm := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "test", Labels: labels}}
			kvm := &cnv.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vmobj", Namespace: "test", Labels: labels}}

			Expect(kubevirt.Destination.Create(context.TODO(), sec)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), cm)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), kvm)).To(Succeed())

			Expect(kubevirt.DeleteSecret(vm)).To(Succeed())
			Expect(kubevirt.DeleteConfigMap(vm)).To(Succeed())
			Expect(kubevirt.DeleteVM(vm)).To(Succeed())

			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "s1", Namespace: "test"}, &v1.Secret{})).ToNot(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "cm1", Namespace: "test"}, &v1.ConfigMap{})).ToNot(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "vmobj", Namespace: "test"}, &cnv.VirtualMachine{})).ToNot(Succeed())
		})

		ginkgo.It("GetPods should list pods for the VM labels", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1"}}}

			labels := kubevirt.vmAllButMigrationLabels(vm.Ref)
			p1 := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "test", Labels: labels}}
			p2 := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "test", Labels: labels}}
			Expect(kubevirt.Destination.Create(context.TODO(), p1)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), p2)).To(Succeed())

			list, err := kubevirt.GetPods(vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))
		})
	})

	ginkgo.Describe("transfer network", func() {
		ginkgo.It("vddkLabels should include use=vddk-conf", func() {
			kubevirt := createKubeVirtSpectro()
			Expect(kubevirt.vddkLabels()).To(HaveKeyWithValue(kUse, VddkConf))
		})

		ginkgo.It("setTransferNetwork should set modern selection annotation when route is present and valid", func() {
			kubevirt := createKubeVirtSpectro()
			kubevirt.Plan.Spec.TransferNetwork = &v1.ObjectReference{Name: "nad", Namespace: "ns"}

			nad := &k8snet.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nad",
					Namespace: "ns",
					Annotations: map[string]string{
						AnnForkliftNetworkRoute: "10.0.0.1",
					},
				},
			}
			Expect(kubevirt.Client.Create(context.TODO(), nad)).To(Succeed())

			ann := map[string]string{}
			Expect(kubevirt.setTransferNetwork(ann)).To(Succeed())
			Expect(ann).To(HaveKey(AnnTransferNetwork))

			var elems []k8snet.NetworkSelectionElement
			Expect(json.Unmarshal([]byte(ann[AnnTransferNetwork]), &elems)).To(Succeed())
			Expect(elems).To(HaveLen(1))
			Expect(elems[0].Name).To(Equal("nad"))
			Expect(elems[0].Namespace).To(Equal("ns"))
			Expect(elems[0].GatewayRequest).To(HaveLen(1))
			Expect(elems[0].GatewayRequest[0].String()).To(Equal("10.0.0.1"))
		})

		ginkgo.It("setTransferNetwork should fall back to legacy annotation when route is absent", func() {
			kubevirt := createKubeVirtSpectro()
			kubevirt.Plan.Spec.TransferNetwork = &v1.ObjectReference{Name: "nad", Namespace: "ns"}

			nad := &k8snet.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "nad",
					Namespace:   "ns",
					Annotations: map[string]string{
						// no route annotation
					},
				},
			}
			Expect(kubevirt.Client.Create(context.TODO(), nad)).To(Succeed())

			ann := map[string]string{}
			Expect(kubevirt.setTransferNetwork(ann)).To(Succeed())
			Expect(ann).To(HaveKeyWithValue(AnnLegacyTransferNetwork, "ns/nad"))
		})

		ginkgo.It("setTransferNetwork should error when route is invalid", func() {
			kubevirt := createKubeVirtSpectro()
			kubevirt.Plan.Spec.TransferNetwork = &v1.ObjectReference{Name: "nad", Namespace: "ns"}

			nad := &k8snet.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nad",
					Namespace: "ns",
					Annotations: map[string]string{
						AnnForkliftNetworkRoute: "not-an-ip",
					},
				},
			}
			Expect(kubevirt.Client.Create(context.TODO(), nad)).To(Succeed())

			ann := map[string]string{}
			Expect(kubevirt.setTransferNetwork(ann)).ToNot(Succeed())
		})
	})

	ginkgo.Describe("templates", func() {
		ginkgo.It("vmTemplate should select the newest matching template, process it, decode a VM, and sanitize it", func() {
			kubevirt := createKubeVirtSpectro()
			kubevirt.Builder = fakeBuilder{
				templateLabels: map[string]string{
					"os.template.kubevirt.io/rhel8.1": "true",
				},
			}

			// Make 2 templates with the same labels; ensure the newest wins.
			lbls := map[string]string{"os.template.kubevirt.io/rhel8.1": "true"}
			rawVM := []byte(`{"apiVersion":"kubevirt.io/v1","kind":"VirtualMachine","metadata":{"name":"tmpl","labels":{"x":"y"},"annotations":{"` + AnnKubevirtValidations + `":"something"}},"spec":{"template":{"spec":{"domain":{}}}}}`)
			u := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubevirt.io/v1",
					"kind":       "VirtualMachine",
					"metadata": map[string]interface{}{
						"name": "tmpl",
						"labels": map[string]interface{}{
							"x": "y",
						},
						"annotations": map[string]interface{}{
							AnnKubevirtValidations: "something",
						},
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"domain": map[string]interface{}{},
							},
						},
					},
				},
			}

			old := &templatev1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "old",
					Namespace:         "openshift",
					Labels:            lbls,
					CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
				},
				Objects: []runtime.RawExtension{{Raw: rawVM, Object: u}},
			}
			newer := &templatev1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "newer",
					Namespace:         "openshift",
					Labels:            lbls,
					CreationTimestamp: metav1.NewTime(time.Now()),
				},
				Parameters: []templatev1.Parameter{{Name: "NAME"}},
				Objects:    []runtime.RawExtension{{Raw: rawVM, Object: u}},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), old)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), newer)).To(Succeed())

			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1", Name: "myvm"}}}
			got, ok := kubevirt.vmTemplate(vm)
			Expect(ok).To(BeTrue())
			Expect(got).ToNot(BeNil())

			// Sanitization applied by vmTemplate.
			Expect(got.Name).To(Equal("myvm"))
			Expect(got.Namespace).To(Equal("test"))
			Expect(got.Spec.Template).ToNot(BeNil())
			Expect(got.Spec.Template.Spec.Volumes).To(BeEmpty())
			Expect(got.Spec.Template.Spec.Networks).To(BeEmpty())
			Expect(got.Spec.DataVolumeTemplates).To(BeEmpty())
			Expect(got.Annotations).ToNot(HaveKey(AnnKubevirtValidations))
			Expect(got.Labels).To(HaveKeyWithValue(kVM, "vm-1"))
		})

		ginkgo.It("decodeTemplate should error when template has no objects", func() {
			kubevirt := createKubeVirtSpectro()
			_, err := kubevirt.decodeTemplate(&templatev1.Template{})
			Expect(err).To(HaveOccurred())
		})
	})

	ginkgo.Describe("EnsureVM/virtualMachine", func() {
		mkTemplate := func(lbls map[string]string) *templatev1.Template {
			rawVM := []byte(`{"apiVersion":"kubevirt.io/v1","kind":"VirtualMachine","metadata":{"name":"tmpl","labels":{"x":"y"},"annotations":{"` + AnnKubevirtValidations + `":"something"}},"spec":{"template":{"spec":{"domain":{}}}}}`)
			u := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubevirt.io/v1",
					"kind":       "VirtualMachine",
					"metadata": map[string]interface{}{
						"name": "tmpl",
						"labels": map[string]interface{}{
							"x": "y",
						},
						"annotations": map[string]interface{}{
							AnnKubevirtValidations: "something",
						},
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"domain": map[string]interface{}{},
							},
						},
					},
				},
			}
			return &templatev1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "tmpl",
					Namespace:         "openshift",
					Labels:            lbls,
					CreationTimestamp: metav1.NewTime(time.Now()),
				},
				Parameters: []templatev1.Parameter{{Name: "NAME"}},
				Objects:    []runtime.RawExtension{{Raw: rawVM, Object: u}},
			}
		}

		ginkgo.It("EnsureVM should create VM when missing and patch PVC ownerRefs", func() {
			kubevirt := createKubeVirtSpectro()

			// Ensure setVmLabels doesn't nil-deref on referenced providers.
			convertDisk := true
			kubevirt.Plan.Provider.Source = &v1beta1.Provider{Spec: v1beta1.ProviderSpec{ConvertDisk: &convertDisk}}
			kubevirt.Plan.Provider.Destination = &v1beta1.Provider{}

			// Provide source provider type (Undefined is fine, it causes preference lookup to fail and fall back to template).
			kubevirt.Source.Provider = &v1beta1.Provider{}

			lbls := map[string]string{"os.template.kubevirt.io/rhel8.1": "true"}
			kubevirt.Builder = fakeBuilder{templateLabels: lbls}
			Expect(kubevirt.Destination.Create(context.TODO(), mkTemplate(lbls))).To(Succeed())

			vm := &planapi.VMStatus{
				VM: planapi.VM{Ref: ref.Ref{ID: "vm-1", Name: "orig"}},
			}
			vm.RestorePowerState = planapi.VMPowerStateOn
			vm.NewName = "renamed"

			// PVC that virtualMachine()/EnsureVM will patch.
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc1",
					Namespace: "test",
					Labels: map[string]string{
						"migration": string(kubevirt.Migration.UID),
						kVM:         vm.ID,
					},
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), pvc)).To(Succeed())

			Expect(kubevirt.EnsureVM(vm)).To(Succeed())

			// VM created with new name and run strategy Always.
			vmList := &cnv.VirtualMachineList{}
			Expect(kubevirt.Destination.List(context.TODO(), vmList)).To(Succeed())
			Expect(vmList.Items).To(HaveLen(1))
			Expect(vmList.Items[0].Name).To(Equal("renamed"))
			Expect(vmList.Items[0].Spec.RunStrategy).ToNot(BeNil())
			Expect(*vmList.Items[0].Spec.RunStrategy).To(Equal(cnv.RunStrategyAlways))
			Expect(vmList.Items[0].Annotations).To(HaveKeyWithValue(AnnDisplayName, "orig"))
			Expect(vmList.Items[0].Annotations).To(HaveKeyWithValue(AnnOriginalID, "vm-1"))

			// PVC patched with owner ref.
			gotPVC := &v1.PersistentVolumeClaim{}
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "pvc1", Namespace: "test"}, gotPVC)).To(Succeed())
			Expect(gotPVC.OwnerReferences).To(HaveLen(1))
			Expect(gotPVC.OwnerReferences[0].Kind).To(Equal("VirtualMachine"))
			Expect(gotPVC.OwnerReferences[0].Name).To(Equal("renamed"))
		})

		ginkgo.It("EnsureVM should use existing VM if present", func() {
			kubevirt := createKubeVirtSpectro()
			convertDisk := false
			kubevirt.Plan.Provider.Source = &v1beta1.Provider{Spec: v1beta1.ProviderSpec{ConvertDisk: &convertDisk}}
			kubevirt.Plan.Provider.Destination = &v1beta1.Provider{}
			kubevirt.Source.Provider = &v1beta1.Provider{}

			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1", Name: "n"}}}

			existing := &cnv.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing",
					Namespace: "test",
					Labels:    kubevirt.vmLabels(vm.Ref),
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), existing)).To(Succeed())

			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc1",
					Namespace: "test",
					Labels: map[string]string{
						"migration": string(kubevirt.Migration.UID),
						kVM:         vm.ID,
					},
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), pvc)).To(Succeed())

			Expect(kubevirt.EnsureVM(vm)).To(Succeed())

			// Existing VM should remain.
			vmList := &cnv.VirtualMachineList{}
			Expect(kubevirt.Destination.List(context.TODO(), vmList)).To(Succeed())
			Expect(vmList.Items).To(HaveLen(1))
			Expect(vmList.Items[0].Name).To(Equal("existing"))
		})
	})

	ginkgo.Describe("populator ownership + cleanup", func() {
		ginkgo.It("SetPopulatorPodOwnership should set PVC as ownerRef on matching populator pod", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1"}}}

			// Ensure Plan.Status.Migration.ActiveSnapshot().Migration.UID is set for getPopulatorPods().
			kubevirt.Plan.Status.Migration.History = []planapi.Snapshot{
				{Migration: planapi.SnapshotRef{UID: types.UID("miguid")}},
			}

			// PVC with UID that will match the populator pod suffix.
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc1",
					Namespace: "test",
					UID:       types.UID("pvcuid"),
					Labels: map[string]string{
						"migration": string(kubevirt.Migration.UID),
						kVM:         vm.ID,
					},
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), pvc)).To(Succeed())

			// Matching populator pod (name suffix equals pvc UID) + migration label.
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PopulatorPodPrefix + "pvcuid",
					Namespace: "test",
					Labels:    map[string]string{kMigration: "miguid"},
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), pod)).To(Succeed())

			Expect(kubevirt.SetPopulatorPodOwnership(vm)).To(Succeed())

			got := &v1.Pod{}
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: "test"}, got)).To(Succeed())
			Expect(got.OwnerReferences).To(HaveLen(1))
			Expect(got.OwnerReferences[0].Kind).To(Equal("PersistentVolumeClaim"))
			Expect(got.OwnerReferences[0].Name).To(Equal("pvc1"))
		})

		ginkgo.It("DeletePopulatedPVCs should delete prime + populated PVC and clear finalizers", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1"}}}

			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "pvc1",
					Namespace:  "test",
					UID:        types.UID("pvcuid"),
					Finalizers: []string{"finalizer.example"},
					Labels: map[string]string{
						"migration": string(kubevirt.Migration.UID),
						kVM:         vm.ID,
					},
				},
			}
			prime := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-pvcuid",
					Namespace: "test",
				},
			}
			Expect(kubevirt.Destination.Create(context.TODO(), pvc)).To(Succeed())
			Expect(kubevirt.Destination.Create(context.TODO(), prime)).To(Succeed())

			Expect(kubevirt.DeletePopulatedPVCs(vm)).To(Succeed())

			// Both should be deleted.
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "prime-pvcuid", Namespace: "test"}, &v1.PersistentVolumeClaim{})).ToNot(Succeed())
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: "pvc1", Namespace: "test"}, &v1.PersistentVolumeClaim{})).ToNot(Succeed())
		})
	})

	ginkgo.Describe("misc helpers", func() {
		ginkgo.It("emptyVm should set namespace/name/labels/template", func() {
			kubevirt := createKubeVirtSpectro()
			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1", Name: "vmname"}}}
			out := kubevirt.emptyVm(vm)
			Expect(out.Namespace).To(Equal("test"))
			Expect(out.Name).To(Equal("vmname"))
			Expect(out.Labels).To(HaveKeyWithValue(kVM, "vm-1"))
			Expect(out.Spec.Template).ToNot(BeNil())
		})

		ginkgo.It("isDataVolumeExistsInList should match by Builder stable identifier", func() {
			kubevirt := createKubeVirtSpectro()
			// Builder.ResolveDataVolumeIdentifier returns dv.Name in fakeBuilder
			kubevirt.Builder = fakeBuilder{}
			l := &cdi.DataVolumeList{Items: []cdi.DataVolume{{ObjectMeta: metav1.ObjectMeta{Name: "dv1"}}}}
			Expect(kubevirt.isDataVolumeExistsInList(&cdi.DataVolume{ObjectMeta: metav1.ObjectMeta{Name: "dv1"}}, l)).To(BeTrue())
			Expect(kubevirt.isDataVolumeExistsInList(&cdi.DataVolume{ObjectMeta: metav1.ObjectMeta{Name: "dv2"}}, l)).To(BeFalse())
		})
	})

	ginkgo.Describe("ensureSecret/ensureConfigMap/findConfigMapInNamespace", func() {
		ginkgo.It("findConfigMapInNamespace should return exists=false on NotFound and true when present", func() {
			kubevirt := createKubeVirtSpectro()
			cm, exists, err := kubevirt.findConfigMapInNamespace("missing", "test")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
			Expect(cm).To(BeNil())

			obj := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "present", Namespace: "test"}}
			Expect(kubevirt.Destination.Create(context.TODO(), obj)).To(Succeed())
			cm, exists, err = kubevirt.findConfigMapInNamespace("present", "test")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
			Expect(cm).ToNot(BeNil())
		})

		ginkgo.It("ensureConfigMap should create configmap when none exists and reuse when one exists", func() {
			kubevirt := createKubeVirtSpectro()
			kubevirt.Source.Inventory = stubInventory{}
			kubevirt.Source.Secret = &v1.Secret{Data: map[string][]byte{}}
			kubevirt.Builder = fakeBuilder{}
			vmRef := ref.Ref{ID: "vm-1"}

			cm1, err := kubevirt.ensureConfigMap(vmRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(cm1).ToNot(BeNil())

			// second call should reuse existing labeled configmap
			cm2, err := kubevirt.ensureConfigMap(vmRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(cm2.Name).To(Equal(cm1.Name))
		})

		ginkgo.It("ensureSecret should create then update secret.StringData", func() {
			kubevirt := createKubeVirtSpectro()
			kubevirt.Source.Inventory = stubInventory{}
			vmRef := ref.Ref{ID: "vm-1"}
			labels := kubevirt.vmLabels(vmRef)

			setter1 := func(s *v1.Secret) error {
				s.StringData = map[string]string{"a": "1"}
				return nil
			}
			sec, err := kubevirt.ensureSecret(vmRef, setter1, labels)
			Expect(err).ToNot(HaveOccurred())
			Expect(sec).ToNot(BeNil())

			// Update path (list finds existing secret by same labels).
			setter2 := func(s *v1.Secret) error {
				s.StringData = map[string]string{"a": "2", "b": "3"}
				return nil
			}
			sec2, err := kubevirt.ensureSecret(vmRef, setter2, labels)
			Expect(err).ToNot(HaveOccurred())
			Expect(sec2.Name).To(Equal(sec.Name))

			got := &v1.Secret{}
			Expect(kubevirt.Destination.Get(context.TODO(), types.NamespacedName{Name: sec.Name, Namespace: "test"}, got)).To(Succeed())
			Expect(got.StringData).To(HaveKeyWithValue("a", "2"))
			Expect(got.StringData).To(HaveKeyWithValue("b", "3"))
		})
	})

	ginkgo.Describe("libvirt configmap + pod mounts", func() {
		ginkgo.It("ensureLibvirtConfigMap should write input.xml based on VM volumes + PVC modes", func() {
			kubevirt := createKubeVirtSpectro()
			kubevirt.Source.Inventory = stubInventory{}
			kubevirt.Source.Secret = &v1.Secret{Data: map[string][]byte{}}
			kubevirt.Builder = fakeBuilder{}

			// Source provider type affects podVolumeMounts but not libvirtDomain.
			t := v1beta1.VSphere
			kubevirt.Source.Provider = &v1beta1.Provider{Spec: v1beta1.ProviderSpec{Type: &t}}

			vmRef := ref.Ref{ID: "vm-1"}

			block := v1.PersistentVolumeBlock
			fs := v1.PersistentVolumeFilesystem
			pvcBlock := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-block", Namespace: "test"}, Spec: v1.PersistentVolumeClaimSpec{VolumeMode: &block}}
			pvcFS := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-fs", Namespace: "test"}, Spec: v1.PersistentVolumeClaimSpec{VolumeMode: &fs}}

			vmCr := &VirtualMachine{
				VirtualMachine: &cnv.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: "test"},
					Spec: cnv.VirtualMachineSpec{
						Template: &cnv.VirtualMachineInstanceTemplateSpec{
							Spec: cnv.VirtualMachineInstanceSpec{
								Volumes: []cnv.Volume{
									{
										Name: "v0",
										VolumeSource: cnv.VolumeSource{
											PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-block"},
											},
										},
									},
									{
										Name: "v1",
										VolumeSource: cnv.VolumeSource{
											PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-fs"},
											},
										},
									},
									// missing PVC should be skipped
									{
										Name: "v2",
										VolumeSource: cnv.VolumeSource{
											PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{ClaimName: "missing"},
											},
										},
									},
								},
								Domain: cnv.DomainSpec{
									CPU: &cnv.CPU{Sockets: 2, Cores: 2},
									Resources: cnv.ResourceRequirements{
										Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("1Gi")},
									},
								},
							},
						},
					},
				},
			}

			cm, err := kubevirt.ensureLibvirtConfigMap(vmRef, vmCr, []*v1.PersistentVolumeClaim{pvcBlock, pvcFS})
			Expect(err).ToNot(HaveOccurred())
			Expect(cm).ToNot(BeNil())
			Expect(cm.BinaryData).To(HaveKey("input.xml"))
			xml := string(cm.BinaryData["input.xml"])
			Expect(xml).To(ContainSubstring("<domain"))
			Expect(xml).To(ContainSubstring("/dev/block0"))
			Expect(xml).To(ContainSubstring("/mnt/disks/disk1/disk.img"))
		})

		ginkgo.It("podVolumeMounts should create volumes/mounts/devices for mixed PVC modes + extra/vddk configmaps", func() {
			// Save/restore settings globals used by podVolumeMounts.
			prev := settings.Settings
			ginkgo.DeferCleanup(func() { settings.Settings = prev })

			kubevirt := createKubeVirtSpectro()
			kubevirt.Source.Inventory = stubInventory{}
			kubevirt.Source.Secret = &v1.Secret{Data: map[string][]byte{}}
			kubevirt.Builder = fakeBuilder{}

			// VSphere branch.
			t := v1beta1.VSphere
			kubevirt.Source.Provider = &v1beta1.Provider{Spec: v1beta1.ProviderSpec{Type: &t}}

			// Enable extra config map mount.
			settings.Settings.VirtV2vExtraConfConfigMap = "something"

			block := v1.PersistentVolumeBlock
			fs := v1.PersistentVolumeFilesystem
			pvcBlock := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-block", Namespace: "test"}, Spec: v1.PersistentVolumeClaimSpec{VolumeMode: &block}}
			pvcFS := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-fs", Namespace: "test"}, Spec: v1.PersistentVolumeClaimSpec{VolumeMode: &fs}}

			vmVolumes := []cnv.Volume{
				{Name: "v0", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-block"}}}},
				{Name: "v1", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-fs"}}}},
				{Name: "v2", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{ClaimName: "missing"}}}},
			}

			libvirtCM := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "libvirtcm", Namespace: "test"}}
			vddkCM := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "vddkcm", Namespace: "test"}}

			vm := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "vm-1"}}}
			vols, mounts, devices, err := kubevirt.podVolumeMounts(vmVolumes, libvirtCM, vddkCM, []*v1.PersistentVolumeClaim{pvcBlock, pvcFS}, vm)
			Expect(err).ToNot(HaveOccurred())

			// Should include disk volumes for found PVCs.
			Expect(vols).To(ContainElement(HaveField("Name", "pvc-block")))
			Expect(vols).To(ContainElement(HaveField("Name", "pvc-fs")))

			// Block mode should create a VolumeDevice; filesystem mode should create a mount.
			Expect(devices).To(ContainElement(HaveField("Name", "pvc-block")))
			Expect(mounts).To(ContainElement(HaveField("Name", "pvc-fs")))

			// Should mount libvirt xml + vddk vol mount path in VSphere branch.
			Expect(mounts).To(ContainElement(HaveField("Name", "libvirt-domain-xml")))
			Expect(mounts).To(ContainElement(HaveField("Name", VddkVolumeName)))

			// Extra/vddk configmaps should be present as volumes.
			Expect(vols).To(ContainElement(HaveField("Name", ExtraV2vConf)))
			Expect(vols).To(ContainElement(HaveField("Name", VddkConf)))
		})
	})

})

func createKubeVirtSpectro(objs ...runtime.Object) *KubeVirt {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	v1beta1.SchemeBuilder.AddToScheme(scheme)
	_ = cnv.AddToScheme(scheme)
	_ = cdi.AddToScheme(scheme)
	_ = k8snet.AddToScheme(scheme)
	_ = templatev1.AddToScheme(scheme)
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()
	plan := &v1beta1.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "plan",
			Namespace: "test",
			UID:       types.UID("plan-uid"),
		},
		Spec: v1beta1.PlanSpec{
			TargetNamespace: "test",
		},
	}
	return &KubeVirt{
		Context: &plancontext.Context{
			Destination: plancontext.Destination{
				Client: client,
			},
			Log:       KubeVirtLogSpectro,
			Migration: createMigrationSpectro(),
			Plan:      plan,
			Client:    client,
		},
	}
}

func createMigrationSpectro() *v1beta1.Migration {
	return &v1beta1.Migration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
			UID:       "test",
		},
	}
}
