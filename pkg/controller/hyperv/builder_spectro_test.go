package hyperv

import "testing"

// Outside OpenShift no SCC assigns a UID, so the provider server pod must name
// one itself or kubelet rejects runAsNonRoot for the root-default image.
func TestSecurityContext_SetsNonRootUser(t *testing.T) {
	sc := (&Builder{}).securityContext()
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Fatalf("expected runAsNonRoot true")
	}
	if sc.RunAsUser == nil || *sc.RunAsUser != QEMUUser {
		t.Fatalf("expected runAsUser %d, got %v", QEMUUser, sc.RunAsUser)
	}
	if sc.RunAsGroup == nil || *sc.RunAsGroup != QEMUGroup {
		t.Fatalf("expected runAsGroup %d, got %v", QEMUGroup, sc.RunAsGroup)
	}
}
