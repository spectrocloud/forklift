package controller

import (
	"errors"
	"testing"

	"github.com/kubev2v/forklift/pkg/settings"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestAddToManager_InventoryRole_LoadsInventoryControllers(t *testing.T) {
	oldMain := MainControllers
	oldInv := InventoryControllers
	oldRoles := Settings.Role.Roles
	t.Cleanup(func() {
		MainControllers = oldMain
		InventoryControllers = oldInv
		Settings.Role.Roles = oldRoles
	})

	var invCalled, mainCalled int
	InventoryControllers = []AddFunction{
		func(m manager.Manager) error { invCalled++; return nil },
	}
	MainControllers = []AddFunction{
		func(m manager.Manager) error { mainCalled++; return nil },
	}

	Settings.Role.Roles = map[string]bool{
		settings.InventoryRole: true,
	}

	if err := AddToManager(nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if invCalled != 1 {
		t.Fatalf("expected inventory controllers called once, got %d", invCalled)
	}
	if mainCalled != 0 {
		t.Fatalf("expected main controllers not called, got %d", mainCalled)
	}
}

func TestAddToManager_MainRole_LoadsMainControllers(t *testing.T) {
	oldMain := MainControllers
	oldInv := InventoryControllers
	oldRoles := Settings.Role.Roles
	t.Cleanup(func() {
		MainControllers = oldMain
		InventoryControllers = oldInv
		Settings.Role.Roles = oldRoles
	})

	var invCalled, mainCalled int
	InventoryControllers = []AddFunction{
		func(m manager.Manager) error { invCalled++; return nil },
	}
	MainControllers = []AddFunction{
		func(m manager.Manager) error { mainCalled++; return nil },
		func(m manager.Manager) error { mainCalled++; return nil },
	}

	Settings.Role.Roles = map[string]bool{
		settings.MainRole: true,
	}

	if err := AddToManager(nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if invCalled != 0 {
		t.Fatalf("expected inventory controllers not called, got %d", invCalled)
	}
	if mainCalled != 2 {
		t.Fatalf("expected main controllers called twice, got %d", mainCalled)
	}
}

func TestAddToManager_StopsOnError(t *testing.T) {
	oldMain := MainControllers
	oldInv := InventoryControllers
	oldRoles := Settings.Role.Roles
	t.Cleanup(func() {
		MainControllers = oldMain
		InventoryControllers = oldInv
		Settings.Role.Roles = oldRoles
	})

	sentinel := errors.New("boom")
	var called int
	InventoryControllers = []AddFunction{
		func(m manager.Manager) error { called++; return sentinel },
		func(m manager.Manager) error { called++; return nil },
	}
	MainControllers = nil
	Settings.Role.Roles = map[string]bool{
		settings.InventoryRole: true,
	}

	err := AddToManager(nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected to stop after first error, called=%d", called)
	}
}
