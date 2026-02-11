package apis

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddToScheme(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
