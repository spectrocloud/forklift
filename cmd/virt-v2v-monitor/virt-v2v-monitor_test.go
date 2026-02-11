package main

import (
	"bufio"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestLimitedScanLines_TrimsAtMaxBuffer(t *testing.T) {
	data := make([]byte, bufio.MaxScanTokenSize)
	for i := range data {
		data[i] = 'a'
	}
	advance, token, err := LimitedScanLines(data, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if advance != len(data) {
		t.Fatalf("expected advance=%d got %d", len(data), advance)
	}
	if token == nil || len(token) != len(data) {
		t.Fatalf("expected full token, got len=%d", len(token))
	}
}

func TestUpdateProgress(t *testing.T) {
	cv := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_v2v_progress", Help: "x"},
		[]string{"disk_id"},
	)

	// disk=0 => no-op.
	if err := updateProgress(cv, 0, 50); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// increasing
	if err := updateProgress(cv, 1, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &dto.Metric{}
	if err := cv.WithLabelValues("1").Write(m); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if m.Counter == nil || m.Counter.Value == nil || *m.Counter.Value < 9.9 || *m.Counter.Value > 10.1 {
		t.Fatalf("expected ~10 got %#v", m.Counter)
	}

	// non-increasing should not subtract.
	if err := updateProgress(cv, 1, 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m2 := &dto.Metric{}
	if err := cv.WithLabelValues("1").Write(m2); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if m2.Counter == nil || m2.Counter.Value == nil || *m2.Counter.Value < 9.9 {
		t.Fatalf("expected still ~10 got %#v", m2.Counter)
	}
}
