package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileSystemImpl_ReadWriteStatSymlink(t *testing.T) {
	fs := FileSystemImpl{}
	dir := t.TempDir()

	p := filepath.Join(dir, "f.txt")
	if err := fs.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := fs.Stat(p); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	entries, err := fs.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("ReadDir: err=%v entries=%d", err, len(entries))
	}

	link := filepath.Join(dir, "ln")
	if err := fs.Symlink(p, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("Lstat link: %v", err)
	}
}

func TestConvertMockDirEntryToOs(t *testing.T) {
	in := []MockDirEntry{{FileName: "a", FileIsDir: false}, {FileName: "d", FileIsDir: true}}
	out := ConvertMockDirEntryToOs(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries")
	}
	if out[0].Name() != "a" || out[1].IsDir() != true {
		t.Fatalf("unexpected values")
	}
}
