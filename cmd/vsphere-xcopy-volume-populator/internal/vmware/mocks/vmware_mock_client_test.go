package vmware_mocks

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestMockClient_Methods(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	m := NewMockClient(ctrl)
	ctx := context.Background()

	m.EXPECT().GetDatastore(gomock.Any(), gomock.Any(), "ds").Return(nil, nil)
	m.EXPECT().GetEsxByVm(gomock.Any(), "vm").Return(nil, nil)
	m.EXPECT().RunEsxCommand(gomock.Any(), gomock.Any(), []string{"ls"}).Return(nil, nil)

	if _, err := m.GetDatastore(ctx, nil, "ds"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := m.GetEsxByVm(ctx, "vm"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := m.RunEsxCommand(ctx, nil, []string{"ls"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}


