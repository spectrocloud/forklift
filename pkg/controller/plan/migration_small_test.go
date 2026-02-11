package plan

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	web "github.com/kubev2v/forklift/pkg/controller/provider/web"
	webbase "github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type stubWebClient struct {
	calls int
}

var _ web.Client = (*stubWebClient)(nil)

func (s *stubWebClient) Finder() web.Finder                              { return nil }
func (s *stubWebClient) Get(resource interface{}, id string) error       { panic("unused") }
func (s *stubWebClient) List(list interface{}, param ...web.Param) error { panic("unused") }
func (s *stubWebClient) Watch(resource interface{}, h web.EventHandler) (*web.Watch, error) {
	panic("unused")
}
func (s *stubWebClient) Find(resource interface{}, ref webbase.Ref) error { panic("unused") }
func (s *stubWebClient) VM(r *webbase.Ref) (interface{}, error) {
	s.calls++
	if r != nil && r.ID == "" {
		r.ID = "resolved"
	}
	return &struct{}{}, nil
}
func (s *stubWebClient) Workload(ref *webbase.Ref) (interface{}, error) { panic("unused") }
func (s *stubWebClient) Network(ref *webbase.Ref) (interface{}, error)  { panic("unused") }
func (s *stubWebClient) Storage(ref *webbase.Ref) (interface{}, error)  { panic("unused") }
func (s *stubWebClient) Host(ref *webbase.Ref) (interface{}, error)     { panic("unused") }

func TestMigration_resolveCanceledRefs_CallsInventoryVM(t *testing.T) {
	inv := &stubWebClient{}
	m := &Migration{
		Context: &plancontext.Context{
			Source: plancontext.Source{Inventory: inv},
			Migration: &api.Migration{
				Spec: api.MigrationSpec{
					Cancel: []ref.Ref{{Name: "vm1"}, {ID: "x"}},
				},
			},
		},
	}
	m.resolveCanceledRefs()
	if inv.calls != 2 {
		t.Fatalf("expected 2 VM() calls, got %d", inv.calls)
	}
	if m.Context.Migration.Spec.Cancel[0].ID == "" {
		t.Fatalf("expected first cancel ref to be resolved in-place")
	}
}

func TestMigration_runningVMs_FiltersRunningOnly(t *testing.T) {
	vm1 := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "1"}}}
	vm2 := &planapi.VMStatus{VM: planapi.VM{Ref: ref.Ref{ID: "2"}}}
	vm1.MarkStarted()
	vm2.MarkCompleted()

	m := &Migration{
		Context: &plancontext.Context{
			Plan: &api.Plan{
				ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
				Status: api.PlanStatus{
					Migration: planapi.MigrationStatus{
						VMs: []*planapi.VMStatus{vm1, vm2},
					},
				},
			},
		},
	}

	got := m.runningVMs()
	if len(got) != 1 || got[0].Ref.ID != "1" {
		t.Fatalf("unexpected runningVMs result: %#v", got)
	}
}
