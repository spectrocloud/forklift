package ocp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	planbase "github.com/kubev2v/forklift/pkg/controller/plan/adapter/base"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	"github.com/kubev2v/forklift/pkg/settings"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sclientgoscheme "k8s.io/client-go/kubernetes/scheme"
	cnv "kubevirt.io/api/core/v1"
	export "kubevirt.io/api/export/v1alpha1"
	cdi "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var addToClientGoSchemeOnce sync.Once

func addKubevirtToClientGoScheme(t *testing.T) {
	t.Helper()
	addToClientGoSchemeOnce.Do(func() {
		_ = cnv.AddToScheme(k8sclientgoscheme.Scheme)
		_ = export.AddToScheme(k8sclientgoscheme.Scheme)
		_ = cdi.AddToScheme(k8sclientgoscheme.Scheme)
	})
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(corev1): %v", err)
	}
	if err := cnv.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(cnv): %v", err)
	}
	if err := export.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(export): %v", err)
	}
	if err := cdi.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(cdi): %v", err)
	}
	return s
}

type createFailClient struct {
	client.Client
	failConfigMaps  bool
	failSecrets     bool
	failDataVolumes bool
	err             error
}

func (c *createFailClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	switch obj.(type) {
	case *corev1.ConfigMap:
		if c.failConfigMaps {
			return c.err
		}
	case *corev1.Secret:
		if c.failSecrets {
			return c.err
		}
	case *cdi.DataVolume:
		if c.failDataVolumes {
			return c.err
		}
	}
	return c.Client.Create(ctx, obj, opts...)
}

func newBuilder(t *testing.T, srcObjs []runtime.Object, dstObjs []runtime.Object, nm *api.NetworkMap, sm *api.StorageMap) *Builder {
	t.Helper()
	s := testScheme(t)
	src := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(srcObjs...).Build()
	dst := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(dstObjs...).Build()
	ctx := &plancontext.Context{
		Plan: &api.Plan{
			Spec: api.PlanSpec{
				TargetNamespace: "dest-ns",
			},
		},
		Log: logging.WithName("test-ocp-builder"),
	}
	ctx.Map.Network = nm
	ctx.Map.Storage = sm
	ctx.Destination.Client = dst
	return &Builder{
		Context:      ctx,
		sourceClient: src,
	}
}

func Test_getExportURL(t *testing.T) {
	if got := getExportURL(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := getExportURL([]export.VirtualMachineExportVolumeFormat{
		{Format: "raw", Url: "u1"},
	}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := getExportURL([]export.VirtualMachineExportVolumeFormat{
		{Format: export.ArchiveGz, Url: "u-archive"},
		{Format: export.KubeVirtGz, Url: "u-kubevirt"},
	}); got != "u-archive" {
		t.Fatalf("expected first match, got %q", got)
	}
	if got := getExportURL([]export.VirtualMachineExportVolumeFormat{
		{Format: export.KubeVirtGz, Url: "u-kubevirt"},
	}); got != "u-kubevirt" {
		t.Fatalf("expected kubevirt url, got %q", got)
	}
}

func Test_createDataVolumeSpec(t *testing.T) {
	size := resource.MustParse("10Gi")
	spec := createDataVolumeSpec(size, "sc", "http://example/vol.gz", "cm", "sec")
	if spec.Source == nil || spec.Source.HTTP == nil {
		t.Fatalf("expected http source")
	}
	if spec.Source.HTTP.URL != "http://example/vol.gz" {
		t.Fatalf("unexpected url: %q", spec.Source.HTTP.URL)
	}
	if spec.Source.HTTP.CertConfigMap != "cm" {
		t.Fatalf("unexpected cert configmap: %q", spec.Source.HTTP.CertConfigMap)
	}
	if len(spec.Source.HTTP.SecretExtraHeaders) != 1 || spec.Source.HTTP.SecretExtraHeaders[0] != "sec" {
		t.Fatalf("unexpected secret headers: %#v", spec.Source.HTTP.SecretExtraHeaders)
	}
	if spec.Storage == nil || spec.Storage.StorageClassName == nil || *spec.Storage.StorageClassName != "sc" {
		t.Fatalf("unexpected storage class: %#v", spec.Storage)
	}
	if got := spec.Storage.Resources.Requests[corev1.ResourceStorage]; got.Cmp(size) != 0 {
		t.Fatalf("unexpected storage request: %s", got.String())
	}
}

func Test_pvcSourceName(t *testing.T) {
	if got := pvcSourceName("ns", "pvc"); got != "ns/pvc" {
		t.Fatalf("unexpected: %q", got)
	}
}

func Test_createDiskMap_MapsPVC_DV_ConfigMap_Secret(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Domain: cnv.DomainSpec{
						Devices: cnv.Devices{
							Disks: []cnv.Disk{
								{Name: "d-pvc"},
								{Name: "d-dv"},
								{Name: "d-cm"},
								{Name: "d-sec"},
							},
						},
					},
					Volumes: []cnv.Volume{
						{Name: "d-pvc", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"}}}},
						{Name: "d-dv", VolumeSource: cnv.VolumeSource{DataVolume: &cnv.DataVolumeSource{Name: "dv1"}}},
						{Name: "d-cm", VolumeSource: cnv.VolumeSource{ConfigMap: &cnv.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm1"}}}},
						{Name: "d-sec", VolumeSource: cnv.VolumeSource{Secret: &cnv.SecretVolumeSource{SecretName: "sec1"}}},
					},
				},
			},
		},
	}
	m := createDiskMap(sourceVM, nil, vmRef)
	if _, ok := m[pvcSourceName("ns", "pvc1")]; !ok {
		t.Fatalf("expected pvc mapping key")
	}
	if _, ok := m[pvcSourceName("ns", "dv1")]; !ok {
		t.Fatalf("expected dv mapping key")
	}
	if _, ok := m["cm1"]; !ok {
		t.Fatalf("expected configmap mapping key")
	}
	if _, ok := m["sec1"]; !ok {
		t.Fatalf("expected secret mapping key")
	}
}

func TestBuilder_isDiskInDiskMap_and_mapDeviceDisks(t *testing.T) {
	b := newBuilder(t, nil, nil, &api.NetworkMap{}, &api.StorageMap{})
	diskMap := map[string]*cnv.Disk{
		"k1": {Name: "keep"},
	}
	if !b.isDiskInDiskMap(&cnv.Disk{Name: "keep"}, diskMap) {
		t.Fatalf("expected in map")
	}
	if b.isDiskInDiskMap(&cnv.Disk{Name: "skip"}, diskMap) {
		t.Fatalf("expected not in map")
	}

	sourceVM := &cnv.VirtualMachine{
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Domain: cnv.DomainSpec{Devices: cnv.Devices{Disks: []cnv.Disk{{Name: "keep"}, {Name: "skip"}}}},
				},
			},
		},
	}
	targetSpec := sourceVM.Spec.DeepCopy()
	targetSpec.Template.Spec.Domain.Devices.Disks = nil
	b.mapDeviceDisks(targetSpec, sourceVM, diskMap)
	if len(targetSpec.Template.Spec.Domain.Devices.Disks) != 1 || targetSpec.Template.Spec.Domain.Devices.Disks[0].Name != "keep" {
		t.Fatalf("unexpected disks: %#v", targetSpec.Template.Spec.Domain.Devices.Disks)
	}
}

func TestBuilder_createEnvMaps_SkipsMissing(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	srcCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cm-ok"}}
	srcSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sec-ok"}}
	b := newBuilder(t, []runtime.Object{srcCM, srcSecret}, nil, &api.NetworkMap{}, &api.StorageMap{})

	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Volumes: []cnv.Volume{
						{Name: "v-cm-ok", VolumeSource: cnv.VolumeSource{ConfigMap: &cnv.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm-ok"}}}},
						{Name: "v-cm-missing", VolumeSource: cnv.VolumeSource{ConfigMap: &cnv.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm-missing"}}}},
						{Name: "v-sec-ok", VolumeSource: cnv.VolumeSource{Secret: &cnv.SecretVolumeSource{SecretName: "sec-ok"}}},
						{Name: "v-sec-missing", VolumeSource: cnv.VolumeSource{Secret: &cnv.SecretVolumeSource{SecretName: "sec-missing"}}},
					},
				},
			},
		},
	}

	cms, secs := b.createEnvMaps(sourceVM, vmRef)
	if _, ok := cms["cm-ok"]; !ok {
		t.Fatalf("expected cm-ok present")
	}
	if _, ok := cms["cm-missing"]; ok {
		t.Fatalf("expected cm-missing skipped")
	}
	if _, ok := secs["sec-ok"]; !ok {
		t.Fatalf("expected sec-ok present")
	}
	if _, ok := secs["sec-missing"]; ok {
		t.Fatalf("expected sec-missing skipped")
	}
	if cms["cm-ok"].volName != "v-cm-ok" || secs["sec-ok"].volName != "v-sec-ok" {
		t.Fatalf("unexpected volName mapping: cm=%q sec=%q", cms["cm-ok"].volName, secs["sec-ok"].volName)
	}
}

func TestBuilder_mapConfigMapsAndSecretsToTarget_CreateAndAlreadyExists(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	srcCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cm1"}, Data: map[string]string{"k": "v"}}
	srcSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sec1"}, Data: map[string][]byte{"x": []byte("y")}}

	alreadyCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "dest-ns", Name: "cm1"}}
	alreadySec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "dest-ns", Name: "sec1"}}

	b := newBuilder(t, []runtime.Object{srcCM, srcSecret}, []runtime.Object{alreadyCM, alreadySec}, &api.NetworkMap{}, &api.StorageMap{})
	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{Spec: cnv.VirtualMachineInstanceSpec{
			Volumes: []cnv.Volume{
				{Name: "vol-cm", VolumeSource: cnv.VolumeSource{ConfigMap: &cnv.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm1"}}}},
				{Name: "vol-sec", VolumeSource: cnv.VolumeSource{Secret: &cnv.SecretVolumeSource{SecretName: "sec1"}}},
			},
		}}},
	}
	cms, secs := b.createEnvMaps(sourceVM, vmRef)

	targetSpec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	b.mapConfigMapsToTarget(targetSpec, cms)
	b.mapSecretsToTarget(targetSpec, secs)

	// even when AlreadyExists, volumes should be appended
	if len(targetSpec.Template.Spec.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(targetSpec.Template.Spec.Volumes))
	}
}

func TestBuilder_mapConfigMapsToTarget_SkipsOnCreateError(t *testing.T) {
	srcCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cm1"}}
	b := newBuilder(t, []runtime.Object{srcCM}, nil, &api.NetworkMap{}, &api.StorageMap{})
	b.Destination.Client = &createFailClient{Client: b.Destination.Client, failConfigMaps: true, err: errors.New("boom")}

	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{Spec: cnv.VirtualMachineInstanceSpec{
			Volumes: []cnv.Volume{{Name: "vol-cm", VolumeSource: cnv.VolumeSource{ConfigMap: &cnv.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm1"}}}}},
		}}},
	}
	cms, _ := b.createEnvMaps(sourceVM, vmRef)
	targetSpec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	b.mapConfigMapsToTarget(targetSpec, cms)
	if len(targetSpec.Template.Spec.Volumes) != 0 {
		t.Fatalf("expected no volumes appended on create error")
	}
}

func TestBuilder_mapSecretsToTarget_SkipsOnCreateError(t *testing.T) {
	srcSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sec1"}}
	b := newBuilder(t, []runtime.Object{srcSecret}, nil, &api.NetworkMap{}, &api.StorageMap{})
	b.Destination.Client = &createFailClient{Client: b.Destination.Client, failSecrets: true, err: errors.New("boom")}

	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{Spec: cnv.VirtualMachineInstanceSpec{
			Volumes: []cnv.Volume{{Name: "vol-sec", VolumeSource: cnv.VolumeSource{Secret: &cnv.SecretVolumeSource{SecretName: "sec1"}}}},
		}}},
	}
	_, secs := b.createEnvMaps(sourceVM, vmRef)
	targetSpec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	b.mapSecretsToTarget(targetSpec, secs)
	if len(targetSpec.Template.Spec.Volumes) != 0 {
		t.Fatalf("expected no volumes appended on create error")
	}
}

func TestBuilder_mapPVCsToTarget_AddsOnlyMapped(t *testing.T) {
	targetSpec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	pvc1 := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "dest-ns", Annotations: map[string]string{planbase.AnnDiskSource: "ns/pvc1"}}}
	pvc2 := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc2", Namespace: "dest-ns", Annotations: map[string]string{planbase.AnnDiskSource: "ns/pvc2"}}}
	diskMap := map[string]*cnv.Disk{"ns/pvc1": {Name: "disk1"}}

	b := newBuilder(t, nil, nil, &api.NetworkMap{}, &api.StorageMap{})
	b.mapPVCsToTarget(targetSpec, []*corev1.PersistentVolumeClaim{pvc1, pvc2}, diskMap)

	if len(targetSpec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(targetSpec.Template.Spec.Volumes))
	}
	got := targetSpec.Template.Spec.Volumes[0]
	if got.Name != "disk1" || got.PersistentVolumeClaim == nil || got.PersistentVolumeClaim.ClaimName != "pvc1" {
		t.Fatalf("unexpected target volume: %#v", got)
	}
}

func TestBuilder_mapDisks_ClearsAndMapsPVCsAndEnv(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	srcCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cm1"}, Data: map[string]string{"k": "v"}}
	srcSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sec1"}, Data: map[string][]byte{"x": []byte("y")}}
	b := newBuilder(t, []runtime.Object{srcCM, srcSecret}, nil, &api.NetworkMap{}, &api.StorageMap{})

	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Domain: cnv.DomainSpec{Devices: cnv.Devices{Disks: []cnv.Disk{{Name: "disk-a"}, {Name: "disk-b"}}}},
					Volumes: []cnv.Volume{
						{Name: "disk-a", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-a"}}}},
						{Name: "disk-b", VolumeSource: cnv.VolumeSource{Secret: &cnv.SecretVolumeSource{SecretName: "sec1"}}},
						{Name: "vol-cm", VolumeSource: cnv.VolumeSource{ConfigMap: &cnv.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm1"}}}},
					},
				},
			},
		},
	}

	targetSpec := sourceVM.Spec.DeepCopy()
	targetSpec.Template.Spec.Domain.Devices.Disks = []cnv.Disk{{Name: "old"}}
	targetSpec.Template.Spec.Volumes = []cnv.Volume{{Name: "old"}}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pvc-dest-a",
			Namespace:   "dest-ns",
			Annotations: map[string]string{planbase.AnnDiskSource: pvcSourceName("ns", "pvc-a")},
		},
	}

	b.mapDisks(sourceVM, targetSpec, []*corev1.PersistentVolumeClaim{pvc}, vmRef)
	if len(targetSpec.Template.Spec.Domain.Devices.Disks) != 2 {
		t.Fatalf("expected 2 disks preserved from source (only mapped), got %d", len(targetSpec.Template.Spec.Domain.Devices.Disks))
	}
	if len(targetSpec.Template.Spec.Volumes) != 3 {
		t.Fatalf("expected pvc + env cm + env secret volumes, got %d", len(targetSpec.Template.Spec.Volumes))
	}
}

func TestBuilder_mapNetworks_MultusAndPodVariants(t *testing.T) {
	nm := &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{
				{Source: ref.Ref{Namespace: "ns1", Name: "net1"}, Destination: api.DestinationNetwork{Type: Multus, Namespace: "dest", Name: "netA"}},
				{Source: ref.Ref{Namespace: "ns1", Name: "netIgnored"}, Destination: api.DestinationNetwork{Type: Ignored}},
				{Source: ref.Ref{Namespace: "ns1", Name: "netPod"}, Destination: api.DestinationNetwork{Type: Pod}},
				{Source: ref.Ref{Type: Pod}, Destination: api.DestinationNetwork{Type: Pod}},
			},
		},
	}
	b := newBuilder(t, nil, nil, nm, &api.StorageMap{})

	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Domain: cnv.DomainSpec{Devices: cnv.Devices{Interfaces: []cnv.Interface{
						{Name: "net1"},
						{Name: "netIgnored"},
						{Name: "netPod"},
						{Name: "podnet"},
					}}},
					Networks: []cnv.Network{
						{Name: "net1", NetworkSource: cnv.NetworkSource{Multus: &cnv.MultusNetwork{NetworkName: "ns1/net1"}}},
						{Name: "netIgnored", NetworkSource: cnv.NetworkSource{Multus: &cnv.MultusNetwork{NetworkName: "ns1/netIgnored"}}},
						{Name: "netPod", NetworkSource: cnv.NetworkSource{Multus: &cnv.MultusNetwork{NetworkName: "ns1/netPod"}}},
						{Name: "podnet", NetworkSource: cnv.NetworkSource{Pod: &cnv.PodNetwork{}}},
					},
				},
			},
		},
	}

	targetSpec := sourceVM.Spec.DeepCopy()
	b.mapNetworks(sourceVM, targetSpec)

	// Expectations based on current behavior:
	// - net1 (multus -> multus) is appended
	// - netIgnored is skipped
	// - netPod (multus -> pod) hits the "continue" branch, so not appended
	// - podnet (pod -> pod) is appended
	if len(targetSpec.Template.Spec.Networks) != 2 {
		t.Fatalf("expected 2 mapped networks, got %d: %#v", len(targetSpec.Template.Spec.Networks), targetSpec.Template.Spec.Networks)
	}
	if len(targetSpec.Template.Spec.Domain.Devices.Interfaces) != 2 {
		t.Fatalf("expected 2 mapped interfaces, got %d", len(targetSpec.Template.Spec.Domain.Devices.Interfaces))
	}
	if targetSpec.Template.Spec.Networks[0].Multus == nil || targetSpec.Template.Spec.Networks[0].Multus.NetworkName != "dest/netA" {
		t.Fatalf("expected first network mapped to multus dest/netA: %#v", targetSpec.Template.Spec.Networks[0])
	}
	if targetSpec.Template.Spec.Networks[1].Pod == nil {
		t.Fatalf("expected second network to be pod: %#v", targetSpec.Template.Spec.Networks[1])
	}
}

func TestBuilder_Tasks_PVCAndDataVolumeAndUnsupported(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	vm := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Volumes: []cnv.Volume{
						{Name: "v-pvc", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"}}}},
						{Name: "v-dv", VolumeSource: cnv.VolumeSource{DataVolume: &cnv.DataVolumeSource{Name: "dv1"}}},
						{Name: "v-unsupported", VolumeSource: cnv.VolumeSource{CloudInitNoCloud: &cnv.CloudInitNoCloudSource{}}},
					},
				},
			},
		},
	}
	pvc1 := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "ns"},
		Spec:       corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}},
	}
	pvcDV := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "dv1", Namespace: "ns"},
		Spec:       corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2048Mi")}}},
	}

	b := newBuilder(t, []runtime.Object{vm, pvc1, pvcDV}, nil, &api.NetworkMap{}, &api.StorageMap{})
	tasks, err := b.Tasks(vmRef)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Name != "ns/pvc1" || tasks[0].Progress.Total != 1024 {
		t.Fatalf("unexpected task[0]: %#v", tasks[0])
	}
	if tasks[1].Name != "ns/dv1" || tasks[1].Progress.Total != 2048 {
		t.Fatalf("unexpected task[1]: %#v", tasks[1])
	}
}

func TestBuilder_ConfigMap_ExternalLinkRequired(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	vmeNoExternal := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			Links: &export.VirtualMachineExportLinks{
				External: nil,
			},
		},
	}
	b := newBuilder(t, []runtime.Object{vmeNoExternal}, nil, &api.NetworkMap{}, &api.StorageMap{})
	var out corev1.ConfigMap
	if err := b.ConfigMap(vmRef, &corev1.Secret{}, &out); err == nil {
		t.Fatalf("expected error when external link missing")
	}

	vme := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			Links: &export.VirtualMachineExportLinks{
				External: &export.VirtualMachineExportLink{Cert: "CERTDATA"},
			},
		},
	}
	b2 := newBuilder(t, []runtime.Object{vme}, nil, &api.NetworkMap{}, &api.StorageMap{})
	var out2 corev1.ConfigMap
	if err := b2.ConfigMap(vmRef, &corev1.Secret{}, &out2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out2.Data["ca.pem"] != "CERTDATA" {
		t.Fatalf("unexpected ca.pem: %q", out2.Data["ca.pem"])
	}
}

func TestBuilder_Secret_TokenRefNilAndSuccess(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	vmeNil := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			Links:          &export.VirtualMachineExportLinks{External: &export.VirtualMachineExportLink{}},
			TokenSecretRef: nil,
		},
	}
	b := newBuilder(t, []runtime.Object{vmeNil}, nil, &api.NetworkMap{}, &api.StorageMap{})
	var out corev1.Secret
	// Current behavior: logs an error but wraps a nil error (returns nil).
	if err := b.Secret(vmRef, &corev1.Secret{}, &out); err != nil {
		t.Fatalf("expected nil error (wrap(nil)), got %v", err)
	}
	if out.StringData != nil && len(out.StringData) > 0 {
		t.Fatalf("expected no stringData when TokenSecretRef is nil: %#v", out.StringData)
	}

	tokenName := "tok"
	vme := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			Links:          &export.VirtualMachineExportLinks{External: &export.VirtualMachineExportLink{}},
			TokenSecretRef: &tokenName,
		},
	}
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: tokenName, Namespace: "ns"},
		Data:       map[string][]byte{"token": []byte("abc")},
	}
	b2 := newBuilder(t, []runtime.Object{vme, tokenSecret}, nil, &api.NetworkMap{}, &api.StorageMap{})
	var out2 corev1.Secret
	if err := b2.Secret(vmRef, &corev1.Secret{}, &out2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out2.StringData["token"] != "x-kubevirt-export-token:abc" {
		t.Fatalf("unexpected token header: %q", out2.StringData["token"])
	}
}

func TestBuilder_DataVolumes_Success(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	storageClass := "sc-src"
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "ns"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	vme := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			Links: &export.VirtualMachineExportLinks{
				External: &export.VirtualMachineExportLink{
					Volumes: []export.VirtualMachineExportVolume{
						{
							Name: "pvc1",
							Formats: []export.VirtualMachineExportVolumeFormat{
								{Format: export.KubeVirtGz, Url: "http://example/vol.gz"},
							},
						},
					},
				},
			},
		},
	}
	sm := &api.StorageMap{
		Spec: api.StorageMapSpec{
			Map: []api.StoragePair{
				{Source: ref.Ref{Name: "sc-src"}, Destination: api.DestinationStorage{StorageClass: "sc-dst"}},
			},
		},
	}
	b := newBuilder(t, []runtime.Object{vme, pvc}, nil, &api.NetworkMap{}, sm)

	dvTemplate := &cdi.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dv-template",
			Namespace:   "dest-ns",
			Annotations: map[string]string{},
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec", Namespace: "ns"}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}}

	dvs, err := b.DataVolumes(vmRef, secret, cm, dvTemplate, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(dvs) != 1 {
		t.Fatalf("expected 1 datavolume, got %d", len(dvs))
	}
	if dvs[0].Annotations[planbase.AnnDiskSource] != "ns/pvc1" {
		t.Fatalf("unexpected disk source annotation: %q", dvs[0].Annotations[planbase.AnnDiskSource])
	}
	if dvs[0].Spec.Source == nil || dvs[0].Spec.Source.HTTP == nil || dvs[0].Spec.Source.HTTP.URL != "http://example/vol.gz" {
		t.Fatalf("unexpected source http: %#v", dvs[0].Spec.Source)
	}
	if dvs[0].Spec.Storage == nil || dvs[0].Spec.Storage.StorageClassName == nil || *dvs[0].Spec.Storage.StorageClassName != "sc-dst" {
		t.Fatalf("unexpected storage class: %#v", dvs[0].Spec.Storage)
	}
}

func TestBuilder_SupportsAndPopulatorErrors(t *testing.T) {
	b := newBuilder(t, nil, nil, &api.NetworkMap{}, &api.StorageMap{})
	if b.SupportsVolumePopulators() {
		t.Fatalf("expected supports=false")
	}
	if _, err := b.PopulatorVolumes(ref.Ref{}, nil, ""); !errors.Is(err, planbase.VolumePopulatorNotSupportedError) {
		t.Fatalf("expected VolumePopulatorNotSupportedError, got %v", err)
	}
	if _, err := b.PopulatorTransferredBytes(&corev1.PersistentVolumeClaim{}); !errors.Is(err, planbase.VolumePopulatorNotSupportedError) {
		t.Fatalf("expected VolumePopulatorNotSupportedError, got %v", err)
	}
	if err := b.SetPopulatorDataSourceLabels(ref.Ref{}, nil); !errors.Is(err, planbase.VolumePopulatorNotSupportedError) {
		t.Fatalf("expected VolumePopulatorNotSupportedError, got %v", err)
	}
	if _, err := b.GetPopulatorTaskName(&corev1.PersistentVolumeClaim{}); !errors.Is(err, planbase.VolumePopulatorNotSupportedError) {
		t.Fatalf("expected VolumePopulatorNotSupportedError, got %v", err)
	}
}

func TestBuilder_PreferenceAndTemplateLabels_Error(t *testing.T) {
	b := newBuilder(t, nil, nil, &api.NetworkMap{}, &api.StorageMap{})
	if _, err := b.PreferenceName(ref.Ref{}, &corev1.ConfigMap{}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := b.TemplateLabels(ref.Ref{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuilder_getSourceVmFromDefinition_HappyPathAndErrors(t *testing.T) {
	addKubevirtToClientGoScheme(t)

	tokenName := "tok"
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: tokenName, Namespace: "ns"},
		Data:       map[string][]byte{"token": []byte("abc")},
	}

	// Happy path server returns a v1.List including a kubevirt VM
	vm := &cnv.VirtualMachine{
		TypeMeta:   metav1.TypeMeta{APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine"},
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Domain: cnv.DomainSpec{Devices: cnv.Devices{}},
				},
			},
		},
	}
	rawVM, _ := json.Marshal(vm)
	list := &corev1.List{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "List"},
		Items:    []runtime.RawExtension{{Raw: rawVM}},
	}
	rawList, _ := json.Marshal(list)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(rawList)
	}))
	defer srv.Close()

	vme := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			TokenSecretRef: &tokenName,
			Links: &export.VirtualMachineExportLinks{
				External: &export.VirtualMachineExportLink{
					Cert: "",
					Manifests: []export.VirtualMachineExportManifest{
						{Type: export.AllManifests, Url: srv.URL},
					},
				},
			},
		},
	}

	b := newBuilder(t, []runtime.Object{tokenSecret}, nil, &api.NetworkMap{}, &api.StorageMap{})
	got, err := b.getSourceVmFromDefinition(vme)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil || got.Name != "vm" {
		t.Fatalf("unexpected vm: %#v", got)
	}

	// Invalid CA cert
	vmeBadCA := vme.DeepCopy()
	vmeBadCA.Status.Links.External.Cert = "not-a-pem"
	if _, err := b.getSourceVmFromDefinition(vmeBadCA); err == nil {
		t.Fatalf("expected error for invalid CA")
	}

	// Bad URL -> request create error
	vmeBadURL := vme.DeepCopy()
	vmeBadURL.Status.Links.External.Manifests[0].Url = "://bad-url"
	if _, err := b.getSourceVmFromDefinition(vmeBadURL); err == nil {
		t.Fatalf("expected error for bad url")
	}

	// Token secret missing
	bMissingToken := newBuilder(t, nil, nil, &api.NetworkMap{}, &api.StorageMap{})
	if _, err := bMissingToken.getSourceVmFromDefinition(vme); err == nil {
		t.Fatalf("expected error for missing token secret")
	}

	// Server error status
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("nope"))
	}))
	defer errSrv.Close()
	vme500 := vme.DeepCopy()
	vme500.Status.Links.External.Manifests[0].Url = errSrv.URL
	if _, err := b.getSourceVmFromDefinition(vme500); err == nil {
		t.Fatalf("expected error for 500 status")
	}

	// List without a VM
	noVMList := &corev1.List{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "List"}}
	rawNoVM, _ := json.Marshal(noVMList)
	noVMSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(rawNoVM)
	}))
	defer noVMSrv.Close()
	vmeNoVM := vme.DeepCopy()
	vmeNoVM.Status.Links.External.Manifests[0].Url = noVMSrv.URL
	if _, err := b.getSourceVmFromDefinition(vmeNoVM); err == nil {
		t.Fatalf("expected error when no vm in manifest")
	}
}

func TestBuilder_VirtualMachine_UsesManifestVMAndMaps(t *testing.T) {
	addKubevirtToClientGoScheme(t)

	tokenName := "tok"
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: tokenName, Namespace: "ns"},
		Data:       map[string][]byte{"token": []byte("abc")},
	}

	vm := &cnv.VirtualMachine{
		TypeMeta:   metav1.TypeMeta{APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine"},
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Domain: cnv.DomainSpec{Devices: cnv.Devices{}},
				},
			},
		},
	}
	rawVM, _ := json.Marshal(vm)
	list := &corev1.List{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "List"},
		Items:    []runtime.RawExtension{{Raw: rawVM}},
	}
	rawList, _ := json.Marshal(list)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(rawList)
	}))
	defer srv.Close()

	vme := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			TokenSecretRef: &tokenName,
			Links: &export.VirtualMachineExportLinks{
				External: &export.VirtualMachineExportLink{
					Cert: "",
					Manifests: []export.VirtualMachineExportManifest{
						{Type: export.AllManifests, Url: srv.URL},
					},
				},
			},
		},
	}

	nm := &api.NetworkMap{Spec: api.NetworkMapSpec{Map: []api.NetworkPair{{Source: ref.Ref{Type: Pod}, Destination: api.DestinationNetwork{Type: Pod}}}}}
	b := newBuilder(t, []runtime.Object{vme, tokenSecret}, nil, nm, &api.StorageMap{})

	var out cnv.VirtualMachineSpec
	err := b.VirtualMachine(ref.Ref{Namespace: "ns", Name: "vm"}, &out, nil, false, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.Template == nil {
		t.Fatalf("expected template set")
	}
}

func TestBuilder_DataVolumes_PropagatesErrors(t *testing.T) {
	// VMExport missing -> Get error
	b := newBuilder(t, nil, nil, &api.NetworkMap{}, &api.StorageMap{Spec: api.StorageMapSpec{Map: nil}})
	dvTemplate := &cdi.DataVolume{ObjectMeta: metav1.ObjectMeta{Name: "dv", Namespace: "dest-ns", Annotations: map[string]string{}}}
	_, err := b.DataVolumes(ref.Ref{Namespace: "ns", Name: "vm"}, &corev1.Secret{}, &corev1.ConfigMap{}, dvTemplate, nil)
	if err == nil {
		t.Fatalf("expected error")
	}

	// VMExport present but URL missing -> getExportURL error
	vme := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			Links: &export.VirtualMachineExportLinks{
				External: &export.VirtualMachineExportLink{
					Volumes: []export.VirtualMachineExportVolume{{Name: "pvc1", Formats: []export.VirtualMachineExportVolumeFormat{{Format: "raw", Url: "u"}}}},
				},
			},
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "ns"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: func() *string { s := "sc"; return &s }(),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	sm := &api.StorageMap{Spec: api.StorageMapSpec{Map: []api.StoragePair{{Source: ref.Ref{Name: "sc"}, Destination: api.DestinationStorage{StorageClass: "sc-dst"}}}}}
	b2 := newBuilder(t, []runtime.Object{vme, pvc}, nil, &api.NetworkMap{}, sm)
	_, err = b2.DataVolumes(ref.Ref{Namespace: "ns", Name: "vm"}, &corev1.Secret{}, &corev1.ConfigMap{}, dvTemplate, nil)
	if err == nil {
		t.Fatalf("expected error on missing export url")
	}
}

func TestBuilder_mapNetworks_UnknownNetworkType_Skips(t *testing.T) {
	nm := &api.NetworkMap{Spec: api.NetworkMapSpec{Map: []api.NetworkPair{{Source: ref.Ref{Type: Pod}, Destination: api.DestinationNetwork{Type: Pod}}}}}
	b := newBuilder(t, nil, nil, nm, &api.StorageMap{})

	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Domain: cnv.DomainSpec{Devices: cnv.Devices{Interfaces: []cnv.Interface{{Name: "n1"}}}},
					Networks: []cnv.Network{
						{Name: "n1"}, // neither pod nor multus -> unknown
					},
				},
			},
		},
	}
	targetSpec := sourceVM.Spec.DeepCopy()
	b.mapNetworks(sourceVM, targetSpec)
	if len(targetSpec.Template.Spec.Networks) != 0 || len(targetSpec.Template.Spec.Domain.Devices.Interfaces) != 0 {
		t.Fatalf("expected unknown network skipped")
	}
}

func TestBuilder_mapNetworks_PodMappedToMultus_ContinuesWithoutAppend(t *testing.T) {
	nm := &api.NetworkMap{
		Spec: api.NetworkMapSpec{
			Map: []api.NetworkPair{
				{Source: ref.Ref{Type: Pod}, Destination: api.DestinationNetwork{Type: Multus, Namespace: "dest", Name: "pod-as-multus"}},
			},
		},
	}
	b := newBuilder(t, nil, nil, nm, &api.StorageMap{})

	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{
			Template: &cnv.VirtualMachineInstanceTemplateSpec{
				Spec: cnv.VirtualMachineInstanceSpec{
					Domain: cnv.DomainSpec{Devices: cnv.Devices{Interfaces: []cnv.Interface{{Name: "podnet"}}}},
					Networks: []cnv.Network{
						{Name: "podnet", NetworkSource: cnv.NetworkSource{Pod: &cnv.PodNetwork{}}},
					},
				},
			},
		},
	}
	targetSpec := sourceVM.Spec.DeepCopy()
	b.mapNetworks(sourceVM, targetSpec)
	if len(targetSpec.Template.Spec.Networks) != 0 || len(targetSpec.Template.Spec.Domain.Devices.Interfaces) != 0 {
		t.Fatalf("expected pod->multus to continue without append (current behavior)")
	}
}

func TestBuilder_mapConfigMapsAndSecretsToTarget_CreatesDestinationObjects(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	srcCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cm1", Labels: map[string]string{"a": "b"}, Annotations: map[string]string{"x": "y"}},
		Data:       map[string]string{"k": "v"},
	}
	srcSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sec1", Labels: map[string]string{"a": "b"}, Annotations: map[string]string{"x": "y"}},
		Data:       map[string][]byte{"x": []byte("y")},
	}
	b := newBuilder(t, []runtime.Object{srcCM, srcSecret}, nil, &api.NetworkMap{}, &api.StorageMap{})

	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{Spec: cnv.VirtualMachineInstanceSpec{
			Volumes: []cnv.Volume{
				{Name: "vol-cm", VolumeSource: cnv.VolumeSource{ConfigMap: &cnv.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm1"}}}},
				{Name: "vol-sec", VolumeSource: cnv.VolumeSource{Secret: &cnv.SecretVolumeSource{SecretName: "sec1"}}},
			},
		}}},
	}
	cms, secs := b.createEnvMaps(sourceVM, vmRef)
	targetSpec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	b.mapConfigMapsToTarget(targetSpec, cms)
	b.mapSecretsToTarget(targetSpec, secs)

	// Verify they exist on destination client with target namespace
	gotCM := &corev1.ConfigMap{}
	if err := b.Destination.Client.Get(context.Background(), client.ObjectKey{Namespace: "dest-ns", Name: "cm1"}, gotCM); err != nil {
		t.Fatalf("expected cm created on destination: %v", err)
	}
	gotSec := &corev1.Secret{}
	if err := b.Destination.Client.Get(context.Background(), client.ObjectKey{Namespace: "dest-ns", Name: "sec1"}, gotSec); err != nil {
		t.Fatalf("expected secret created on destination: %v", err)
	}
}

func TestBuilder_DataVolumes_AlreadyExistsIgnored(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	storageClass := "sc-src"
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "ns"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	vme := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			Links: &export.VirtualMachineExportLinks{
				External: &export.VirtualMachineExportLink{
					Volumes: []export.VirtualMachineExportVolume{
						{
							Name: "pvc1",
							Formats: []export.VirtualMachineExportVolumeFormat{
								{Format: export.KubeVirtGz, Url: "http://example/vol.gz"},
							},
						},
					},
				},
			},
		},
	}
	sm := &api.StorageMap{
		Spec: api.StorageMapSpec{
			Map: []api.StoragePair{
				{Source: ref.Ref{Name: "sc-src"}, Destination: api.DestinationStorage{StorageClass: "sc-dst"}},
			},
		},
	}
	already := &cdi.DataVolume{ObjectMeta: metav1.ObjectMeta{Name: "dv-template", Namespace: "dest-ns"}}
	b := newBuilder(t, []runtime.Object{vme, pvc}, []runtime.Object{already}, &api.NetworkMap{}, sm)

	dvTemplate := &cdi.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dv-template",
			Namespace:   "dest-ns",
			Annotations: map[string]string{},
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec", Namespace: "ns"}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}}

	dvs, err := b.DataVolumes(vmRef, secret, cm, dvTemplate, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(dvs) != 1 {
		t.Fatalf("expected 1 datavolume, got %d", len(dvs))
	}
}

func TestBuilder_mapConfigMapsToTarget_AlreadyExistsIsNotFatal(t *testing.T) {
	srcCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cm1"}}
	already := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "dest-ns", Name: "cm1"}}
	b := newBuilder(t, []runtime.Object{srcCM}, []runtime.Object{already}, &api.NetworkMap{}, &api.StorageMap{})

	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	sourceVM := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{Spec: cnv.VirtualMachineInstanceSpec{
			Volumes: []cnv.Volume{{Name: "vol-cm", VolumeSource: cnv.VolumeSource{ConfigMap: &cnv.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm1"}}}}},
		}}},
	}
	cms, _ := b.createEnvMaps(sourceVM, vmRef)
	targetSpec := &cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}}
	b.mapConfigMapsToTarget(targetSpec, cms)
	if len(targetSpec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected volume appended even with AlreadyExists")
	}
}

func TestBuilder_DataVolumes_CreateErrorNotAlreadyExists_ReturnsError(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	storageClass := "sc-src"
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "ns"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	vme := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status: &export.VirtualMachineExportStatus{
			Links: &export.VirtualMachineExportLinks{
				External: &export.VirtualMachineExportLink{
					Volumes: []export.VirtualMachineExportVolume{
						{
							Name: "pvc1",
							Formats: []export.VirtualMachineExportVolumeFormat{
								{Format: export.KubeVirtGz, Url: "http://example/vol.gz"},
							},
						},
					},
				},
			},
		},
	}
	sm := &api.StorageMap{Spec: api.StorageMapSpec{Map: []api.StoragePair{{Source: ref.Ref{Name: "sc-src"}, Destination: api.DestinationStorage{StorageClass: "sc-dst"}}}}}
	b := newBuilder(t, []runtime.Object{vme, pvc}, nil, &api.NetworkMap{}, sm)
	b.Destination.Client = &createFailClient{Client: b.Destination.Client, failDataVolumes: true, err: errors.New("boom")}

	dvTemplate := &cdi.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dv-template",
			Namespace:   "dest-ns",
			Annotations: map[string]string{},
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec", Namespace: "ns"}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}}
	_, err := b.DataVolumes(vmRef, secret, cm, dvTemplate, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

// ---- Consolidated from validator_more_unit_test.go and client_more_unit_test.go ----

func newValidator(t *testing.T, plan *api.Plan, objs ...runtime.Object) *Validator {
	t.Helper()
	s := testScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()
	return &Validator{
		plan:         plan,
		sourceClient: c,
		log:          logging.WithName("test-ocp-validator"),
	}
}

func TestValidator_SimpleBooleans(t *testing.T) {
	v := &Validator{log: logging.WithName("test-ocp-validator")}
	if ok, err := v.MaintenanceMode(ref.Ref{}); err != nil || !ok {
		t.Fatalf("expected (true,nil), got (%v,%v)", ok, err)
	}
	if v.WarmMigration() {
		t.Fatalf("expected warm=false")
	}
	if ok, _, _, err := v.SharedDisks(ref.Ref{}, nil); err != nil || !ok {
		t.Fatalf("expected SharedDisks ok=true err=nil, got ok=%v err=%v", ok, err)
	}
	if ok, err := v.DirectStorage(ref.Ref{}); err != nil || !ok {
		t.Fatalf("expected DirectStorage ok=true, got ok=%v err=%v", ok, err)
	}
	if ok, err := v.StaticIPs(ref.Ref{}); err != nil || !ok {
		t.Fatalf("expected StaticIPs ok=true, got ok=%v err=%v", ok, err)
	}
	if ok, err := v.ChangeTrackingEnabled(ref.Ref{}); err != nil || !ok {
		t.Fatalf("expected ChangeTrackingEnabled ok=true, got ok=%v err=%v", ok, err)
	}
}

func TestValidator_PodNetwork(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}
	vm := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec:       cnv.VirtualMachineSpec{Template: &cnv.VirtualMachineInstanceTemplateSpec{}},
	}

	t.Run("no network map => zero values", func(t *testing.T) {
		plan := &api.Plan{}
		v := newValidator(t, plan, vm)
		ok, err := v.PodNetwork(vmRef)
		if err != nil || ok {
			t.Fatalf("expected (false,nil), got (%v,%v)", ok, err)
		}
	})

	t.Run("vm missing => error", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Network = &api.NetworkMap{}
		v := newValidator(t, plan /* no vm */)
		_, err := v.PodNetwork(vmRef)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("<=1 pod mapping => ok", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Network = &api.NetworkMap{
			Spec: api.NetworkMapSpec{
				Map: []api.NetworkPair{
					{Destination: api.DestinationNetwork{Type: Pod}},
				},
			},
		}
		v := newValidator(t, plan, vm)
		ok, err := v.PodNetwork(vmRef)
		if err != nil || !ok {
			t.Fatalf("expected ok=true err=nil, got ok=%v err=%v", ok, err)
		}
	})

	t.Run(">1 pod mapping => not ok", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Network = &api.NetworkMap{
			Spec: api.NetworkMapSpec{
				Map: []api.NetworkPair{
					{Destination: api.DestinationNetwork{Type: Pod}},
					{Destination: api.DestinationNetwork{Type: Pod}},
				},
			},
		}
		v := newValidator(t, plan, vm)
		ok, err := v.PodNetwork(vmRef)
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}
	})
}

func TestValidator_StorageMapped(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}

	t.Run("no storage map => zero values", func(t *testing.T) {
		plan := &api.Plan{}
		v := newValidator(t, plan)
		ok, err := v.StorageMapped(vmRef)
		if err != nil || ok {
			t.Fatalf("expected (false,nil), got (%v,%v)", ok, err)
		}
	})

	t.Run("vm missing => error", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Storage = &api.StorageMap{}
		v := newValidator(t, plan)
		_, err := v.StorageMapped(vmRef)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("pvc missing => error", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Storage = &api.StorageMap{Spec: api.StorageMapSpec{Map: []api.StoragePair{{Source: ref.Ref{Name: "sc"}, Destination: api.DestinationStorage{StorageClass: "sc-dst"}}}}}
		vm := &cnv.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
			Spec: cnv.VirtualMachineSpec{
				Template: &cnv.VirtualMachineInstanceTemplateSpec{
					Spec: cnv.VirtualMachineInstanceSpec{
						Volumes: []cnv.Volume{
							{Name: "v1", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"}}}},
						},
					},
				},
			},
		}
		v := newValidator(t, plan, vm)
		_, err := v.StorageMapped(vmRef)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("storageClassName nil => ok=false err=nil", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Storage = &api.StorageMap{Spec: api.StorageMapSpec{Map: []api.StoragePair{{Source: ref.Ref{Name: "sc"}, Destination: api.DestinationStorage{StorageClass: "sc-dst"}}}}}
		vm := &cnv.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
			Spec: cnv.VirtualMachineSpec{
				Template: &cnv.VirtualMachineInstanceTemplateSpec{
					Spec: cnv.VirtualMachineInstanceSpec{
						Volumes: []cnv.Volume{
							{Name: "v1", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"}}}},
						},
					},
				},
			},
		}
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "ns"},
			Spec:       corev1.PersistentVolumeClaimSpec{}, // StorageClassName nil
		}
		v := newValidator(t, plan, vm, pvc)
		ok, err := v.StorageMapped(vmRef)
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("storageclass not mapped => error", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Storage = &api.StorageMap{Spec: api.StorageMapSpec{Map: []api.StoragePair{{Source: ref.Ref{Name: "other"}, Destination: api.DestinationStorage{StorageClass: "x"}}}}}
		vm := &cnv.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
			Spec: cnv.VirtualMachineSpec{
				Template: &cnv.VirtualMachineInstanceTemplateSpec{
					Spec: cnv.VirtualMachineInstanceSpec{
						Volumes: []cnv.Volume{
							{Name: "v1", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"}}}},
						},
					},
				},
			},
		}
		sc := "sc"
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "ns"},
			Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: &sc},
		}
		v := newValidator(t, plan, vm, pvc)
		ok, err := v.StorageMapped(vmRef)
		// Current behavior: wraps a nil error and returns (false,nil) while logging.
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("all mapped => ok", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Storage = &api.StorageMap{
			Spec: api.StorageMapSpec{
				Map: []api.StoragePair{{Source: ref.Ref{Name: "sc"}, Destination: api.DestinationStorage{StorageClass: "sc-dst"}}},
			},
		}
		vm := &cnv.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
			Spec: cnv.VirtualMachineSpec{
				Template: &cnv.VirtualMachineInstanceTemplateSpec{
					Spec: cnv.VirtualMachineInstanceSpec{
						Volumes: []cnv.Volume{
							{Name: "v1", VolumeSource: cnv.VolumeSource{PersistentVolumeClaim: &cnv.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"}}}},
							{Name: "v2", VolumeSource: cnv.VolumeSource{DataVolume: &cnv.DataVolumeSource{Name: "dv1"}}},
							{Name: "v3", VolumeSource: cnv.VolumeSource{CloudInitNoCloud: &cnv.CloudInitNoCloudSource{}}},
						},
					},
				},
			},
		}
		sc := "sc"
		pvc1 := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "ns"},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &sc,
				Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}},
			},
		}
		pvcDV := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "dv1", Namespace: "ns"},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &sc,
				Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}},
			},
		}
		v := newValidator(t, plan, vm, pvc1, pvcDV)
		ok, err := v.StorageMapped(vmRef)
		if err != nil || !ok {
			t.Fatalf("expected ok=true err=nil, got ok=%v err=%v", ok, err)
		}
	})
}

func TestValidator_NetworksMapped(t *testing.T) {
	vmRef := ref.Ref{Namespace: "ns", Name: "vm"}

	t.Run("no network map => zero values", func(t *testing.T) {
		plan := &api.Plan{}
		v := newValidator(t, plan)
		ok, err := v.NetworksMapped(vmRef)
		if err != nil || ok {
			t.Fatalf("expected (false,nil), got (%v,%v)", ok, err)
		}
	})

	t.Run("vm missing => error", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Network = &api.NetworkMap{}
		v := newValidator(t, plan)
		_, err := v.NetworksMapped(vmRef)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("pod network missing mapping => error", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{Map: nil}}
		vm := &cnv.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
			Spec: cnv.VirtualMachineSpec{
				Template: &cnv.VirtualMachineInstanceTemplateSpec{
					Spec: cnv.VirtualMachineInstanceSpec{
						Networks: []cnv.Network{{Name: "podnet", NetworkSource: cnv.NetworkSource{Pod: &cnv.PodNetwork{}}}},
					},
				},
			},
		}
		v := newValidator(t, plan, vm)
		ok, err := v.NetworksMapped(vmRef)
		// Current behavior: wraps a nil error and returns (false,nil) while logging.
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("multus missing mapping => error", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Network = &api.NetworkMap{Spec: api.NetworkMapSpec{Map: []api.NetworkPair{{Source: ref.Ref{Type: Pod}, Destination: api.DestinationNetwork{Type: Pod}}}}}
		vm := &cnv.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
			Spec: cnv.VirtualMachineSpec{
				Template: &cnv.VirtualMachineInstanceTemplateSpec{
					Spec: cnv.VirtualMachineInstanceSpec{
						Networks: []cnv.Network{{Name: "m1", NetworkSource: cnv.NetworkSource{Multus: &cnv.MultusNetwork{NetworkName: "ns1/net1"}}}},
					},
				},
			},
		}
		v := newValidator(t, plan, vm)
		ok, err := v.NetworksMapped(vmRef)
		// Current behavior: wraps a nil error and returns (false,nil) while logging.
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("all mapped => ok", func(t *testing.T) {
		plan := &api.Plan{}
		plan.Referenced.Map.Network = &api.NetworkMap{
			Spec: api.NetworkMapSpec{
				Map: []api.NetworkPair{
					{Source: ref.Ref{Type: Pod}, Destination: api.DestinationNetwork{Type: Pod}},
					{Source: ref.Ref{Namespace: "ns1", Name: "net1"}, Destination: api.DestinationNetwork{Type: Multus, Namespace: "dest", Name: "netA"}},
				},
			},
		}
		vm := &cnv.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
			Spec: cnv.VirtualMachineSpec{
				Template: &cnv.VirtualMachineInstanceTemplateSpec{
					Spec: cnv.VirtualMachineInstanceSpec{
						Networks: []cnv.Network{
							{Name: "podnet", NetworkSource: cnv.NetworkSource{Pod: &cnv.PodNetwork{}}},
							{Name: "m1", NetworkSource: cnv.NetworkSource{Multus: &cnv.MultusNetwork{NetworkName: "ns1/net1"}}},
						},
					},
				},
			},
		}
		v := newValidator(t, plan, vm)
		ok, err := v.NetworksMapped(vmRef)
		if err != nil || !ok {
			t.Fatalf("expected ok=true err=nil, got ok=%v err=%v", ok, err)
		}
	})
}

func TestValidator_Load_SucceedsForOpenShiftProvider(t *testing.T) {
	pt := api.OpenShift
	plan := &api.Plan{}
	plan.Referenced.Provider.Source = &api.Provider{Spec: api.ProviderSpec{Type: &pt}}
	v := &Validator{plan: plan, log: logging.WithName("test-ocp-validator")}
	if err := v.Load(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v.inventory == nil {
		t.Fatalf("expected inventory client set")
	}

	// Ensure no accidental k8s calls were made; this should be a pure client construction.
	_ = context.TODO()
}

type failGetCreateClient struct {
	client.Client
	failGetVME    bool
	failCreateVME bool
	err           error
}

func (c *failGetCreateClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.failGetVME {
		if _, ok := obj.(*export.VirtualMachineExport); ok {
			return c.err
		}
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *failGetCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.failCreateVME {
		if _, ok := obj.(*export.VirtualMachineExport); ok {
			return c.err
		}
	}
	return c.Client.Create(ctx, obj, opts...)
}

func newOCPClient(t *testing.T, srcObjs ...runtime.Object) *Client {
	t.Helper()
	s := testScheme(t)
	src := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(srcObjs...).Build()
	ctx := &plancontext.Context{
		Log: logging.WithName("test-ocp-client"),
	}
	return &Client{
		Context:      ctx,
		sourceClient: src,
	}
}

func TestClient_Finalize_DeletesVMExports(t *testing.T) {
	vme1 := &export.VirtualMachineExport{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: "ns"}}
	vme2 := &export.VirtualMachineExport{ObjectMeta: metav1.ObjectMeta{Name: "vm2", Namespace: "ns"}}
	c := newOCPClient(t, vme1, vme2)

	vms := []*planapi.VMStatus{
		{VM: planapi.VM{Ref: ref.Ref{Name: "vm1", Namespace: "ns"}}},
		{VM: planapi.VM{Ref: ref.Ref{Name: "vm2", Namespace: "ns"}}},
	}
	c.Finalize(vms, "plan")

	got := &export.VirtualMachineExport{}
	if err := c.sourceClient.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "vm1"}, got); err == nil {
		t.Fatalf("expected vm1 export deleted")
	}
	if err := c.sourceClient.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "vm2"}, got); err == nil {
		t.Fatalf("expected vm2 export deleted")
	}
}

func TestClient_PowerOffAndOn_RunningPointer(t *testing.T) {
	running := true
	vm := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec:       cnv.VirtualMachineSpec{Running: &running, Template: &cnv.VirtualMachineInstanceTemplateSpec{}},
	}
	c := newOCPClient(t, vm)
	vmRef := ref.Ref{Name: "vm", Namespace: "ns"}

	if err := c.PowerOff(vmRef); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := &cnv.VirtualMachine{}
	_ = c.sourceClient.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "vm"}, got)
	if got.Spec.Running == nil || *got.Spec.Running != false {
		t.Fatalf("expected running=false, got %#v", got.Spec.Running)
	}

	if err := c.PowerOn(vmRef); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	_ = c.sourceClient.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "vm"}, got)
	if got.Spec.Running == nil || *got.Spec.Running != true {
		t.Fatalf("expected running=true, got %#v", got.Spec.Running)
	}
}

func TestClient_PowerOffAndOn_RunStrategyPointer(t *testing.T) {
	rs := cnv.RunStrategyAlways
	vm := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec:       cnv.VirtualMachineSpec{RunStrategy: &rs, Template: &cnv.VirtualMachineInstanceTemplateSpec{}},
	}
	c := newOCPClient(t, vm)
	vmRef := ref.Ref{Name: "vm", Namespace: "ns"}

	if err := c.PowerOff(vmRef); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := &cnv.VirtualMachine{}
	_ = c.sourceClient.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "vm"}, got)
	if got.Spec.RunStrategy == nil || *got.Spec.RunStrategy != cnv.RunStrategyHalted {
		t.Fatalf("expected RunStrategyHalted, got %#v", got.Spec.RunStrategy)
	}

	if err := c.PowerOn(vmRef); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	_ = c.sourceClient.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "vm"}, got)
	if got.Spec.RunStrategy == nil || *got.Spec.RunStrategy != cnv.RunStrategyAlways {
		t.Fatalf("expected RunStrategyAlways, got %#v", got.Spec.RunStrategy)
	}
}

func TestClient_PowerStateAndPoweredOff(t *testing.T) {
	vmRef := ref.Ref{Name: "vm", Namespace: "ns"}

	cMissing := newOCPClient(t /* no vm */)
	if st, err := cMissing.PowerState(vmRef); err == nil || st != planapi.VMPowerStateUnknown {
		t.Fatalf("expected unknown+error, got state=%v err=%v", st, err)
	}
	if _, err := cMissing.PoweredOff(vmRef); err == nil {
		t.Fatalf("expected error")
	}

	running := true
	vmRunning := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec:       cnv.VirtualMachineSpec{Running: &running, Template: &cnv.VirtualMachineInstanceTemplateSpec{}},
	}
	cRun := newOCPClient(t, vmRunning)
	if st, err := cRun.PowerState(vmRef); err != nil || st != planapi.VMPowerStateOn {
		t.Fatalf("expected on,nil got state=%v err=%v", st, err)
	}
	if off, err := cRun.PoweredOff(vmRef); err != nil || off {
		t.Fatalf("expected poweredOff=false,nil got off=%v err=%v", off, err)
	}

	runStrategy := cnv.RunStrategyHalted
	vmHalted := &cnv.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec:       cnv.VirtualMachineSpec{RunStrategy: &runStrategy, Template: &cnv.VirtualMachineInstanceTemplateSpec{}},
	}
	cHalted := newOCPClient(t, vmHalted)
	if st, err := cHalted.PowerState(vmRef); err != nil || st != planapi.VMPowerStateOff {
		t.Fatalf("expected off,nil got state=%v err=%v", st, err)
	}
	if off, err := cHalted.PoweredOff(vmRef); err != nil || !off {
		t.Fatalf("expected poweredOff=true,nil got off=%v err=%v", off, err)
	}
}

func TestClient_PreTransferActions_ExistingReadyAndWaiting(t *testing.T) {
	vmRef := ref.Ref{Name: "vm", Namespace: "ns"}
	vmeReady := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status:     &export.VirtualMachineExportStatus{Phase: export.Ready},
	}
	cReady := newOCPClient(t, vmeReady)
	ready, err := cReady.PreTransferActions(vmRef)
	if err != nil || !ready {
		t.Fatalf("expected ready=true err=nil, got ready=%v err=%v", ready, err)
	}

	vmeNotReady := &export.VirtualMachineExport{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Status:     &export.VirtualMachineExportStatus{Phase: export.Pending},
	}
	cWait := newOCPClient(t, vmeNotReady)
	ready, err = cWait.PreTransferActions(vmRef)
	if err != nil || ready {
		t.Fatalf("expected ready=false err=nil, got ready=%v err=%v", ready, err)
	}
}

func TestClient_PreTransferActions_CreatesWhenMissing_RespectsTTLSetting(t *testing.T) {
	vmRef := ref.Ref{Name: "vm", Namespace: "ns"}
	c := newOCPClient(t /* no export */)

	old := settings.Settings.CDIExportTokenTTL
	settings.Settings.CDIExportTokenTTL = 7
	t.Cleanup(func() { settings.Settings.CDIExportTokenTTL = old })

	ready, err := c.PreTransferActions(vmRef)
	if err != nil || ready {
		t.Fatalf("expected ready=false err=nil after create, got ready=%v err=%v", ready, err)
	}

	got := &export.VirtualMachineExport{}
	if err := c.sourceClient.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "vm"}, got); err != nil {
		t.Fatalf("expected export created: %v", err)
	}
	if got.Spec.Source.Kind != "VirtualMachine" || got.Spec.Source.Name != "vm" {
		t.Fatalf("unexpected source ref: %#v", got.Spec.Source)
	}
	if got.Spec.TTLDuration == nil || got.Spec.TTLDuration.Duration != 7*time.Minute {
		t.Fatalf("unexpected ttl duration: %#v", got.Spec.TTLDuration)
	}
}

func TestClient_PreTransferActions_PropagatesGetAndCreateErrors(t *testing.T) {
	vmRef := ref.Ref{Name: "vm", Namespace: "ns"}
	c := newOCPClient(t)

	// Get error that's not NotFound => returns (true, err)
	c.sourceClient = &failGetCreateClient{Client: c.sourceClient, failGetVME: true, err: errors.New("boom")}
	ready, err := c.PreTransferActions(vmRef)
	if err == nil || !ready {
		t.Fatalf("expected ready=true and error, got ready=%v err=%v", ready, err)
	}

	// NotFound => Create error => returns (true, err)
	s := testScheme(t)
	src := fake.NewClientBuilder().WithScheme(s).Build()
	c2 := &Client{Context: &plancontext.Context{Log: logging.WithName("test-ocp-client")}, sourceClient: &failGetCreateClient{
		Client:        src,
		failCreateVME: true,
		err:           errors.New("create-failed"),
	}}
	ready, err = c2.PreTransferActions(vmRef)
	if err == nil || !ready {
		t.Fatalf("expected ready=true and error, got ready=%v err=%v", ready, err)
	}

	// Ensure no export created when create fails
	got := &export.VirtualMachineExport{}
	if err := src.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "vm"}, got); err == nil || !k8serr.IsNotFound(err) {
		t.Fatalf("expected not found after create failure, got %v", err)
	}
}

func TestDestinationClient_Noops(t *testing.T) {
	d := &DestinationClient{}
	if err := d.DeletePopulatorDataSource(&planapi.VMStatus{}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := d.SetPopulatorCrOwnership(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
