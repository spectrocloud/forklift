package utils

import (
	"bytes"
	"os/exec"
	"reflect"
	"testing"
)

func TestCommandBuilderImpl_Build_ComposesArgs(t *testing.T) {
	cb := &CommandBuilderImpl{}
	cb.New("echo").
		AddFlag("-n").
		AddArg("x", "y").
		AddArg("skip", "").
		AddArgs("--k", "a", "", "b").
		AddPositional("p1").
		AddPositional("").
		AddExtraArgs("e1", "e2")

	if cb.BaseCommand != "echo" {
		t.Fatalf("unexpected base: %q", cb.BaseCommand)
	}
	if len(cb.Args) == 0 {
		t.Fatalf("expected args")
	}

	ce := cb.Build()
	if ce == nil {
		t.Fatalf("expected executor")
	}
	if _, ok := ce.(*Command); !ok {
		t.Fatalf("expected *Command executor, got %T", ce)
	}
}

func TestCommand_WiresStdIOAndRun(t *testing.T) {
	// Use a command that does nothing and always exits 0.
	c := &Command{cmd: exec.Command("true")}

	var b bytes.Buffer
	c.SetStdout(&b)
	c.SetStderr(&b)
	c.SetStdin(bytes.NewReader([]byte("in")))

	if err := c.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Just ensure the fields are set; output is empty for `true`.
	if c.cmd.Stdout == nil || c.cmd.Stderr == nil || c.cmd.Stdin == nil {
		t.Fatalf("expected stdio fields set")
	}
}

func TestCommandBuilderImpl_MethodsReturnSameBuilder(t *testing.T) {
	cb := &CommandBuilderImpl{}
	if reflect.ValueOf(cb.New("x")).Pointer() != reflect.ValueOf(cb).Pointer() {
		t.Fatalf("expected chaining on same builder")
	}
}
