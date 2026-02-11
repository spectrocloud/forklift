package openstack

import (
	"context"
	"errors"
	"testing"

	v1beta1 "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	planbase "github.com/kubev2v/forklift/pkg/controller/plan/adapter/base"
	provweb "github.com/kubev2v/forklift/pkg/controller/provider/web"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/openstack"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdi "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func storageProfile(name string) *cdi.StorageProfile {
	fs := corev1.PersistentVolumeFilesystem
	return &cdi.StorageProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: cdi.StorageProfileStatus{
			ClaimPropertySets: []cdi.ClaimPropertySet{
				{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					VolumeMode:  &fs,
				},
			},
		},
	}
}

func populatorCR(name, imageID string) *v1beta1.OpenstackVolumePopulator {
	return &v1beta1.OpenstackVolumePopulator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test",
			Labels: map[string]string{
				"migration": "migration1",
				"imageID":   imageID,
			},
		},
		Spec: v1beta1.OpenstackVolumePopulatorSpec{
			IdentityURL: "https://identity.example.invalid",
			SecretName:  "sec",
			ImageID:     imageID,
		},
	}
}

func populatorPVC(name, imageID string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test",
			Labels: map[string]string{
				"migration": "migration1",
				"imageID":   imageID,
			},
		},
	}
}

func TestGetVolumePopulatorCR_NotFoundWhenMissing(t *testing.T) {
	b := createBuilder()
	_, err := b.getVolumePopulatorCR("img1")
	if err == nil || !k8serr.IsNotFound(err) {
		t.Fatalf("expected notfound, got %v", err)
	}
}

func TestGetVolumePopulatorCR_ErrWhenMultiple(t *testing.T) {
	b := createBuilder(populatorCR("a", "img1"), populatorCR("b", "img1"))
	_, err := b.getVolumePopulatorCR("img1")
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestEnsureVolumePopulator_ReturnsExisting(t *testing.T) {
	b := createBuilder(populatorCR("p1", "img1"))
	w := &model.Workload{}
	img := &model.Image{Resource: model.Resource{ID: "img1"}}
	got, err := b.ensureVolumePopulator(w, img, "sec")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.Name != "p1" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestGetVolumePopulatorPVC_NotFoundWhenMissing(t *testing.T) {
	b := createBuilder()
	_, err := b.getVolumePopulatorPVC("img1")
	if err == nil || !k8serr.IsNotFound(err) {
		t.Fatalf("expected notfound, got %v", err)
	}
}

func TestGetVolumePopulatorPVC_ErrWhenMultiple(t *testing.T) {
	b := createBuilder(populatorPVC("pvc-a", "img1"), populatorPVC("pvc-b", "img1"))
	_, err := b.getVolumePopulatorPVC("img1")
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestEnsureVolumePopulatorPVC_NoStorageMap_Err(t *testing.T) {
	b := createBuilder(storageProfile("sc1"))
	b.Context.Map.Storage.Spec.Map = []v1beta1.StoragePair{} // empty map triggers error

	w := &model.Workload{}
	w.ImageID = "img"
	img := &model.Image{Resource: model.Resource{ID: "img1", Name: "img1"}, Properties: map[string]interface{}{forkliftPropertyOriginalImageID: "img"}}
	_, err := b.ensureVolumePopulatorPVC(w, img, map[string]string{}, "pop")
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestPersistentVolumeClaimWithSourceRef_CreatesPVCAndSetsDataSourceRef(t *testing.T) {
	b := createBuilder(storageProfile("sc1"))

	img := model.Image{
		Resource:    model.Resource{ID: "img1", Name: "img1"},
		SizeBytes:   1024,
		VirtualSize: 0,
		Properties:  map[string]interface{}{forkliftPropertyOriginalImageID: "origImg"},
	}
	ann := map[string]string{}
	pvc, err := b.persistentVolumeClaimWithSourceRef(img, "sc1", "pop1", ann, "vm1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if pvc.Spec.DataSourceRef == nil || pvc.Spec.DataSourceRef.Kind != v1beta1.OpenstackVolumePopulatorKind || pvc.Spec.DataSourceRef.Name != "pop1" {
		t.Fatalf("unexpected datasource ref: %#v", pvc.Spec.DataSourceRef)
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "sc1" {
		t.Fatalf("unexpected storage class")
	}
	if pvc.Annotations[planbase.AnnDiskSource] != "origImg" {
		t.Fatalf("expected disk source annotation")
	}
}

func TestBuilder_PopulatorVolumes_ImageBased_SetsRequiresConversionAndCreatesPVCs(t *testing.T) {
	b := createBuilder(storageProfile("sc1"),
		// Pre-create populators to avoid GenerateName behavior in fake client.
		populatorCR("p-vol", "img-vol"),
		populatorCR("p-snap", "img-snap"),
	)
	b.Context.Map.Storage.Spec.Map = []v1beta1.StoragePair{
		{Source: refapi.Ref{Name: v1beta1.GlanceSource}, Destination: v1beta1.DestinationStorage{StorageClass: "sc1"}},
	}

	workload := &model.Workload{}
	workload.ID = "vm1"
	workload.ImageID = "orig-image"
	workload.Volumes = []model.Volume{{Resource: model.Resource{ID: "vol1"}}}

	volImageName := getImageFromVolumeName(b.Context, workload.ID, "vol1")
	snapImageName := getVmSnapshotName(b.Context, workload.ID)

	imgVol := model.Image{
		Resource:    model.Resource{ID: "img-vol", Name: "img-vol"},
		Status:      string(ImageStatusActive),
		DiskFormat:  "raw",
		Properties:  map[string]interface{}{forkliftPropertyOriginalVolumeID: "vol1"},
		SizeBytes:   1024,
		VirtualSize: 0,
	}
	imgSnap := model.Image{
		Resource:   model.Resource{ID: "img-snap", Name: "img-snap"},
		Status:     string(ImageStatusActive),
		DiskFormat: "qcow2",
		Properties: map[string]interface{}{forkliftPropertyOriginalImageID: workload.ImageID},
		SizeBytes:  1024,
	}

	src := &stubInv2{}
	src.findFn = func(resource interface{}, rf refapi.Ref) error {
		switch r := resource.(type) {
		case *model.Workload:
			*r = *workload
			return nil
		case *model.Image:
			switch {
			case rf.Name == volImageName:
				*r = imgVol
				return nil
			case rf.Name == snapImageName:
				*r = imgSnap
				return nil
			case rf.ID == "img-vol":
				*r = imgVol
				return nil
			case rf.ID == "img-snap":
				*r = imgSnap
				return nil
			default:
				return model.NotFoundError{}
			}
		default:
			return nil
		}
	}
	b.Source.Inventory = src

	ann := map[string]string{}
	pvcs, err := b.PopulatorVolumes(refapi.Ref{ID: workload.ID}, ann, "sec")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(pvcs) != 2 {
		t.Fatalf("expected 2 pvcs, got %d", len(pvcs))
	}
	if ann[planbase.AnnRequiresConversion] != "true" || ann[planbase.AnnSourceFormat] != "qcow2" {
		t.Fatalf("expected conversion annotations set, got: %#v", ann)
	}
	for _, pvc := range pvcs {
		if pvc.Spec.DataSourceRef == nil || pvc.Spec.DataSourceRef.Kind != v1beta1.OpenstackVolumePopulatorKind {
			t.Fatalf("expected pvc datasource ref set: %#v", pvc.Spec.DataSourceRef)
		}
	}
}

func TestBuilder_PopulatorVolumes_VolumeBased_UsesVolumeTypeMapping(t *testing.T) {
	b := createBuilder(storageProfile("sc1"),
		populatorCR("p-vol", "img-vol"),
	)
	// storage map and volume types used when workload.ImageID == ""
	b.Context.Map.Storage.Spec.Map = []v1beta1.StoragePair{
		{Source: refapi.Ref{ID: "vtid"}, Destination: v1beta1.DestinationStorage{StorageClass: "sc1"}},
	}

	workload := &model.Workload{}
	workload.ID = "vm1"
	workload.ImageID = ""
	workload.Volumes = []model.Volume{{Resource: model.Resource{ID: "vol1"}, VolumeType: "fast"}}
	workload.VolumeTypes = []model.VolumeType{{Resource: model.Resource{ID: "vtid", Name: "fast"}}}

	volImageName := getImageFromVolumeName(b.Context, workload.ID, "vol1")
	imgVol := model.Image{
		Resource:   model.Resource{ID: "img-vol", Name: "img-vol"},
		Status:     string(ImageStatusActive),
		DiskFormat: "raw",
		Properties: map[string]interface{}{forkliftPropertyOriginalVolumeID: "vol1"},
		SizeBytes:  1024,
	}

	src := &stubInv2{}
	src.findFn = func(resource interface{}, rf refapi.Ref) error {
		switch r := resource.(type) {
		case *model.Workload:
			*r = *workload
			return nil
		case *model.Image:
			switch {
			case rf.Name == volImageName:
				*r = imgVol
				return nil
			case rf.ID == "img-vol":
				*r = imgVol
				return nil
			default:
				return model.NotFoundError{}
			}
		default:
			return nil
		}
	}
	b.Source.Inventory = src

	pvcs, err := b.PopulatorVolumes(refapi.Ref{ID: workload.ID}, map[string]string{}, "sec")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(pvcs) != 1 {
		t.Fatalf("expected 1 pvc, got %d", len(pvcs))
	}
	if pvcs[0].Spec.StorageClassName == nil || *pvcs[0].Spec.StorageClassName != "sc1" {
		t.Fatalf("expected storage class sc1, got %#v", pvcs[0].Spec.StorageClassName)
	}
}

func TestBuilder_SetPopulatorDataSourceLabels_PatchesPopulatorCRLabels(t *testing.T) {
	// Pre-create populator CR with correct imageID/migration labels but without vmID.
	cr1 := populatorCR("p1", "img1")
	delete(cr1.Labels, "vmID")
	pvc1 := populatorPVC("pvc1", "img1")

	b := createBuilder(cr1, pvc1)

	workload := &model.Workload{}
	workload.ID = "vm1"
	workload.Volumes = []model.Volume{{Resource: model.Resource{ID: "vol1"}}}

	lookupName := getImageFromVolumeName(b.Context, workload.ID, "vol1")
	img := model.Image{Resource: model.Resource{ID: "img1", Name: lookupName}}

	src := &stubInv2{}
	src.findFn = func(resource interface{}, rf refapi.Ref) error {
		switch r := resource.(type) {
		case *model.Workload:
			*r = *workload
			return nil
		case *model.Image:
			switch {
			case rf.Name == lookupName:
				*r = img
				return nil
			case rf.ID == "img1":
				*r = img
				return nil
			default:
				return model.NotFoundError{}
			}
		default:
			return nil
		}
	}
	b.Source.Inventory = src

	err := b.SetPopulatorDataSourceLabels(refapi.Ref{ID: "vm1"}, []*corev1.PersistentVolumeClaim{pvc1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	updated := &v1beta1.OpenstackVolumePopulator{}
	if err := b.Destination.Client.Get(context.TODO(), client.ObjectKey{Namespace: "test", Name: "p1"}, updated); err != nil {
		t.Fatalf("expected CR to exist: %v", err)
	}
	if updated.Labels["vmID"] != "vm1" {
		t.Fatalf("expected vmID label set, got %#v", updated.Labels)
	}
	if updated.Labels["migration"] != "migration1" {
		t.Fatalf("expected migration label set to migration1, got %#v", updated.Labels)
	}
}

// ---- Consolidated from validator_more_unit_test.go ----

type stubInvFind struct {
	workload *model.Workload
	err      error
}

func (s stubInvFind) Finder() provweb.Finder { return nil }
func (s stubInvFind) Get(resource interface{}, id string) error {
	return nil
}
func (s stubInvFind) List(list interface{}, param ...provweb.Param) error { return nil }
func (s stubInvFind) Watch(resource interface{}, h provweb.EventHandler) (*provweb.Watch, error) {
	return nil, nil
}
func (s stubInvFind) Find(resource interface{}, rf refapi.Ref) error {
	if s.err != nil {
		return s.err
	}
	switch r := resource.(type) {
	case *model.Workload:
		if s.workload != nil {
			*r = *s.workload
		}
		return nil
	default:
		return nil
	}
}
func (s stubInvFind) VM(rf *refapi.Ref) (interface{}, error)       { return struct{}{}, nil }
func (s stubInvFind) Workload(rf *refapi.Ref) (interface{}, error) { return struct{}{}, nil }
func (s stubInvFind) Network(rf *refapi.Ref) (interface{}, error)  { return struct{}{}, nil }
func (s stubInvFind) Storage(rf *refapi.Ref) (interface{}, error)  { return struct{}{}, nil }
func (s stubInvFind) Host(rf *refapi.Ref) (interface{}, error)     { return struct{}{}, nil }

func TestValidator_OpenStack_SimpleAndMappings(t *testing.T) {
	t.Run("load constructs provider web client", func(t *testing.T) {
		pt := v1beta1.OpenStack
		plan := &v1beta1.Plan{}
		plan.Referenced.Provider.Source = &v1beta1.Provider{Spec: v1beta1.ProviderSpec{Type: &pt}}
		v := &Validator{plan: plan}
		if err := v.Load(); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if v.inventory == nil {
			t.Fatalf("expected inventory set")
		}
	})

	t.Run("simple no-ops", func(t *testing.T) {
		v := &Validator{plan: &v1beta1.Plan{}}
		if ok, err := v.MaintenanceMode(refapi.Ref{}); err != nil || !ok {
			t.Fatalf("expected maintenance ok=true err=nil, got ok=%v err=%v", ok, err)
		}
		if ok := v.WarmMigration(); ok {
			t.Fatalf("expected warm=false")
		}
		if ok, _, _, err := v.SharedDisks(refapi.Ref{}, nil); err != nil || !ok {
			t.Fatalf("expected SharedDisks ok=true err=nil, got ok=%v err=%v", ok, err)
		}
		if ok, err := v.DirectStorage(refapi.Ref{}); err != nil || !ok {
			t.Fatalf("expected DirectStorage ok=true err=nil")
		}
		if ok, err := v.StaticIPs(refapi.Ref{}); err != nil || !ok {
			t.Fatalf("expected StaticIPs ok=true err=nil")
		}
		if ok, err := v.ChangeTrackingEnabled(refapi.Ref{}); err != nil || !ok {
			t.Fatalf("expected ChangeTrackingEnabled ok=true err=nil")
		}
	})

	t.Run("storage mapped: requires storage refs and glance when image-based", func(t *testing.T) {
		plan := &v1beta1.Plan{}
		plan.Referenced.Map.Storage = &v1beta1.StorageMap{}
		plan.Referenced.Map.Storage.Status.Refs.List = []refapi.Ref{
			{ID: "vtid"},
			{Name: v1beta1.GlanceSource},
		}
		v := &Validator{plan: plan}
		w := &model.Workload{}
		w.VolumeTypes = []model.VolumeType{{Resource: model.Resource{ID: "vtid"}}}
		w.ImageID = "img" // image-based requires glance
		v.inventory = stubInvFind{workload: w}

		ok, err := v.StorageMapped(refapi.Ref{ID: "vm1"})
		if err != nil || !ok {
			t.Fatalf("expected ok=true err=nil, got ok=%v err=%v", ok, err)
		}

		// Missing volume type ref => not ok.
		planMissingVT := &v1beta1.Plan{}
		planMissingVT.Referenced.Map.Storage = &v1beta1.StorageMap{}
		planMissingVT.Referenced.Map.Storage.Status.Refs.List = []refapi.Ref{
			{Name: v1beta1.GlanceSource}, // no volume type id
		}
		vMissingVT := &Validator{plan: planMissingVT, inventory: stubInvFind{workload: w}}
		ok, err = vMissingVT.StorageMapped(refapi.Ref{ID: "vm1"})
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}

		// Missing glance ref => not ok.
		plan2 := &v1beta1.Plan{}
		plan2.Referenced.Map.Storage = &v1beta1.StorageMap{}
		plan2.Referenced.Map.Storage.Status.Refs.List = []refapi.Ref{{ID: "vtid"}}
		v2 := &Validator{plan: plan2, inventory: stubInvFind{workload: w}}
		ok, err = v2.StorageMapped(refapi.Ref{ID: "vm1"})
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("networks mapped", func(t *testing.T) {
		plan := &v1beta1.Plan{}
		plan.Referenced.Map.Network = &v1beta1.NetworkMap{}
		plan.Referenced.Map.Network.Status.Refs.List = []refapi.Ref{{ID: "net1"}}
		v := &Validator{plan: plan}
		w := &model.Workload{}
		w.Networks = []model.Network{{Resource: model.Resource{ID: "net1"}}}
		v.inventory = stubInvFind{workload: w}

		ok, err := v.NetworksMapped(refapi.Ref{ID: "vm1"})
		if err != nil || !ok {
			t.Fatalf("expected ok=true err=nil, got ok=%v err=%v", ok, err)
		}

		plan2 := &v1beta1.Plan{}
		plan2.Referenced.Map.Network = &v1beta1.NetworkMap{}
		plan2.Referenced.Map.Network.Status.Refs.List = []refapi.Ref{} // missing
		v2 := &Validator{plan: plan2, inventory: stubInvFind{workload: w}}
		ok, err = v2.NetworksMapped(refapi.Ref{ID: "vm1"})
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("pod network at most one pod mapping for vm networks", func(t *testing.T) {
		plan := &v1beta1.Plan{}
		plan.Referenced.Map.Network = &v1beta1.NetworkMap{
			Spec: v1beta1.NetworkMapSpec{
				Map: []v1beta1.NetworkPair{
					{Source: refapi.Ref{ID: "net1"}, Destination: v1beta1.DestinationNetwork{Type: "Pod"}},
				},
			},
		}
		v := &Validator{plan: plan}
		w := &model.Workload{}
		w.Networks = []model.Network{{Resource: model.Resource{ID: "net1"}}}
		v.inventory = stubInvFind{workload: w}

		ok, err := v.PodNetwork(refapi.Ref{ID: "vm1"})
		if err != nil || !ok {
			t.Fatalf("expected ok=true err=nil, got ok=%v err=%v", ok, err)
		}

		plan.Referenced.Map.Network.Spec.Map = append(plan.Referenced.Map.Network.Spec.Map,
			v1beta1.NetworkPair{Source: refapi.Ref{ID: "net1"}, Destination: v1beta1.DestinationNetwork{Type: "Pod"}},
		)
		ok, err = v.PodNetwork(refapi.Ref{ID: "vm1"})
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}

		// Missing network map => zero values.
		v3 := &Validator{plan: &v1beta1.Plan{}, inventory: stubInvFind{workload: w}}
		ok, err = v3.PodNetwork(refapi.Ref{ID: "vm1"})
		if err != nil || ok {
			t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("inventory find error is wrapped", func(t *testing.T) {
		plan := &v1beta1.Plan{}
		plan.Referenced.Map.Network = &v1beta1.NetworkMap{}
		v := &Validator{plan: plan, inventory: stubInvFind{err: errors.New("boom")}}
		_, err := v.NetworksMapped(refapi.Ref{ID: "vm1"})
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestEnsureVolumePopulatorPVC_ImageBased_UsesGlanceStorageMap(t *testing.T) {
	b := createBuilder(storageProfile("sc-glance"))
	b.Context.Map.Storage.Spec.Map = []v1beta1.StoragePair{
		{Source: refapi.Ref{Name: v1beta1.GlanceSource}, Destination: v1beta1.DestinationStorage{StorageClass: "sc-glance"}},
	}

	w := &model.Workload{}
	w.ImageID = "img0"
	img := &model.Image{
		Resource:  model.Resource{ID: "img1", Name: "img1"},
		SizeBytes: 1024,
		Properties: map[string]interface{}{
			forkliftPropertyOriginalImageID: "img0",
		},
	}
	pvc, err := b.ensureVolumePopulatorPVC(w, img, map[string]string{}, "pop1")
	if err != nil || pvc == nil {
		t.Fatalf("unexpected: pvc=%v err=%v", pvc, err)
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "sc-glance" {
		t.Fatalf("unexpected storage class: %#v", pvc.Spec.StorageClassName)
	}
}

func TestPopulatorTransferredBytes_ParseError_ReturnsZeroNoErr(t *testing.T) {
	// Create CR with bad progress and verify we get 0 bytes with nil err.
	cr := populatorCR("p1", "img1")
	cr.Status.Progress = "not-int"
	b := createBuilder(cr)

	// Source inventory maps pvc->image
	src := &stubInv2{}
	src.findFn = func(resource interface{}, rf refapi.Ref) error {
		img := resource.(*model.Image)
		img.Resource.ID = rf.ID
		img.Name = "img-name"
		return nil
	}
	b.Source.Inventory = src

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"imageID": "img1"},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}
	n, err := b.PopulatorTransferredBytes(pvc)
	if err != nil || n != 0 {
		t.Fatalf("expected (0,nil) got (%d,%v)", n, err)
	}
}

func TestGetPopulatorTaskName_UsesImageName(t *testing.T) {
	b := createBuilder()
	src := &stubInv2{}
	src.findFn = func(resource interface{}, rf refapi.Ref) error {
		img := resource.(*model.Image)
		img.Name = "task-img"
		return nil
	}
	b.Source.Inventory = src

	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"imageID": "img1"}}}
	name, err := b.GetPopulatorTaskName(pvc)
	if err != nil || name != "task-img" {
		t.Fatalf("unexpected: %q %v", name, err)
	}
}

func TestCreateVolumePopulatorCR_SetsFields(t *testing.T) {
	b := createBuilder()
	img := model.Image{Resource: model.Resource{ID: "img1", Name: "img-name"}}
	cr, err := b.createVolumePopulatorCR(img, "sec", "vm1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cr.Namespace != "test" || cr.Spec.SecretName != "sec" || cr.Spec.ImageID != "img1" {
		t.Fatalf("unexpected: %#v", cr)
	}
	if cr.GenerateName != "img-name-" {
		t.Fatalf("unexpected generateName: %q", cr.GenerateName)
	}
}
