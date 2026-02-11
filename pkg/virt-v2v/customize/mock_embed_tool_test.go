package customize

import (
	"testing"

	"go.uber.org/mock/gomock"
)

func TestMockEmbedTool_CreateFilesFromFS(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	m := NewMockEmbedTool(ctrl)
	m.EXPECT().CreateFilesFromFS("/tmp/out").Return(nil)

	if err := m.CreateFilesFromFS("/tmp/out"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
