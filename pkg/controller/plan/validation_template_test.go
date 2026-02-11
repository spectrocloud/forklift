package plan

import "testing"

func TestReconciler_IsValidTemplate(t *testing.T) {
	r := &Reconciler{}

	t.Run("syntax error", func(t *testing.T) {
		if _, err := r.IsValidTemplate("{{", map[string]any{}); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("empty output invalid", func(t *testing.T) {
		if _, err := r.IsValidTemplate("{{- /*empty*/ -}}", map[string]any{}); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("valid output", func(t *testing.T) {
		got, err := r.IsValidTemplate("x-{{ .A }}", map[string]any{"A": "y"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "x-y" {
			t.Fatalf("expected x-y, got %q", got)
		}
	})
}

func TestReconciler_NameTemplates_And_TargetName(t *testing.T) {
	r := &Reconciler{}

	// Empty template is allowed (means "use default behavior").
	if err := r.IsValidPVCNameTemplate(""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.IsValidVolumeNameTemplate(""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.IsValidNetworkNameTemplate(""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Valid templates.
	if err := r.IsValidPVCNameTemplate("{{ .VmName }}-{{ .DiskIndex }}"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.IsValidVolumeNameTemplate("{{ .PVCName }}-{{ .VolumeIndex }}"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.IsValidNetworkNameTemplate("{{ .NetworkName }}-{{ .NetworkIndex }}"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invalid outputs: uppercase violates DNS1123 label rules.
	if err := r.IsValidPVCNameTemplate("Bad"); err == nil {
		t.Fatalf("expected error")
	}
	if err := r.IsValidVolumeNameTemplate("Bad"); err == nil {
		t.Fatalf("expected error")
	}
	if err := r.IsValidNetworkNameTemplate("Bad"); err == nil {
		t.Fatalf("expected error")
	}

	// Target name: empty ok, invalid subdomain should error.
	if err := r.IsValidTargetName(""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.IsValidTargetName("bad_name"); err == nil {
		t.Fatalf("expected error")
	}
}
