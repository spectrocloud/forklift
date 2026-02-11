package main

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
)

func TestSensitiveInfo(t *testing.T) {
	for _, tc := range []struct {
		option string
		want   bool
	}{
		{"password", true},
		{"applicationCredentialSecret", true},
		{"token", true},
		{"username", false},
		{"regionName", false},
	} {
		if got := sensitiveInfo(tc.option); got != tc.want {
			t.Fatalf("sensitiveInfo(%q)=%v, want %v", tc.option, got, tc.want)
		}
	}
}

func TestReadOptions_PreservesValues(t *testing.T) {
	t.Setenv("regionName", "region-1")
	t.Setenv("username", "u1")
	t.Setenv("password", "p1")
	t.Setenv("token", "t1")
	t.Setenv("insecureSkipVerify", "true")

	opts := readOptions()
	if opts["regionName"] != "region-1" {
		t.Fatalf("regionName: expected %q, got %q", "region-1", opts["regionName"])
	}
	if opts["username"] != "u1" {
		t.Fatalf("username: expected %q, got %q", "u1", opts["username"])
	}
	if opts["password"] != "p1" {
		t.Fatalf("password: expected %q, got %q", "p1", opts["password"])
	}
	if opts["token"] != "t1" {
		t.Fatalf("token: expected %q, got %q", "t1", opts["token"])
	}
	if _, ok := opts["applicationCredentialID"]; !ok {
		t.Fatalf("expected applicationCredentialID key to exist")
	}
}

func TestCountingReader_ReadCountsBytes(t *testing.T) {
	r := io.NopCloser(strings.NewReader("hello world"))
	read := int64(0)
	cr := &CountingReader{reader: r, total: 100, read: &read}

	buf := make([]byte, 5)
	n, err := cr.Read(buf)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 5 || string(buf) != "hello" {
		t.Fatalf("unexpected read: n=%d buf=%q", n, string(buf))
	}
	if read != 5 {
		t.Fatalf("expected read=5, got %d", read)
	}
}

func TestUpdateProgress_TotalZero_NoOp(t *testing.T) {
	progress := createProgressCounter()
	read := int64(10)
	cr := &CountingReader{read: &read, total: 0}
	updateProgress(cr, progress, "uid")
}

func TestUpdateProgress_AdvancesCounter(t *testing.T) {
	progress := createProgressCounter()
	read := int64(50)
	cr := &CountingReader{read: &read, total: 100}

	updateProgress(cr, progress, "uid")
	metric := &dto.Metric{}
	_ = progress.WithLabelValues("uid").Write(metric)
	if metric.Counter == nil || metric.Counter.Value == nil {
		t.Fatalf("expected counter metric")
	}
	if math.Abs(*metric.Counter.Value-50.0) > 0.001 {
		t.Fatalf("expected ~50, got %v", *metric.Counter.Value)
	}

	// Advance to 60% and ensure it only adds the delta.
	read = 60
	updateProgress(cr, progress, "uid")
	metric2 := &dto.Metric{}
	_ = progress.WithLabelValues("uid").Write(metric2)
	if math.Abs(*metric2.Counter.Value-60.0) > 0.001 {
		t.Fatalf("expected ~60, got %v", *metric2.Counter.Value)
	}
}

func TestFinalizeProgress_AdvancesTo100(t *testing.T) {
	progress := createProgressCounter()
	progress.WithLabelValues("uid").Add(20)

	finalizeProgress(progress, "uid")

	metric := &dto.Metric{}
	_ = progress.WithLabelValues("uid").Write(metric)
	if math.Abs(*metric.Counter.Value-100.0) > 0.001 {
		t.Fatalf("expected ~100, got %v", *metric.Counter.Value)
	}
}

func TestOpenFile_DiskImg_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "disk.img")
	f := openFile(p)
	t.Cleanup(func() { _ = f.Close() })

	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestOpenFile_NonDiskImg_OpensExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "volume.bin")
	if err := os.WriteFile(p, []byte("x"), 0o640); err != nil {
		t.Fatalf("write: %v", err)
	}

	f := openFile(p)
	t.Cleanup(func() { _ = f.Close() })
}
