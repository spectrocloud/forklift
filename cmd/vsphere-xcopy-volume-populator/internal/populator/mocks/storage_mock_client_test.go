package storage_mocks

import (
	"testing"

	populator "github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/populator"
	"go.uber.org/mock/gomock"
)

func TestMockStorageApi_Methods(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	m := NewMockStorageApi(ctrl)

	var (
		lun     populator.LUN
		ctx     populator.MappingContext
		pv      populator.PersistentVolume
		mapped  populator.LUN
		groups  []string
		mapCtx  populator.MappingContext
	)

	m.EXPECT().CurrentMappedGroups(lun, ctx).Return(groups, nil)
	m.EXPECT().EnsureClonnerIgroup("ig", []string{"iqn1"}).Return(mapCtx, nil)
	m.EXPECT().Map("ig", lun, ctx).Return(mapped, nil)
	m.EXPECT().ResolvePVToLUN(pv).Return(lun, nil)
	m.EXPECT().UnMap("ig", lun, ctx).Return(nil)

	if _, err := m.CurrentMappedGroups(lun, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := m.EnsureClonnerIgroup("ig", []string{"iqn1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := m.Map("ig", lun, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := m.ResolvePVToLUN(pv); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := m.UnMap("ig", lun, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}


