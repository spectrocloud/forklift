package utils

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestMockCommandExecutor_RunStartWaitAndStreams(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	exec := NewMockCommandExecutor(ctrl)

	exec.EXPECT().SetStdout(gomock.Any())
	exec.EXPECT().SetStderr(gomock.Any())
	exec.EXPECT().SetStdin(gomock.Any())
	exec.EXPECT().Start().Return(nil)
	exec.EXPECT().Wait().Return(nil)
	exec.EXPECT().Run().Return(nil)

	exec.SetStdout(&bytes.Buffer{})
	exec.SetStderr(&bytes.Buffer{})
	exec.SetStdin(bytes.NewReader([]byte("in")))
	if err := exec.Start(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := exec.Wait(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := exec.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockCommandBuilder_ChainingAndBuild(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	b := NewMockCommandBuilder(ctrl)
	exec := NewMockCommandExecutor(ctrl)

	gomock.InOrder(
		b.EXPECT().New("virt-v2v").Return(b),
		b.EXPECT().AddFlag("--verbose").Return(b),
		b.EXPECT().AddArg("--name", "vm1").Return(b),
		b.EXPECT().AddArgs("--net", "n1", "n2").Return(b),
		b.EXPECT().AddExtraArgs("--extra1", "--extra2").Return(b),
		b.EXPECT().AddPositional("pos").Return(b),
		b.EXPECT().Build().Return(exec),
		exec.EXPECT().Run().Return(nil),
	)

	ce := b.
		New("virt-v2v").
		AddFlag("--verbose").
		AddArg("--name", "vm1").
		AddArgs("--net", "n1", "n2").
		AddExtraArgs("--extra1", "--extra2").
		AddPositional("pos").
		Build()
	if err := ce.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
