package logging

import (
	"errors"
	"os"
	"testing"

	"github.com/go-logr/logr"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
)

type recordSink struct {
	infos  []recordEntry
	errors []recordEntry
}

type recordEntry struct {
	msg string
	kv  []interface{}
}

type unwrapNil struct{}

func (unwrapNil) Error() string { return "wrap" }
func (unwrapNil) Unwrap() error { return nil }

func (r *recordSink) Init(logr.RuntimeInfo) {}
func (r *recordSink) Enabled(_ int) bool    { return true }
func (r *recordSink) WithName(_ string) logr.LogSink {
	return r
}
func (r *recordSink) WithValues(kv ...interface{}) logr.LogSink {
	// keep a copy to avoid mutation surprises
	cp := append([]interface{}(nil), kv...)
	r.infos = append(r.infos, recordEntry{msg: "WithValues", kv: cp})
	return r
}
func (r *recordSink) Info(_ int, msg string, kv ...interface{}) {
	cp := append([]interface{}(nil), kv...)
	r.infos = append(r.infos, recordEntry{msg: msg, kv: cp})
}
func (r *recordSink) Error(err error, msg string, kv ...interface{}) {
	cp := append([]interface{}(nil), kv...)
	// include the error string in kv for easier assertions.
	cp = append(cp, "err", "")
	if err != nil {
		cp[len(cp)-1] = err.Error()
	}
	r.errors = append(r.errors, recordEntry{msg: msg, kv: cp})
}

func kvHasKey(kv []interface{}, key string) bool {
	for i := 0; i+1 < len(kv); i += 2 {
		if s, ok := kv[i].(string); ok && s == key {
			return true
		}
	}
	return false
}

func TestSettings_Load_FromEnv(t *testing.T) {
	t.Setenv(EnvDevelopment, "true")
	t.Setenv(EnvLevel, "7")
	var s _Settings
	s.Load()
	if !s.Development {
		t.Fatalf("expected development=true")
	}
	if s.Level != 7 {
		t.Fatalf("expected level=7, got %d", s.Level)
	}
	if !s.allowed(7) || s.allowed(8) {
		t.Fatalf("unexpected allowed behavior for level=%d", s.Level)
	}
	if !s.atDebug(10) || s.atDebug(1) {
		t.Fatalf("unexpected atDebug behavior (threshold=%d)", s.DebugThreshold)
	}
}

func TestLogger_Error_WrappedAndUnwrapped(t *testing.T) {
	prev := Settings
	t.Cleanup(func() { Settings = prev })
	Settings.Level = 10

	sink := &recordSink{}
	real := logr.New(sink)
	l := &Logger{Real: real, level: 0}

	t.Run("nil err is ignored", func(t *testing.T) {
		l.Error(nil, "ignored")
	})

	t.Run("wrapped liberr.Error logs via Info with stacktrace keys", func(t *testing.T) {
		e := liberr.New("boom")
		l.Error(e, "wrapped", "k", "v")
		if len(sink.infos) == 0 {
			t.Fatalf("expected at least one info log")
		}
		last := sink.infos[len(sink.infos)-1]
		if last.msg != "wrapped" {
			t.Fatalf("expected msg 'wrapped', got %q", last.msg)
		}
		if !kvHasKey(last.kv, Error) || !kvHasKey(last.kv, Stack) {
			t.Fatalf("expected %q and %q keys in kv, got: %#v", Error, Stack, last.kv)
		}
	})

	t.Run("unwrap-to-nil does nothing", func(t *testing.T) {
		l.Error(unwrapNil{}, "unwrap-nil")
	})

	t.Run("plain error uses Error()", func(t *testing.T) {
		l.Error(errors.New("x"), "plain", "a", "b")
		if len(sink.errors) == 0 {
			t.Fatalf("expected at least one error log")
		}
	})

	t.Run("not allowed skips logging", func(t *testing.T) {
		Settings.Level = -1
		before := len(sink.errors) + len(sink.infos)
		l.Error(os.ErrInvalid, "skip")
		after := len(sink.errors) + len(sink.infos)
		if after != before {
			t.Fatalf("expected no logs when not allowed")
		}
	})
}

// ---- Merged from logger_more_test.go ----

type sink struct {
	logr.LogSink
	infos  int
	errors int
}

func (s *sink) Init(logr.RuntimeInfo) {}
func (s *sink) Enabled(_ int) bool    { return true }
func (s *sink) Info(_ int, _ string, _ ...interface{}) {
	s.infos++
}
func (s *sink) Error(_ error, _ string, _ ...interface{}) {
	s.errors++
}
func (s *sink) WithValues(_ ...interface{}) logr.LogSink { return s }
func (s *sink) WithName(_ string) logr.LogSink           { return s }

func TestSettings_allowed_RespectsLevel(t *testing.T) {
	old := Settings
	defer func() { Settings = old }()
	Settings.Level = 1
	if !Settings.allowed(1) {
		t.Fatalf("expected allowed")
	}
	if Settings.allowed(2) {
		t.Fatalf("expected not allowed")
	}
}

func TestLogger_Error_Nil_NoLog(t *testing.T) {
	s := &sink{}
	l := &Logger{Real: logr.New(s)}
	l.Error(nil, "msg")
	if s.infos != 0 && s.errors != 0 {
		t.Fatalf("expected no logs")
	}
}

func TestLogger_Error_UnwrapsWrappedErrorAndLogsError(t *testing.T) {
	old := Settings
	defer func() { Settings = old }()
	Settings.Level = 10

	s := &sink{}
	l := &Logger{Real: logr.New(s)}
	err := errors.New("root")
	wrapped := liberr.Wrap(err)
	l.Error(wrapped, "msg", "k", "v")
	// Wrapped error logs via Info() path in Logger.Error
	if s.infos == 0 {
		t.Fatalf("expected info log")
	}
}

func TestLevelLoggerImpl_Info_RespectsSettingsAllowed(t *testing.T) {
	old := Settings
	defer func() { Settings = old }()
	Settings.Level = 0
	s := &sink{}
	ll := &levelLoggerImpl{real: logr.New(s), level: 1}
	ll.Info("msg")
	if s.infos != 0 {
		t.Fatalf("expected no info")
	}
	Settings.Level = 2
	ll.Info("msg")
	if s.infos == 0 {
		t.Fatalf("expected info")
	}
}
