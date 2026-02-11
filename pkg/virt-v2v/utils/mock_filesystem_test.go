package utils

import (
	"os"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestMockFileSystem_Methods(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	fs := NewMockFileSystem(ctrl)

	fs.EXPECT().ReadDir("/tmp").Return([]os.DirEntry{}, nil)
	fs.EXPECT().Stat("/tmp/file").Return(nil, os.ErrNotExist)
	fs.EXPECT().Symlink("old", "new").Return(nil)

	if _, err := fs.ReadDir("/tmp"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := fs.Stat("/tmp/file"); err == nil {
		t.Fatalf("expected error")
	}
	if err := fs.Symlink("old", "new"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
