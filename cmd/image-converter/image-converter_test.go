package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFakeQemuImg(t *testing.T, dir string) (logPath string) {
	t.Helper()

	logPath = filepath.Join(dir, "qemu-img.log")
	script := `#!/bin/sh
set -eu
last=""
for a in "$@"; do last="$a"; done
if [ "${QEMUIMG_EXIT:-0}" != "0" ]; then
  echo "forced failure" 1>&2
  exit "${QEMUIMG_EXIT}"
fi
if [ "${LOGFILE:-}" != "" ]; then
  echo "$@" >> "${LOGFILE}"
fi
# simulate output file creation to support convert()'s Filesystem mv path
if [ "${last}" != "" ]; then
  echo "fake" > "${last}"
fi
echo "progress 50%"
exit 0
`
	path := filepath.Join(dir, "qemu-img")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake qemu-img: %v", err)
	}
	return logPath
}

func TestQemuimgConvert_Success(t *testing.T) {
	tmp := t.TempDir()
	_ = writeFakeQemuImg(t, tmp)
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := qemuimgConvert("src", "dst", "raw", "qcow2"); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestQemuimgConvert_Failure(t *testing.T) {
	tmp := t.TempDir()
	_ = writeFakeQemuImg(t, tmp)
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("QEMUIMG_EXIT", "7")

	if err := qemuimgConvert("src", "dst", "raw", "qcow2"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestConvert_BlockMode_CallsQemuImgTwice(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "calls.log")
	_ = writeFakeQemuImg(t, tmp)
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LOGFILE", logPath)

	src := filepath.Join(tmp, "src.img")
	dst := filepath.Join(tmp, "dst.img")

	if err := convert(src, dst, "raw", "qcow2", "Block"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 qemu-img calls, got %d (%q)", len(lines), string(b))
	}
}

func TestConvert_FilesystemMode_MovesDstToSrc(t *testing.T) {
	tmp := t.TempDir()
	_ = writeFakeQemuImg(t, tmp)
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	src := filepath.Join(tmp, "src.img")
	dst := filepath.Join(tmp, "dst.img")

	if err := convert(src, dst, "raw", "qcow2", "Filesystem"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("expected src to exist after mv: %v", err)
	}
}

func TestConvert_UnknownMode_OnlyRunsConvert(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "calls.log")
	_ = writeFakeQemuImg(t, tmp)
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LOGFILE", logPath)

	src := filepath.Join(tmp, "src.img")
	dst := filepath.Join(tmp, "dst.img")

	if err := convert(src, dst, "raw", "qcow2", ""); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 qemu-img call, got %d (%q)", len(lines), string(b))
	}
}

func TestMain_SucceedsWithFakeQemuImg(t *testing.T) {
	old := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = old })
	flag.CommandLine = flag.NewFlagSet("image-converter-test", flag.ContinueOnError)

	tmp := t.TempDir()
	_ = writeFakeQemuImg(t, tmp)
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	src := filepath.Join(tmp, "src.img")
	dst := filepath.Join(tmp, "dst.img")
	// main() will mv dst over src in Filesystem mode; ensure src path exists (mv will replace it).
	if err := os.WriteFile(src, []byte("old"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{
		"image-converter",
		"-src-path=" + src,
		"-dst-path=" + dst,
		"-src-format=raw",
		"-dst-format=qcow2",
		"-volume-mode=Filesystem",
	}

	main()
}
