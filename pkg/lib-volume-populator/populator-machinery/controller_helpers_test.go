package populator_machinery

import (
	"context"
	"net/http"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-helpers/storage/volume"
)

func TestNotificationMaps_AddAndCleanup(t *testing.T) {
	c := &controller{
		notifyMap:  map[string]*stringSet{},
		cleanupMap: map[string]*stringSet{},
	}

	c.addNotification("key1", "pvc", "ns", "p1")
	c.addNotification("key2", "pvc", "ns", "p1")
	c.addNotification("key2", "pvc", "", "cluster-scope")

	// cleanup key2 should remove it from both notify entries
	c.cleanupNotifications("key2")
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.notifyMap["pvc/ns/p1"]; s == nil || len(s.set) != 1 {
		t.Fatalf("expected only 1 key remaining for pvc/ns/p1, got: %#v", s)
	}
	if _, ok := c.notifyMap["pvc/ns/p1"].set["key1"]; !ok {
		t.Fatalf("expected key1 to remain")
	}
	// cluster-scope entry should be removed entirely (only key2 was there)
	if _, ok := c.notifyMap["pvc/cluster-scope"]; ok {
		t.Fatalf("expected cluster-scope notify entry removed")
	}
}

func TestTranslateObject(t *testing.T) {
	pod := &corev1.Pod{}
	if got := translateObject(pod); got == nil {
		t.Fatalf("expected object")
	}
	tomb := cache.DeletedFinalStateUnknown{Obj: pod}
	if got := translateObject(tomb); got == nil {
		t.Fatalf("expected object from tombstone")
	}
}

func TestHandlePVC_AddsWorkQueueKeyAndMappedNotifications(t *testing.T) {
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())
	t.Cleanup(q.ShutDown)

	c := &controller{
		notifyMap:  map[string]*stringSet{},
		cleanupMap: map[string]*stringSet{},
		workqueue:  q,
	}

	// register notification to enqueue when pvc/ns/p1 changes
	c.addNotification("call-me", "pvc", "ns", "p1")

	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Namespace = "ns"
	pvc.Name = "p1"
	c.handlePVC(pvc)

	// Expect at least two keys: the explicit pvc key and our notification.
	got := map[string]bool{}
	for i := 0; i < 2; i++ {
		item, _ := q.Get()
		got[item] = true
		q.Done(item)
	}
	if !got["pvc/ns/p1"] {
		t.Fatalf("expected pvc/ns/p1 enqueued, got: %#v", got)
	}
	if !got["call-me"] {
		t.Fatalf("expected call-me enqueued, got: %#v", got)
	}
}

func TestUpdatePopulatorProgress(t *testing.T) {
	cr := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if err := updatePopulatorProgress(42, cr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, found, err := unstructured.NestedString(cr.Object, "status", "progress")
	if err != nil || !found || v != "42" {
		t.Fatalf("unexpected progress field: found=%v err=%v v=%q", found, err, v)
	}
}

func TestMakePopulatePodSpec(t *testing.T) {
	spec := makePopulatePodSpec("prime-pvc", "sec")
	if len(spec.Containers) != 1 || spec.Containers[0].Name != populatorContainerName {
		t.Fatalf("unexpected containers: %#v", spec.Containers)
	}
	if spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatalf("expected RestartPolicyNever")
	}
	if spec.SecurityContext == nil || spec.SecurityContext.FSGroup == nil || *spec.SecurityContext.FSGroup != int64(qemuGroup) {
		t.Fatalf("expected FSGroup=%d", qemuGroup)
	}
	if len(spec.Volumes) != 1 || spec.Volumes[0].Name != populatorPodVolumeName {
		t.Fatalf("unexpected volumes: %#v", spec.Volumes)
	}
	if spec.Volumes[0].PersistentVolumeClaim == nil || spec.Volumes[0].PersistentVolumeClaim.ClaimName != "prime-pvc" {
		t.Fatalf("unexpected pvc volume source: %#v", spec.Volumes[0].VolumeSource)
	}
}

func TestBuildHTTPClient_InsecureSkipVerify(t *testing.T) {
	c := buildHTTPClient()
	if c == nil || c.Transport == nil {
		t.Fatalf("expected client transport")
	}
	ht, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.Transport)
	}
	if ht.TLSClientConfig == nil || !ht.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify=true")
	}
}

func TestGetPodMetricsPortAndURL(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "c",
					Ports: []corev1.ContainerPort{{Name: "metrics", ContainerPort: 8443}},
				},
			},
		},
		Status: corev1.PodStatus{PodIP: "10.0.0.1"},
	}
	port, err := getPodMetricsPort(pod)
	if err != nil || port != 8443 {
		t.Fatalf("unexpected port: %d err=%v", port, err)
	}
	url, err := getMetricsURL(pod)
	if err != nil || url != "https://10.0.0.1:8443/metrics" {
		t.Fatalf("unexpected url: %q err=%v", url, err)
	}
	// nil pod returns "", nil
	if url, err := getMetricsURL(nil); err != nil || url != "" {
		t.Fatalf("expected empty url for nil pod")
	}
	// missing port errors
	pod2 := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}}, Status: corev1.PodStatus{PodIP: "10.0.0.1"}}
	if _, err := getPodMetricsPort(pod2); err == nil {
		t.Fatalf("expected error for missing port")
	}
}

func TestCheckIntreeStorageClass(t *testing.T) {
	c := &controller{}
	pvc := &corev1.PersistentVolumeClaim{}
	sc := &storagev1.StorageClass{Provisioner: "kubernetes.io/aws-ebs"}

	// Not migrated => error
	if err := c.checkIntreeStorageClass(pvc, sc); err == nil {
		t.Fatalf("expected error for in-tree SC without migration")
	}
	// Mark migrated => ok
	pvc.Annotations = map[string]string{volume.AnnMigratedTo: "ebs.csi.aws.com"}
	if err := c.checkIntreeStorageClass(pvc, sc); err != nil {
		t.Fatalf("expected nil for migrated pvc, got %v", err)
	}
	// CSI provisioner => ok
	sc2 := &storagev1.StorageClass{Provisioner: "csi.example.com"}
	if err := c.checkIntreeStorageClass(&corev1.PersistentVolumeClaim{}, sc2); err != nil {
		t.Fatalf("expected nil for CSI provisioner, got %v", err)
	}
}

func TestEnsureFinalizer_BuildsPatchOps(t *testing.T) {
	client := fake.NewSimpleClientset()
	c := &controller{kubeClient: client}

	ns := "ns"
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Namespace = ns
	pvc.Name = "p1"
	pvc.Finalizers = []string{"a", "b"}

	_, _ = client.CoreV1().PersistentVolumeClaims(ns).Create(context.Background(), pvc, metav1.CreateOptions{})

	var patched string
	client.Fake.PrependReactor("patch", "persistentvolumeclaims", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		pa := action.(k8stesting.PatchAction)
		patched = string(pa.GetPatch())
		// IMPORTANT: don't call back into the fake client from inside a reactor (can deadlock).
		// Just return a PVC object so the patch call can complete.
		return true, pvc.DeepCopy(), nil
	})

	// add finalizer
	if err := c.ensureFinalizer(context.Background(), pvc, "x", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(patched, "\"op\":\"add\"") || !strings.Contains(patched, "\"x\"") {
		t.Fatalf("expected add patch, got: %s", patched)
	}

	// remove finalizer: simulate it already present
	pvc.Finalizers = append(pvc.Finalizers, "x")
	patched = ""
	if err := c.ensureFinalizer(context.Background(), pvc, "x", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(patched, "\"op\":\"remove\"") {
		t.Fatalf("expected remove patch, got: %s", patched)
	}
}
