package migrator

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	"github.com/kubev2v/forklift/pkg/lib/logging"
)

func TestNew_UnsupportedProviderType_ReturnsError(t *testing.T) {
	// Provider type Undefined => adapter.New fails in base migrator Init().
	p := &api.Plan{}
	pt := api.Undefined
	src := &api.Provider{}
	src.Spec.Type = &pt
	p.Provider.Source = src
	p.Referenced.Provider.Source = src

	ctx := &plancontext.Context{Plan: p}
	ctx.Source.Provider = src
	ctx.Log = logging.WithName("test")

	_, err := New(ctx)
	if err == nil {
		t.Fatalf("expected err")
	}
}
