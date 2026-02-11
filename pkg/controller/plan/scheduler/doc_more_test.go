package scheduler

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	"github.com/kubev2v/forklift/pkg/controller/plan/scheduler/ocp"
	"github.com/kubev2v/forklift/pkg/controller/plan/scheduler/openstack"
	"github.com/kubev2v/forklift/pkg/controller/plan/scheduler/ova"
	"github.com/kubev2v/forklift/pkg/controller/plan/scheduler/ovirt"
	"github.com/kubev2v/forklift/pkg/controller/plan/scheduler/vsphere"
	"github.com/kubev2v/forklift/pkg/settings"
)

func TestNew_VSphere(t *testing.T) {
	old := settings.Settings.MaxInFlight
	t.Cleanup(func() { settings.Settings.MaxInFlight = old })
	settings.Settings.MaxInFlight = 7

	tp := api.VSphere
	ctx := &plancontext.Context{}
	ctx.Source.Provider = &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	s, err := New(ctx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := s.(*vsphere.Scheduler); !ok {
		t.Fatalf("expected vsphere scheduler got %T", s)
	}
	if s.(*vsphere.Scheduler).MaxInFlight != 7 {
		t.Fatalf("expected max=7 got %d", s.(*vsphere.Scheduler).MaxInFlight)
	}
}

func TestNew_OVirt(t *testing.T) {
	old := settings.Settings.MaxInFlight
	t.Cleanup(func() { settings.Settings.MaxInFlight = old })
	settings.Settings.MaxInFlight = 3

	tp := api.OVirt
	ctx := &plancontext.Context{}
	ctx.Source.Provider = &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	s, err := New(ctx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := s.(*ovirt.Scheduler); !ok {
		t.Fatalf("expected ovirt scheduler got %T", s)
	}
	if s.(*ovirt.Scheduler).MaxInFlight != 3 {
		t.Fatalf("expected max=3 got %d", s.(*ovirt.Scheduler).MaxInFlight)
	}
}

func TestNew_OpenStack(t *testing.T) {
	old := settings.Settings.MaxInFlight
	t.Cleanup(func() { settings.Settings.MaxInFlight = old })
	settings.Settings.MaxInFlight = 9

	tp := api.OpenStack
	ctx := &plancontext.Context{}
	ctx.Source.Provider = &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	s, err := New(ctx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := s.(*openstack.Scheduler); !ok {
		t.Fatalf("expected openstack scheduler got %T", s)
	}
	if s.(*openstack.Scheduler).MaxInFlight != 9 {
		t.Fatalf("expected max=9 got %d", s.(*openstack.Scheduler).MaxInFlight)
	}
}

func TestNew_OpenShift(t *testing.T) {
	old := settings.Settings.MaxInFlight
	t.Cleanup(func() { settings.Settings.MaxInFlight = old })
	settings.Settings.MaxInFlight = 2

	tp := api.OpenShift
	ctx := &plancontext.Context{}
	ctx.Source.Provider = &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	s, err := New(ctx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := s.(*ocp.Scheduler); !ok {
		t.Fatalf("expected ocp scheduler got %T", s)
	}
	if s.(*ocp.Scheduler).MaxInFlight != 2 {
		t.Fatalf("expected max=2 got %d", s.(*ocp.Scheduler).MaxInFlight)
	}
}

func TestNew_Ova(t *testing.T) {
	old := settings.Settings.MaxInFlight
	t.Cleanup(func() { settings.Settings.MaxInFlight = old })
	settings.Settings.MaxInFlight = 5

	tp := api.Ova
	ctx := &plancontext.Context{}
	ctx.Source.Provider = &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	s, err := New(ctx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := s.(*ova.Scheduler); !ok {
		t.Fatalf("expected ova scheduler got %T", s)
	}
	if s.(*ova.Scheduler).MaxInFlight != 5 {
		t.Fatalf("expected max=5 got %d", s.(*ova.Scheduler).MaxInFlight)
	}
}

func TestNew_UnsupportedProvider_ReturnsError(t *testing.T) {
	tp := api.ProviderType("nope")
	ctx := &plancontext.Context{}
	ctx.Source.Provider = &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	_, err := New(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
}
