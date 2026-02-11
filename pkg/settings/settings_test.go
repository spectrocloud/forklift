package settings

import (
	"testing"
)

func TestSettingsLoad_InventoryOnlyRole_SucceedsAndSetsDefaults(t *testing.T) {
	// Save/restore global to avoid cross-test pollution.
	prev := Settings
	t.Cleanup(func() { Settings = prev })

	// Avoid main-role required env vars in Migration.Load().
	t.Setenv(Roles, InventoryRole)

	// A few knobs to exercise parsing.
	t.Setenv(MetricsPort, "9090")
	t.Setenv(AuthRequired, "false")
	t.Setenv(OpenShift, "true")
	t.Setenv(Development, "true")

	if err := Settings.Load(); err != nil {
		t.Fatalf("Settings.Load() error: %v", err)
	}

	if !Settings.Role.Has(InventoryRole) {
		t.Fatalf("expected role %q enabled", InventoryRole)
	}
	if Settings.Role.Has(MainRole) {
		t.Fatalf("did not expect role %q enabled", MainRole)
	}
	if Settings.Metrics.Port != 9090 {
		t.Fatalf("expected Metrics.Port=9090, got %d", Settings.Metrics.Port)
	}
	if got := Settings.Metrics.Address(); got != ":9090" {
		t.Fatalf("expected Metrics.Address()=:9090, got %q", got)
	}
	if Settings.Inventory.AuthRequired {
		t.Fatalf("expected Inventory.AuthRequired=false")
	}
	if !Settings.OpenShift || !Settings.Development {
		t.Fatalf("expected OpenShift=true and Development=true")
	}
	if Settings.PolicyAgent.Enabled() {
		t.Fatalf("expected PolicyAgent.Enabled()=false when URL unset")
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Run("getEnvBool", func(t *testing.T) {
		t.Setenv("X_BOOL", "true")
		if got := getEnvBool("X_BOOL", false); got != true {
			t.Fatalf("expected true, got %v", got)
		}
		t.Setenv("X_BOOL", "not-a-bool")
		if got := getEnvBool("X_BOOL", true); got != true {
			t.Fatalf("expected default(true) on invalid bool, got %v", got)
		}
	})

	t.Run("getEnvLimit errors", func(t *testing.T) {
		t.Setenv("X_POS", "nope")
		if _, err := getPositiveEnvLimit("X_POS", 1); err == nil {
			t.Fatalf("expected error for non-integer")
		}
		t.Setenv("X_POS", "0")
		if _, err := getPositiveEnvLimit("X_POS", 1); err == nil {
			t.Fatalf("expected error for < minimum")
		}

		t.Setenv("X_NN", "-1")
		if _, err := getNonNegativeEnvLimit("X_NN", 1); err == nil {
			t.Fatalf("expected error for negative")
		}
	})
}

func TestGetVDDKImage(t *testing.T) {
	prev := Settings
	t.Cleanup(func() { Settings = prev })

	Settings.Migration.VddkImage = "fallback-img"

	if got := GetVDDKImage(map[string]string{"vddkInitImage": "spec-img"}); got != "spec-img" {
		t.Fatalf("expected provider spec image, got %q", got)
	}
	if got := GetVDDKImage(map[string]string{}); got != "fallback-img" {
		t.Fatalf("expected fallback image, got %q", got)
	}
}

func TestMigrationLoad_MainRole_RequiresCertainEnvVars(t *testing.T) {
	prev := Settings
	t.Cleanup(func() { Settings = prev })

	// Ensure main role is enabled so Migration.Load() enforces required env vars.
	t.Setenv(Roles, MainRole)

	// Minimal required values.
	t.Setenv(VirtCustomizeConfigMap, "virt-customize")
	t.Setenv(VirtV2vImage, "quay.io/example/virt-v2v:latest")
	t.Setenv(OvirtOsConfigMap, "ovirt-os-map")
	t.Setenv(VsphereOsConfigMap, "vsphere-os-map")

	// Exercise a couple parsing branches.
	t.Setenv(BlockOverhead, "1Gi")
	t.Setenv(VirtV2vExtraArgs, " -v  -x ")

	if err := Settings.Load(); err != nil {
		t.Fatalf("Settings.Load() error: %v", err)
	}
	if Settings.Migration.BlockOverhead <= 0 {
		t.Fatalf("expected BlockOverhead > 0")
	}
	if Settings.Migration.VirtV2vExtraArgs == "" || Settings.Migration.VirtV2vExtraArgs[0] != '[' {
		t.Fatalf("expected VirtV2vExtraArgs to be JSON array, got %q", Settings.Migration.VirtV2vExtraArgs)
	}
}

func TestMigrationLoad_InvalidBlockOverheadErrors(t *testing.T) {
	prev := Settings
	t.Cleanup(func() { Settings = prev })

	// Enable main role so required vars are enforced; set the minimum required ones.
	t.Setenv(Roles, MainRole)
	t.Setenv(VirtCustomizeConfigMap, "virt-customize")
	t.Setenv(VirtV2vImage, "quay.io/example/virt-v2v:latest")
	t.Setenv(OvirtOsConfigMap, "ovirt-os-map")
	t.Setenv(VsphereOsConfigMap, "vsphere-os-map")

	t.Setenv(BlockOverhead, "not-a-quantity")
	if err := Settings.Load(); err == nil {
		t.Fatalf("expected error for invalid %s", BlockOverhead)
	}
}

func TestMigrationLoad_MissingRequiredEnvVarsWhenMainRole(t *testing.T) {
	prev := Settings
	t.Cleanup(func() { Settings = prev })

	t.Setenv(Roles, MainRole)

	// Missing VirtCustomizeConfigMap should error.
	t.Setenv(VirtV2vImage, "quay.io/example/virt-v2v:latest")
	t.Setenv(OvirtOsConfigMap, "ovirt-os-map")
	t.Setenv(VsphereOsConfigMap, "vsphere-os-map")

	if err := Settings.Load(); err == nil {
		t.Fatalf("expected error for missing required env var %s", VirtCustomizeConfigMap)
	}
}
