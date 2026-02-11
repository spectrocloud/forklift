package main

import (
	"strings"
	"testing"

	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func containsAll(t *testing.T, got []string, want ...string) {
	t.Helper()
	joined := strings.Join(got, " ")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Fatalf("missing %q in args: %v", w, got)
		}
	}
}

func TestGetVolumePath(t *testing.T) {
	if got := getVolumePath(true); got != devicePath {
		t.Fatalf("expected %q, got %q", devicePath, got)
	}
	if got := getVolumePath(false); got != mountPath+"disk.img" {
		t.Fatalf("expected %q, got %q", mountPath+"disk.img", got)
	}
}

func TestGetOvirtPopulatorPodArgs(t *testing.T) {
	obj := &v1beta1.OvirtVolumePopulator{
		ObjectMeta: metav1.ObjectMeta{Name: "cr", Namespace: "ns"},
		Spec: v1beta1.OvirtVolumePopulatorSpec{
			EngineSecretName: "sec",
			DiskID:           "disk1",
			EngineURL:        "https://engine",
		},
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("ToUnstructured: %v", err)
	}
	u := &unstructured.Unstructured{Object: m}

	args, err := getOvirtPopulatorPodArgs(true, u, corev1.PersistentVolumeClaim{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	containsAll(t, args,
		"--volume-path="+devicePath,
		"--secret-name=sec",
		"--disk-id=disk1",
		"--engine-url=https://engine",
		"--cr-name=cr",
		"--cr-namespace=ns",
	)
}

func TestGetOpenstackPopulatorPodArgs(t *testing.T) {
	obj := &v1beta1.OpenstackVolumePopulator{
		ObjectMeta: metav1.ObjectMeta{Name: "cr", Namespace: "ns"},
		Spec: v1beta1.OpenstackVolumePopulatorSpec{
			IdentityURL: "https://keystone",
			SecretName:  "sec",
			ImageID:     "img1",
		},
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("ToUnstructured: %v", err)
	}
	u := &unstructured.Unstructured{Object: m}

	args, err := getOpenstackPopulatorPodArgs(false, u, corev1.PersistentVolumeClaim{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	containsAll(t, args,
		"--volume-path="+mountPath+"disk.img",
		"--endpoint=https://keystone",
		"--secret-name=sec",
		"--image-id=img1",
		"--cr-name=cr",
		"--cr-namespace=ns",
	)
}

func TestGetVXPopulatorPodArgs(t *testing.T) {
	obj := &v1beta1.VSphereXcopyVolumePopulator{
		ObjectMeta: metav1.ObjectMeta{Name: "cr", Namespace: "ns"},
		Spec: v1beta1.VSphereXcopyVolumePopulatorSpec{
			VmId:                 "vm-123",
			VmdkPath:             "[ds] path.vmdk",
			SecretName:           "sec",
			StorageVendorProduct: "vendor",
		},
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("ToUnstructured: %v", err)
	}
	u := &unstructured.Unstructured{Object: m}
	pvc := corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc1"}}

	args, err := getVXPopulatorPodArgs(false, u, pvc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	containsAll(t, args,
		"--source-vm-id=vm-123",
		"--source-vmdk=[ds] path.vmdk",
		"--target-namespace=ns",
		"--cr-name=cr",
		"--cr-namespace=ns",
		"--owner-name=pvc1",
		"--secret-name=sec",
		"--storage-vendor-product=vendor",
	)
}

func TestGetOpenstackPopulatorPodArgs_InvalidUnstructured_ReturnsError(t *testing.T) {
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": "not-a-map",
		},
	}
	if _, err := getOpenstackPopulatorPodArgs(false, u, corev1.PersistentVolumeClaim{}); err == nil {
		t.Fatalf("expected error")
	}
}
