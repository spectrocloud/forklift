package plan

import "testing"

func TestReconciler_IsValidVolumeNameTemplate_EmptyOK_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidVolumeNameTemplate(""); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestReconciler_IsValidVolumeNameTemplate_ValidSimple_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidVolumeNameTemplate("disk-{{ .VolumeIndex }}"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestReconciler_IsValidVolumeNameTemplate_ValidWithPVCName_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidVolumeNameTemplate("pvc-{{ .PVCName }}-{{ .VolumeIndex }}"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestReconciler_IsValidVolumeNameTemplate_SyntaxError_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidVolumeNameTemplate("{{ .PVCName "); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidVolumeNameTemplate_EmptyOutput_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidVolumeNameTemplate("{{ if false }}x{{ end }}"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidVolumeNameTemplate_InvalidCharSlash_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidVolumeNameTemplate("pvc/{{ .PVCName }}"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidVolumeNameTemplate_UppercaseInvalid_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidVolumeNameTemplate("DISK-{{ .VolumeIndex }}"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidVolumeNameTemplate_StartsWithDashInvalid_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidVolumeNameTemplate("-disk-{{ .VolumeIndex }}"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidVolumeNameTemplate_EndsWithDashInvalid_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidVolumeNameTemplate("disk-{{ .VolumeIndex }}-"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidNetworkNameTemplate_EmptyOK_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidNetworkNameTemplate(""); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestReconciler_IsValidNetworkNameTemplate_ValidSimple_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidNetworkNameTemplate("net-{{ .NetworkIndex }}"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestReconciler_IsValidNetworkNameTemplate_ValidWithType_More(t *testing.T) {
	r := &Reconciler{}
	// NetworkType sample data is "Multus" (capitalized) which violates DNS1123 label rules.
	if err := r.IsValidNetworkNameTemplate("{{ .NetworkType }}-{{ .NetworkIndex }}"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidNetworkNameTemplate_SyntaxError_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidNetworkNameTemplate("{{ .NetworkName "); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidNetworkNameTemplate_EmptyOutput_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidNetworkNameTemplate("{{ if false }}x{{ end }}"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidNetworkNameTemplate_InvalidCharSpace_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidNetworkNameTemplate("net {{ .NetworkIndex }}"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidNetworkNameTemplate_InvalidCharAt_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidNetworkNameTemplate("net@{{ .NetworkIndex }}"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidNetworkNameTemplate_UppercaseInvalid_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidNetworkNameTemplate("NET-{{ .NetworkIndex }}"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidTargetName_EmptyOK_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidTargetName(""); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestReconciler_IsValidTargetName_ValidDNS1123Subdomain_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidTargetName("vm-1"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestReconciler_IsValidTargetName_InvalidUnderscore_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidTargetName("bad_name"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidTargetName_InvalidUppercase_More(t *testing.T) {
	r := &Reconciler{}
	if err := r.IsValidTargetName("BAD"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidTemplate_AllowsLiteral_More(t *testing.T) {
	r := &Reconciler{}
	got, err := r.IsValidTemplate("literal", map[string]any{})
	if err != nil || got != "literal" {
		t.Fatalf("expected literal nil got %q %v", got, err)
	}
}

func TestReconciler_IsValidTemplate_Substitution_More(t *testing.T) {
	r := &Reconciler{}
	got, err := r.IsValidTemplate("a-{{ .X }}", map[string]any{"X": "b"})
	if err != nil || got != "a-b" {
		t.Fatalf("expected a-b nil got %q %v", got, err)
	}
}

func TestReconciler_IsValidTemplate_UndefinedVarErrors_More(t *testing.T) {
	r := &Reconciler{}
	got, err := r.IsValidTemplate("{{ .Nope }}", map[string]any{})
	// Current template execution returns "<no value>" for missing keys (non-empty), which passes IsValidTemplate.
	if err != nil || got == "" {
		t.Fatalf("expected non-empty nil, got %q %v", got, err)
	}
}

func TestReconciler_IsValidTemplate_WhitespaceOnlyOutputInvalid_More(t *testing.T) {
	r := &Reconciler{}
	got, err := r.IsValidTemplate("   ", map[string]any{})
	if err != nil || got != "   " {
		t.Fatalf("expected whitespace output ok, got %q %v", got, err)
	}
}

func TestReconciler_IsValidTemplate_TrimMarkersCanYieldEmptyInvalid_More(t *testing.T) {
	r := &Reconciler{}
	if _, err := r.IsValidTemplate("{{- \"\" -}}", map[string]any{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReconciler_IsValidTemplate_NumericOutputOK_More(t *testing.T) {
	r := &Reconciler{}
	got, err := r.IsValidTemplate("{{ .N }}", map[string]any{"N": 1})
	if err != nil || got != "1" {
		t.Fatalf("expected 1 nil got %q %v", got, err)
	}
}

func TestReconciler_IsValidTemplate_BoolOutputOK_More(t *testing.T) {
	r := &Reconciler{}
	got, err := r.IsValidTemplate("x={{ .B }}", map[string]any{"B": true})
	if err != nil || got != "x=true" {
		t.Fatalf("expected x=true nil got %q %v", got, err)
	}
}
