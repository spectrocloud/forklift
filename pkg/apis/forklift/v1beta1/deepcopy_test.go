package v1beta1

import (
	"os"
	"testing"

	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	providerapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/provider"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func TestDeepCopy_Plan(t *testing.T) {
	p := &Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p",
			Namespace: "ns",
			UID:       types.UID("uid"),
			Labels:    map[string]string{"a": "b"},
		},
		Spec: PlanSpec{
			TargetNamespace: "target",
			VMs: []planapi.VM{
				{Ref: refapi.Ref{ID: "vm-1"}, TargetName: "vm1"},
			},
			TransferNetwork: &corev1.ObjectReference{Namespace: "ns", Name: "nad"},
		},
	}

	cp := p.DeepCopy()
	if cp == nil {
		t.Fatalf("DeepCopy returned nil")
	}
	if cp == p {
		t.Fatalf("expected DeepCopy to return a different pointer")
	}
	if cp.Spec.TargetNamespace != "target" || cp.Name != "p" || cp.Namespace != "ns" {
		t.Fatalf("unexpected deepcopy values: %#v", cp)
	}

	// Basic deep-copy sanity: mutating copy must not mutate original.
	cp.Spec.TargetNamespace = "changed"
	if p.Spec.TargetNamespace != "target" {
		t.Fatalf("mutating copy mutated original")
	}
	cp.Labels["a"] = "changed"
	if p.Labels["a"] != "b" {
		t.Fatalf("expected labels map to be deep-copied")
	}
}

func TestDeepCopy_Migration(t *testing.T) {
	m := &Migration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "m",
			Namespace: "ns",
			UID:       types.UID("muid"),
		},
	}
	cp := m.DeepCopy()
	if cp == nil || cp == m {
		t.Fatalf("DeepCopy returned invalid copy: %#v", cp)
	}
	if cp.Name != "m" || cp.Namespace != "ns" || cp.UID != types.UID("muid") {
		t.Fatalf("unexpected deepcopy values: %#v", cp)
	}
}

func TestDeepCopy_Provider(t *testing.T) {
	providerType := VSphere
	p := &Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pr",
			Namespace: "ns",
		},
		Spec: ProviderSpec{
			Type: &providerType,
			URL:  "https://example.invalid",
			Settings: map[string]string{
				"foo": "bar",
			},
		},
	}
	cp := p.DeepCopy()
	if cp == nil || cp == p {
		t.Fatalf("DeepCopy returned invalid copy: %#v", cp)
	}
	if cp.Spec.Type == nil || *cp.Spec.Type != VSphere {
		t.Fatalf("unexpected provider type in copy: %#v", cp.Spec.Type)
	}
	cp.Spec.Settings["foo"] = "changed"
	if p.Spec.Settings["foo"] != "bar" {
		t.Fatalf("expected settings map to be deep-copied")
	}
}

func TestPlanSpec_FindVM(t *testing.T) {
	spec := &PlanSpec{
		VMs: []planapi.VM{
			{Ref: refapi.Ref{ID: "vm-1", Name: "one"}},
			{Ref: refapi.Ref{ID: "vm-2", Name: "two"}},
		},
	}
	got, found := spec.FindVM(refapi.Ref{ID: "vm-2"})
	if !found || got == nil || got.ID != "vm-2" || got.Name != "two" {
		t.Fatalf("unexpected find result: found=%v vm=%#v", found, got)
	}
	_, found = spec.FindVM(refapi.Ref{ID: "missing"})
	if found {
		t.Fatalf("expected not found")
	}
}

func TestMigrationSpec_Canceled(t *testing.T) {
	spec := &MigrationSpec{
		Cancel: []refapi.Ref{
			{ID: ""},
			{ID: "vm-1"},
		},
	}
	if spec.Canceled(refapi.Ref{}) {
		t.Fatalf("expected empty ref to not be canceled")
	}
	if !spec.Canceled(refapi.Ref{ID: "vm-1"}) {
		t.Fatalf("expected vm-1 to be canceled")
	}
	if spec.Canceled(refapi.Ref{ID: "vm-2"}) {
		t.Fatalf("expected vm-2 to not be canceled")
	}
}

func TestMigration_MatchPlan(t *testing.T) {
	m := &Migration{
		Spec: MigrationSpec{
			Plan: corev1.ObjectReference{Namespace: "ns", Name: "p1"},
		},
	}
	p := &Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p1"}}
	if !m.Match(p) {
		t.Fatalf("expected match")
	}
	p2 := &Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p2"}}
	if m.Match(p2) {
		t.Fatalf("expected mismatch")
	}
}

func TestPlan_ShouldUseV2vForTransfer(t *testing.T) {
	vs := VSphere
	ova := Ova
	osType := OpenStack
	host := OpenShift

	vsProvider := &Provider{Spec: ProviderSpec{Type: &vs, URL: "https://vsphere.example.invalid"}}
	ovaProvider := &Provider{Spec: ProviderSpec{Type: &ova, URL: "file://x.ova"}}
	osProvider := &Provider{Spec: ProviderSpec{Type: &osType, URL: "https://openstack.example.invalid"}}
	hostProvider := &Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "konveyor-forklift"}, Spec: ProviderSpec{Type: &host, URL: ""}}
	nonHostDest := &Provider{Spec: ProviderSpec{Type: &host, URL: "https://cluster.example.invalid"}}

	t.Run("missing source provider returns error", func(t *testing.T) {
		p := &Plan{}
		p.Referenced.Provider.Source = nil
		p.Referenced.Provider.Destination = hostProvider
		_, err := p.ShouldUseV2vForTransfer()
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing destination provider returns error", func(t *testing.T) {
		p := &Plan{}
		p.Referenced.Provider.Source = vsProvider
		p.Referenced.Provider.Destination = nil
		_, err := p.ShouldUseV2vForTransfer()
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("vsphere cold local with shared disks + guest conversion uses v2v transfer", func(t *testing.T) {
		p := &Plan{
			Spec: PlanSpec{
				Warm:                false,
				MigrateSharedDisks:  true,
				SkipGuestConversion: false,
			},
		}
		p.Referenced.Provider.Source = vsProvider
		p.Referenced.Provider.Destination = hostProvider
		got, err := p.ShouldUseV2vForTransfer()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatalf("expected true")
		}
	})

	t.Run("vsphere warm returns false", func(t *testing.T) {
		p := &Plan{Spec: PlanSpec{Warm: true, MigrateSharedDisks: true}}
		p.Referenced.Provider.Source = vsProvider
		p.Referenced.Provider.Destination = hostProvider
		got, err := p.ShouldUseV2vForTransfer()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatalf("expected false")
		}
	})

	t.Run("vsphere non-host destination returns false", func(t *testing.T) {
		p := &Plan{Spec: PlanSpec{Warm: false, MigrateSharedDisks: true}}
		p.Referenced.Provider.Source = vsProvider
		p.Referenced.Provider.Destination = nonHostDest
		got, err := p.ShouldUseV2vForTransfer()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatalf("expected false")
		}
	})

	t.Run("vsphere skip shared disks returns false", func(t *testing.T) {
		p := &Plan{Spec: PlanSpec{Warm: false, MigrateSharedDisks: false}}
		p.Referenced.Provider.Source = vsProvider
		p.Referenced.Provider.Destination = hostProvider
		got, err := p.ShouldUseV2vForTransfer()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatalf("expected false")
		}
	})

	t.Run("vsphere skip guest conversion returns false", func(t *testing.T) {
		p := &Plan{Spec: PlanSpec{Warm: false, MigrateSharedDisks: true, SkipGuestConversion: true}}
		p.Referenced.Provider.Source = vsProvider
		p.Referenced.Provider.Destination = hostProvider
		got, err := p.ShouldUseV2vForTransfer()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatalf("expected false")
		}
	})

	t.Run("ova always uses v2v transfer", func(t *testing.T) {
		p := &Plan{}
		p.Referenced.Provider.Source = ovaProvider
		p.Referenced.Provider.Destination = hostProvider
		got, err := p.ShouldUseV2vForTransfer()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatalf("expected true")
		}
	})

	t.Run("other sources return false", func(t *testing.T) {
		p := &Plan{}
		p.Referenced.Provider.Source = osProvider
		p.Referenced.Provider.Destination = hostProvider
		got, err := p.ShouldUseV2vForTransfer()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatalf("expected false")
		}
	})
}

func TestPlan_IsSourceProviderHelpers(t *testing.T) {
	vs := VSphere
	osType := OpenStack
	ov := OVirt
	ocp := OpenShift
	ova := Ova

	p := &Plan{}
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &osType}}
	if !p.IsSourceProviderOpenstack() || p.IsSourceProviderOvirt() || p.IsSourceProviderOCP() || p.IsSourceProviderVSphere() || p.IsSourceProviderOVA() {
		t.Fatalf("unexpected openstack detection")
	}

	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &ov}}
	if !p.IsSourceProviderOvirt() {
		t.Fatalf("expected ovirt")
	}
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &ocp}}
	if !p.IsSourceProviderOCP() {
		t.Fatalf("expected ocp")
	}
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &vs}}
	if !p.IsSourceProviderVSphere() {
		t.Fatalf("expected vsphere")
	}
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &ova}}
	if !p.IsSourceProviderOVA() {
		t.Fatalf("expected ova")
	}
}

func TestMaps_FindHelpers_AndStorageVendorProducts(t *testing.T) {
	nm := &NetworkMap{
		Spec: NetworkMapSpec{
			Map: []NetworkPair{
				{Source: refapi.Ref{ID: "n1", Type: "pod", Name: "ns1/net1"}, Destination: DestinationNetwork{Type: "pod"}},
				{Source: refapi.Ref{ID: "n2", Type: "multus", Namespace: "ns2", Name: "net2"}, Destination: DestinationNetwork{Type: "multus", Namespace: "t", Name: "tn"}},
			},
		},
	}
	pair, found := nm.FindNetwork("n2")
	if !found || pair.Source.ID != "n2" {
		t.Fatalf("unexpected FindNetwork: found=%v pair=%#v", found, pair)
	}
	pair, found = nm.FindNetworkByType("pod")
	if !found || pair.Source.ID != "n1" {
		t.Fatalf("unexpected FindNetworkByType: found=%v pair=%#v", found, pair)
	}
	// Namespace set case.
	pair, found = nm.FindNetworkByNameAndNamespace("ns2", "net2")
	if !found || pair.Source.ID != "n2" {
		t.Fatalf("unexpected FindNetworkByNameAndNamespace (ns/name): found=%v pair=%#v", found, pair)
	}
	// Namespace empty but encoded in name case.
	pair, found = nm.FindNetworkByNameAndNamespace("ns1", "net1")
	if !found || pair.Source.ID != "n1" {
		t.Fatalf("unexpected FindNetworkByNameAndNamespace (encoded): found=%v pair=%#v", found, pair)
	}

	sm := &StorageMap{
		Spec: StorageMapSpec{
			Map: []StoragePair{
				{Source: refapi.Ref{ID: "s1", Name: "ds1"}, Destination: DestinationStorage{StorageClass: "sc1"}},
				{Source: refapi.Ref{ID: "s2", Name: "ds2"}, Destination: DestinationStorage{StorageClass: "sc2"}},
			},
		},
	}
	sp, found := sm.FindStorage("s2")
	if !found || sp.Source.Name != "ds2" {
		t.Fatalf("unexpected FindStorage: found=%v pair=%#v", found, sp)
	}
	sp, found = sm.FindStorageByName("ds1")
	if !found || sp.Source.ID != "s1" {
		t.Fatalf("unexpected FindStorageByName: found=%v pair=%#v", found, sp)
	}

	products := StorageVendorProducts()
	if len(products) == 0 {
		t.Fatalf("expected vendor products")
	}
	seen := map[StorageVendorProduct]bool{}
	for _, p := range products {
		seen[p] = true
	}
	for _, exp := range []StorageVendorProduct{
		StorageVendorProductVantara,
		StorageVendorProductOntap,
		StorageVendorProductPrimera3Par,
		StorageVendorProductPureFlashArray,
		StorageVendorProductPowerFlex,
	} {
		if !seen[exp] {
			t.Fatalf("missing vendor product %s", exp)
		}
	}
}

func TestProviderAndRefHelperMethods(t *testing.T) {
	// ProviderType.String
	if VSphere.String() != "vsphere" {
		t.Fatalf("unexpected provider type string: %s", VSphere.String())
	}

	// Provider.IsRestrictedHost depends on POD_NAMESPACE.
	t.Setenv("POD_NAMESPACE", "konveyor-forklift")
	ocp := OpenShift
	p := &Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "other-ns"},
		Spec:       ProviderSpec{Type: &ocp, URL: ""},
	}
	if !p.IsHost() {
		t.Fatalf("expected host provider")
	}
	if !p.IsRestrictedHost() {
		t.Fatalf("expected restricted host (namespace mismatch)")
	}

	p.Namespace = os.Getenv("POD_NAMESPACE")
	if p.IsRestrictedHost() {
		t.Fatalf("expected non-restricted host (namespace match)")
	}

	// HasReconciled.
	p.Generation = 3
	p.Status.ObservedGeneration = 3
	if !p.HasReconciled() {
		t.Fatalf("expected reconciled")
	}

	// RequiresConversion.
	if p.RequiresConversion() {
		t.Fatalf("expected false when ConvertDisk unset")
	}
	conv := true
	p.Spec.ConvertDisk = &conv
	if !p.RequiresConversion() {
		t.Fatalf("expected true when ConvertDisk true")
	}

	// UseVddkAioOptimization.
	p.Spec.Settings = map[string]string{}
	if p.UseVddkAioOptimization() {
		t.Fatalf("expected false when setting missing")
	}
	p.Spec.Settings[UseVddkAioOptimization] = "not-bool"
	if p.UseVddkAioOptimization() {
		t.Fatalf("expected false when setting invalid")
	}
	p.Spec.Settings[UseVddkAioOptimization] = "true"
	if !p.UseVddkAioOptimization() {
		t.Fatalf("expected true when setting true")
	}

	// ref.Ref helpers.
	r0 := refapi.Ref{}
	if !r0.NotSet() {
		t.Fatalf("expected NotSet true")
	}
	if got := r0.String(); got == "" {
		t.Fatalf("expected non-empty string")
	}
	r1 := refapi.Ref{Type: "vm", ID: "id1", Name: "n1"}
	if r1.NotSet() {
		t.Fatalf("expected NotSet false")
	}
	if got := r1.String(); got == "" {
		t.Fatalf("expected non-empty string")
	}

	refs := &refapi.Refs{List: []refapi.Ref{{ID: "id1"}, {ID: "id2"}}}
	if !refs.Find(refapi.Ref{ID: "id2"}) {
		t.Fatalf("expected Find true")
	}
	if refs.Find(refapi.Ref{ID: "missing"}) {
		t.Fatalf("expected Find false")
	}

	// exercise generated deepcopies in subpackages too.
	if r1.DeepCopy() == nil {
		t.Fatalf("expected deepcopy")
	}
	pair := &providerapi.Pair{Source: corev1.ObjectReference{Name: "s"}, Destination: corev1.ObjectReference{Name: "d"}}
	if pair.DeepCopy() == nil {
		t.Fatalf("expected deepcopy")
	}
}

func TestReferenced_FindHook_AndGetGroupResource(t *testing.T) {
	refd := &Referenced{
		Hooks: []*Hook{
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h1"}},
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h2"}},
		},
	}
	found, hook := refd.FindHook(corev1.ObjectReference{Namespace: "ns", Name: "h2"})
	if !found || hook == nil || hook.Name != "h2" {
		t.Fatalf("unexpected FindHook: found=%v hook=%#v", found, hook)
	}
	if refd.DeepCopy() != refd {
		t.Fatalf("expected DeepCopy to return same pointer for Referenced")
	}

	// GetGroupResource
	if _, err := GetGroupResource(&Provider{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := GetGroupResource(&Plan{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := GetGroupResource(&Migration{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := GetGroupResource(&Hook{}); err == nil {
		t.Fatalf("expected error for unknown type")
	}
}

func TestGeneratedDeepCopy_V1beta1APIObjects(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	vs := VSphere
	ov := OVirt
	osType := OpenStack
	ova := Ova

	cond := libcnd.Condition{
		Type:     libcnd.Ready,
		Status:   libcnd.True,
		Category: libcnd.Required,
		Message:  "ok",
		Items:    []string{"a"},
	}

	hook := &Hook{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "h1",
			Namespace: "ns",
			Labels:    map[string]string{"k": "v"},
		},
		Spec: HookSpec{
			Image:    "img",
			Playbook: "pb",
			Deadline: 10,
		},
		Status: HookStatus{
			Conditions:         libcnd.Conditions{List: []libcnd.Condition{cond}},
			ObservedGeneration: 2,
		},
	}
	hookList := &HookList{Items: []Hook{*hook}}

	host := &Host{
		ObjectMeta: metav1.ObjectMeta{Name: "host1", Namespace: "ns"},
		Spec: HostSpec{
			Ref:       refapi.Ref{ID: "hid", Name: "hname", Type: "host"},
			Provider:  corev1.ObjectReference{Name: "p1", Namespace: "ns"},
			IpAddress: "1.2.3.4",
			Secret:    corev1.ObjectReference{Name: "sec", Namespace: "ns"},
		},
		Status: HostStatus{
			Conditions:         libcnd.Conditions{List: []libcnd.Condition{cond}},
			ObservedGeneration: 3,
		},
	}
	hostList := &HostList{Items: []Host{*host}}

	mig := &Migration{
		ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "ns"},
		Spec: MigrationSpec{
			Plan:    corev1.ObjectReference{Name: "p1", Namespace: "ns"},
			Cancel:  []refapi.Ref{{ID: "vm-1"}, {ID: ""}},
			Cutover: &metav1.Time{Time: metav1.Now().Time},
		},
		Status: MigrationStatus{
			Conditions:         libcnd.Conditions{List: []libcnd.Condition{cond}},
			ObservedGeneration: 1,
			VMs: []*planapi.VMStatus{
				{VM: planapi.VM{Ref: refapi.Ref{ID: "vm-1", Name: "n1"}}},
			},
		},
	}
	migList := &MigrationList{Items: []Migration{*mig}}

	netMap := &NetworkMap{
		ObjectMeta: metav1.ObjectMeta{Name: "nm1", Namespace: "ns"},
		Spec: NetworkMapSpec{
			Provider: providerapi.Pair{
				Source:      corev1.ObjectReference{Name: "src", Namespace: "ns"},
				Destination: corev1.ObjectReference{Name: "dst", Namespace: "ns"},
			},
			Map: []NetworkPair{
				{Source: refapi.Ref{ID: "n1", Type: "pod", Name: "ns1/net1"}, Destination: DestinationNetwork{Type: "pod"}},
				{Source: refapi.Ref{ID: "n2", Type: "multus", Namespace: "ns2", Name: "net2"}, Destination: DestinationNetwork{Type: "multus", Namespace: "t", Name: "tn"}},
			},
		},
		Status: MapStatus{
			Conditions:         libcnd.Conditions{List: []libcnd.Condition{cond}},
			ObservedGeneration: 9,
			Refs:               refapi.Refs{List: []refapi.Ref{{ID: "n1"}, {ID: "n2"}}},
		},
	}
	netMapList := &NetworkMapList{Items: []NetworkMap{*netMap}}

	osPop := &OpenstackVolumePopulator{
		ObjectMeta: metav1.ObjectMeta{Name: "osvp", Namespace: "ns"},
		Spec: OpenstackVolumePopulatorSpec{
			IdentityURL:     "https://id.example.invalid",
			SecretName:      "sec",
			ImageID:         "img",
			TransferNetwork: &corev1.ObjectReference{Name: "nad", Namespace: "ns"},
		},
		Status: OpenstackVolumePopulatorStatus{Progress: "10"},
	}
	osPopList := &OpenstackVolumePopulatorList{Items: []OpenstackVolumePopulator{*osPop}}

	ovPop := &OvirtVolumePopulator{
		ObjectMeta: metav1.ObjectMeta{Name: "ovvp", Namespace: "ns"},
		Spec: OvirtVolumePopulatorSpec{
			EngineURL:        "https://engine.example.invalid",
			EngineSecretName: "sec",
			DiskID:           "disk1",
			TransferNetwork:  &corev1.ObjectReference{Name: "nad", Namespace: "ns"},
		},
		Status: OvirtVolumePopulatorStatus{Progress: "11"},
	}
	ovPopList := &OvirtVolumePopulatorList{Items: []OvirtVolumePopulator{*ovPop}}

	p := &Plan{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"},
		Spec: PlanSpec{
			TargetNamespace:                "tns",
			Warm:                           true,
			Archived:                       true,
			PreserveClusterCPUModel:        true,
			PreserveStaticIPs:              true,
			PVCNameTemplate:                "{{.VmName}}-{{.DiskIndex}}",
			PVCNameTemplateUseGenerateName: true,
			VolumeNameTemplate:             "disk-{{.VolumeIndex}}",
			NetworkNameTemplate:            "net-{{.NetworkIndex}}",
			MigrateSharedDisks:             true,
			DeleteGuestConversionPod:       true,
			InstallLegacyDrivers:           boolPtr(true),
			SkipGuestConversion:            false,
			UseCompatibilityMode:           true,
			TransferNetwork:                &corev1.ObjectReference{Name: "nad", Namespace: "ns"},
			Provider: providerapi.Pair{
				Source:      corev1.ObjectReference{Name: "src", Namespace: "ns"},
				Destination: corev1.ObjectReference{Name: "dst", Namespace: "ns"},
			},
			Map: planapi.Map{
				Network: corev1.ObjectReference{Name: "nm", Namespace: "ns"},
				Storage: corev1.ObjectReference{Name: "sm", Namespace: "ns"},
			},
			VMs: []planapi.VM{
				{
					Ref: refapi.Ref{ID: "vm-1", Name: "vm1"},
					Hooks: []planapi.HookRef{
						{Step: "PreHook", Hook: corev1.ObjectReference{Name: "h1", Namespace: "ns"}},
					},
					TargetName: "vm1-new",
				},
			},
		},
		Status: PlanStatus{
			Conditions:         libcnd.Conditions{List: []libcnd.Condition{cond}},
			ObservedGeneration: 5,
			Migration:          planapi.MigrationStatus{},
		},
	}
	pList := &PlanList{Items: []Plan{*p}}

	prov := &Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "pr1", Namespace: "ns"},
		Spec: ProviderSpec{
			Type:   &vs,
			URL:    "https://vs.example.invalid",
			Secret: corev1.ObjectReference{Name: "sec", Namespace: "ns"},
			Settings: map[string]string{
				UseVddkAioOptimization: "true",
			},
			ConvertDisk: boolPtr(true),
		},
		Status: ProviderStatus{
			Phase:              "Ready",
			Conditions:         libcnd.Conditions{List: []libcnd.Condition{cond}},
			ObservedGeneration: 1,
			Fingerprint:        "fp",
		},
	}
	provList := &ProviderList{Items: []Provider{*prov}}

	storageMap := &StorageMap{
		ObjectMeta: metav1.ObjectMeta{Name: "sm1", Namespace: "ns"},
		Spec: StorageMapSpec{
			Provider: providerapi.Pair{
				Source:      corev1.ObjectReference{Name: "src", Namespace: "ns"},
				Destination: corev1.ObjectReference{Name: "dst", Namespace: "ns"},
			},
			Map: []StoragePair{
				{
					Source: refapi.Ref{ID: "s1", Name: "ds1"},
					Destination: DestinationStorage{
						StorageClass: "sc1",
						VolumeMode:   corev1.PersistentVolumeFilesystem,
						AccessMode:   corev1.ReadWriteOnce,
					},
					OffloadPlugin: &OffloadPlugin{
						VSphereXcopyPluginConfig: &VSphereXcopyPluginConfig{
							SecretRef:            "sref",
							StorageVendorProduct: StorageVendorProductOntap,
						},
					},
				},
			},
		},
		Status: MapStatus{
			Conditions:         libcnd.Conditions{List: []libcnd.Condition{cond}},
			ObservedGeneration: 1,
			Refs:               refapi.Refs{List: []refapi.Ref{{ID: "s1"}}},
		},
	}
	storageMapList := &StorageMapList{Items: []StorageMap{*storageMap}}

	vx := &VSphereXcopyVolumePopulator{
		ObjectMeta: metav1.ObjectMeta{Name: "vx", Namespace: "ns"},
		Spec: VSphereXcopyVolumePopulatorSpec{
			VmId:                 "vm-1",
			VmdkPath:             "[ds] vm/disk.vmdk",
			SecretName:           "sec",
			StorageVendorProduct: "ontap",
		},
		Status: VSphereXcopyVolumePopulatorStatus{Progress: "12"},
	}
	vxList := &VSphereXcopyVolumePopulatorList{Items: []VSphereXcopyVolumePopulator{*vx}}

	// Exercise DeepCopy for the remaining simple structs too.
	if (&DestinationNetwork{Type: "pod", Namespace: "ns", Name: "n"}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy DestinationNetwork")
	}
	if (&DestinationStorage{StorageClass: "sc"}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy DestinationStorage")
	}
	if (&NetworkPair{Source: refapi.Ref{ID: "x"}, Destination: DestinationNetwork{Type: "pod"}}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy NetworkPair")
	}
	if (&StoragePair{Source: refapi.Ref{ID: "x"}, Destination: DestinationStorage{StorageClass: "sc"}}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy StoragePair")
	}
	if (&MapStatus{ObservedGeneration: 1}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy MapStatus")
	}
	if (&NetworkNameTemplateData{NetworkName: "n", NetworkNamespace: "ns", NetworkType: "Pod", NetworkIndex: 1}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy NetworkNameTemplateData")
	}
	if (&PVCNameTemplateData{VmName: "vm", PlanName: "p", DiskIndex: 0, RootDiskIndex: 0, Shared: true}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy PVCNameTemplateData")
	}
	if (&VolumeNameTemplateData{PVCName: "pvc", VolumeIndex: 1}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy VolumeNameTemplateData")
	}

	// Exercise ProviderType pointers in DeepCopy paths via distinct Providers.
	if (&Provider{Spec: ProviderSpec{Type: &ov}}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy Provider (ovirt)")
	}
	if (&Provider{Spec: ProviderSpec{Type: &osType}}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy Provider (openstack)")
	}
	if (&Provider{Spec: ProviderSpec{Type: &ova}}).DeepCopy() == nil {
		t.Fatalf("expected deepcopy Provider (ova)")
	}

	// DeepCopyObject for runtime.Object implementations.
	objects := []runtime.Object{
		hook, hookList,
		host, hostList,
		mig, migList,
		netMap, netMapList,
		osPop, osPopList,
		ovPop, ovPopList,
		p, pList,
		prov, provList,
		storageMap, storageMapList,
		vx, vxList,
	}
	for _, obj := range objects {
		if obj == nil {
			t.Fatalf("unexpected nil object")
		}
		cp := obj.DeepCopyObject()
		if cp == nil {
			t.Fatalf("DeepCopyObject returned nil for %T", obj)
		}
		if cp == obj {
			t.Fatalf("DeepCopyObject returned same pointer for %T", obj)
		}
	}

	// Nil receiver safety on a few representative types.
	var nilHook *Hook
	if nilHook.DeepCopy() != nil {
		t.Fatalf("expected nil deepcopy for nil receiver")
	}
	var nilProv *Provider
	if nilProv.DeepCopyObject() != nil {
		t.Fatalf("expected nil deepcopyobject for nil receiver")
	}
}

func TestGeneratedDeepCopy_V1beta1RemainingTypes(t *testing.T) {
	// This test exists specifically to hit the remaining generated DeepCopy/DeepCopyInto
	// functions for simple Spec/Status structs that aren't exercised deeply by parent objects.
	var (
		_ = (&HookSpec{Image: "i", Playbook: "p", Deadline: 1}).DeepCopy()
		_ = (&HookStatus{ObservedGeneration: 1}).DeepCopy()
		_ = (&HostSpec{IpAddress: "1.2.3.4"}).DeepCopy()
		_ = (&HostStatus{ObservedGeneration: 1}).DeepCopy()
		_ = (&OpenstackVolumePopulatorSpec{IdentityURL: "u", SecretName: "s", ImageID: "i"}).DeepCopy()
		_ = (&OpenstackVolumePopulatorStatus{Progress: "1"}).DeepCopy()
		_ = (&OvirtVolumePopulatorSpec{EngineURL: "u", EngineSecretName: "s", DiskID: "d"}).DeepCopy()
		_ = (&OvirtVolumePopulatorStatus{Progress: "1"}).DeepCopy()
		_ = (&PlanSpec{TargetNamespace: "t"}).DeepCopy()
		_ = (&PlanStatus{ObservedGeneration: 1}).DeepCopy()
		_ = (&ProviderSpec{URL: "u", Settings: map[string]string{"k": "v"}}).DeepCopy()
		_ = (&ProviderStatus{Phase: "p", ObservedGeneration: 1}).DeepCopy()
	)

	// Also explicitly call DeepCopyInto for a couple of these to guarantee the function bodies execute.
	in := &HookSpec{Image: "i"}
	out := &HookSpec{}
	in.DeepCopyInto(out)
	if out.Image != "i" {
		t.Fatalf("unexpected deepcopyinto: %#v", out)
	}
}
